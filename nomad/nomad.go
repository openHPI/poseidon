package nomad

import (
	"context"
	"errors"
	"fmt"
	nomadApi "github.com/hashicorp/nomad/api"
	"github.com/hashicorp/nomad/nomad/structs"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/logging"
	"net/url"
	"time"
)

var (
	log                              = logging.GetLogger("nomad")
	ErrorExecutorCommunicationFailed = errors.New("communication with executor failed")
	errEvaluation                    = errors.New("evaluation could not complete")
	errPlacingAllocations            = errors.New("failed to place all allocations")
)

type AllocationProcessor func(*nomadApi.Allocation)

// ExecutorAPI provides access to an container orchestration solution.
type ExecutorAPI interface {
	apiQuerier

	// LoadRunners loads all allocations of the specified job which are running and not about to get stopped.
	LoadRunners(jobID string) (runnerIds []string, err error)

	// MonitorEvaluation monitors the given evaluation ID.
	// It waits until the evaluation reaches one of the states complete, canceled or failed.
	// If the evaluation was not successful, an error containing the failures is returned.
	// See also https://github.com/hashicorp/nomad/blob/7d5a9ecde95c18da94c9b6ace2565afbfdd6a40d/command/monitor.go#L175
	MonitorEvaluation(evaluationID string, ctx context.Context) error

	// WatchAllocations listens on the Nomad event stream for allocation events.
	// Depending on the incoming event, any of the given function is executed.
	WatchAllocations(ctx context.Context, onNewAllocation, onDeletedAllocation AllocationProcessor) error
}

// APIClient implements the ExecutorAPI interface and can be used to perform different operations on the real
// Executor API and its return values.
type APIClient struct {
	apiQuerier
}

// NewExecutorAPI creates a new api client.
// One client is usually sufficient for the complete runtime of the API.
func NewExecutorAPI(nomadURL *url.URL, nomadNamespace string) (ExecutorAPI, error) {
	client := &APIClient{apiQuerier: &nomadAPIClient{}}
	err := client.init(nomadURL, nomadNamespace)
	return client, err
}

// init prepares an apiClient to be able to communicate to a provided Nomad API.
func (a *APIClient) init(nomadURL *url.URL, nomadNamespace string) error {
	return a.apiQuerier.init(nomadURL, nomadNamespace)
}

// LoadRunners loads the allocations of the specified job.
func (a *APIClient) LoadRunners(jobID string) (runnerIds []string, err error) {
	list, err := a.loadRunners(jobID)
	if err != nil {
		return nil, err
	}
	for _, stub := range list {
		// only add allocations which are running and not about to be stopped
		if stub.ClientStatus == nomadApi.AllocClientStatusRunning && stub.DesiredStatus == nomadApi.AllocDesiredStatusRun {
			runnerIds = append(runnerIds, stub.ID)
		}
	}
	return runnerIds, nil
}

func (a *APIClient) MonitorEvaluation(evaluationID string, ctx context.Context) error {
	stream, err := a.EvaluationStream(evaluationID, ctx)
	if err != nil {
		return fmt.Errorf("failed retrieving evaluation stream: %w", err)
	}
	// If ctx is canceled, the stream will be closed by Nomad and we exit the for loop.
	return receiveAndHandleNomadAPIEvents(stream, handleEvaluationEvent)
}

func (a *APIClient) WatchAllocations(ctx context.Context,
	onNewAllocation, onDeletedAllocation AllocationProcessor) error {
	startTime := time.Now().UnixNano()
	stream, err := a.AllocationStream(ctx)
	if err != nil {
		return fmt.Errorf("failed retrieving allocation stream: %w", err)
	}
	pendingAllocations := make(map[string]bool)

	handler := func(event *nomadApi.Event) (bool, error) {
		return false, handleAllocationEvent(startTime, pendingAllocations, event, onNewAllocation, onDeletedAllocation)
	}

	err = receiveAndHandleNomadAPIEvents(stream, handler)
	return err
}

// nomadAPIEventHandler is a function that receives a nomadApi.Event and processes it.
// It is called by an event listening loop. For each received event, the function is called.
// If done is true, the calling function knows that it should break out of the event listening
// loop.
type nomadAPIEventHandler func(event *nomadApi.Event) (done bool, err error)

// receiveAndHandleNomadAPIEvents receives events from the Nomad event stream and calls the handler function for
// each received event. It skips heartbeat events and returns an error if the received events contain an error.
func receiveAndHandleNomadAPIEvents(stream <-chan *nomadApi.Events, handler nomadAPIEventHandler) error {
	// If original context is canceled, the stream will be closed by Nomad and we exit the for loop.
	for events := range stream {
		if events.IsHeartbeat() {
			continue
		}
		if err := events.Err; err != nil {
			return fmt.Errorf("error receiving events: %w", err)
		}
		for _, event := range events.Events {
			// Don't take the address of the loop variable as the underlying value might change
			eventCopy := event
			done, err := handler(&eventCopy)
			if err != nil || done {
				return err
			}
		}
	}
	return nil
}

// handleEvaluationEvent is a nomadAPIEventHandler that returns whether the evaluation described by the event
// was successful.
func handleEvaluationEvent(event *nomadApi.Event) (bool, error) {
	eval, err := event.Evaluation()
	if err != nil {
		return true, fmt.Errorf("failed to monitor evaluation: %w", err)
	}
	switch eval.Status {
	case structs.EvalStatusComplete, structs.EvalStatusCancelled, structs.EvalStatusFailed:
		return true, checkEvaluation(eval)
	}
	return false, nil
}

// handleAllocationEvent is a nomadAPIEventHandler that processes allocation events.
// If a new allocation is received, onNewAllocation is called. If an allocation is deleted, onDeletedAllocation
// is called. The pendingAllocations map is used to store allocations that are pending but not started yet. Using the
// map the state is persisted between multiple calls of this function.
func handleAllocationEvent(startTime int64, pendingAllocations map[string]bool, event *nomadApi.Event,
	onNewAllocation, onDeletedAllocation AllocationProcessor) error {
	if event.Type != structs.TypeAllocationUpdated {
		return nil
	}
	alloc, err := event.Allocation()
	if err != nil {
		return fmt.Errorf("failed to retrieve allocation from event: %w", err)
	} else if alloc == nil {
		return nil
	}

	// When starting the API and listening on the Nomad event stream we might get events that already
	// happened from Nomad as it seems to buffer them for a certain duration.
	// Ignore old events here.
	if alloc.ModifyTime < startTime {
		return nil
	}

	if alloc.ClientStatus == structs.AllocClientStatusRunning {
		switch alloc.DesiredStatus {
		case structs.AllocDesiredStatusStop:
			onDeletedAllocation(alloc)
		case structs.AllocDesiredStatusRun:
			// is first event that marks the transition between pending and running?
			_, ok := pendingAllocations[alloc.ID]
			if ok {
				onNewAllocation(alloc)
				delete(pendingAllocations, alloc.ID)
			}
		}
	}

	if alloc.ClientStatus == structs.AllocClientStatusPending && alloc.DesiredStatus == structs.AllocDesiredStatusRun {
		// allocation is started, wait until it runs and add to our list afterwards
		pendingAllocations[alloc.ID] = true
	}
	return nil
}

// checkEvaluation checks whether the given evaluation failed.
// If the evaluation failed, it returns an error with a message containing the failure information.
func checkEvaluation(eval *nomadApi.Evaluation) (err error) {
	if len(eval.FailedTGAllocs) == 0 {
		if eval.Status != structs.EvalStatusComplete {
			err = fmt.Errorf("%w: %q", errEvaluation, eval.Status)
		}
	} else {
		err = fmt.Errorf("evaluation %q finished with status %q but %w", eval.ID, eval.Status, errPlacingAllocations)
		for taskGroup, metrics := range eval.FailedTGAllocs {
			err = fmt.Errorf("%w\n%s: %#v", err, taskGroup, metrics)
		}
		if eval.BlockedEval != "" {
			err = fmt.Errorf("%w\nEvaluation %q waiting for additional capacity to place remainder", err, eval.BlockedEval)
		}
	}
	return err
}
