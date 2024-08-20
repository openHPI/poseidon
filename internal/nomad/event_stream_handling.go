package nomad

import (
	"context"
	"fmt"
	"strings"
	"time"

	nomadApi "github.com/hashicorp/nomad/api"
	"github.com/hashicorp/nomad/nomad/structs"
	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
	"github.com/openHPI/poseidon/pkg/dto"
	"github.com/openHPI/poseidon/pkg/monitoring"
	"github.com/openHPI/poseidon/pkg/storage"
)

// AllocationProcessing includes the callbacks to interact with allocation events.
type AllocationProcessing struct {
	OnNew     NewAllocationProcessor
	OnDeleted DeletedAllocationProcessor
}

// nomadAPIEventHandler is a function that receives a nomadApi.Event and processes it.
// It is called by an event listening loop. For each received event, the function is called.
// If done is true, the calling function knows that it should break out of the event listening
// loop.
type nomadAPIEventHandler func(event *nomadApi.Event) (done bool, err error)

// DeletedAllocationProcessor is a handler that will be called for each deleted allocation.
// removedByPoseidon should be true iff the Nomad Manager has removed the runner before.
type (
	DeletedAllocationProcessor func(ctx context.Context, jobID string, RunnerDeletedReason error) (removedByPoseidon bool)
	NewAllocationProcessor     func(context.Context, *nomadApi.Allocation, time.Duration)
)

type allocationData struct {
	// allocClientStatus defines the state defined by Nomad.
	allocClientStatus string
	// allocDesiredStatus defines if the allocation wants to be running or being stopped.
	allocDesiredStatus string
	jobID              string
	start              time.Time
	// stopExpected is used to suppress warnings that could be triggered by a race condition
	// between the Inactivity timer and an external event leadng to allocation rescheduling.
	stopExpected bool
	// Just debugging information
	allocNomadNode string
}

// resultChannelWriteTimeout is to detect the error when more element are written into a channel than expected.
const resultChannelWriteTimeout = 10 * time.Millisecond

// initializeAllocations initializes the allocations storage that keeps track of running allocations.
// It is called on every runner recovery.
func (a *APIClient) initializeAllocations(environmentID dto.EnvironmentID) {
	allocationStubs, err := a.listAllocations()
	if err != nil {
		log.WithError(err).Warn("Could not initialize allocations")
	} else {
		for _, stub := range allocationStubs {
			switch {
			case IsEnvironmentTemplateID(stub.JobID):
				continue
			case !strings.HasPrefix(stub.JobID, RunnerJobID(environmentID, "")):
				continue
			case stub.ClientStatus == structs.AllocClientStatusPending || stub.ClientStatus == structs.AllocClientStatusRunning:
				log.WithField("jobID", stub.JobID).WithField("status", stub.ClientStatus).Debug("Recovered Allocation")
				a.allocations.Add(stub.ID, &allocationData{
					allocClientStatus:  stub.ClientStatus,
					allocDesiredStatus: stub.DesiredStatus,
					jobID:              stub.JobID,
					start:              time.Unix(0, stub.CreateTime),
					allocNomadNode:     stub.NodeName,
				})
			}
		}
	}
}

func (a *APIClient) WatchEventStream(ctx context.Context, callbacks *AllocationProcessing) error {
	startTime := time.Now().UnixNano()
	stream, err := a.EventStream(ctx)
	if err != nil {
		return fmt.Errorf("failed retrieving allocation stream: %w", err)
	}

	handler := func(event *nomadApi.Event) (bool, error) {
		dumpNomadEventToInflux(event)
		switch event.Topic {
		case nomadApi.TopicEvaluation:
			return false, handleEvaluationEvent(a.evaluations, event)
		case nomadApi.TopicAllocation:
			return false, handleAllocationEvent(ctx, startTime, a.allocations, event, callbacks)
		default:
			return false, nil
		}
	}

	a.isListening = true
	err = receiveAndHandleNomadAPIEvents(stream, handler)
	a.isListening = false
	return err
}

func (a *APIClient) MonitorEvaluation(ctx context.Context, evaluationID string) (err error) {
	evaluationErrorChannel := make(chan error, 1)
	a.evaluations.Add(evaluationID, evaluationErrorChannel)

	if !a.isListening {
		var cancel context.CancelFunc
		ctx, cancel = context.WithCancel(ctx)
		defer cancel() // cancel the WatchEventStream when the evaluation result was read.

		go func() {
			err = a.WatchEventStream(ctx, &AllocationProcessing{
				OnNew:     func(_ context.Context, _ *nomadApi.Allocation, _ time.Duration) {},
				OnDeleted: func(_ context.Context, _ string, _ error) bool { return false },
			})
			cancel() // cancel the waiting for an evaluation result if watching the event stream ends.
		}()
	}

	select {
	case <-ctx.Done():
		return err
	case err := <-evaluationErrorChannel:
		// At the moment we expect only one error to be sent via this channel.
		return err
	}
}

func dumpNomadEventToInflux(event *nomadApi.Event) {
	dataPoint := influxdb2.NewPointWithMeasurement(monitoring.MeasurementNomadEvents)
	dataPoint.AddTag("topic", event.Topic.String())
	dataPoint.AddTag("type", event.Type)
	dataPoint.AddTag("key", event.Key)
	dataPoint.AddField("payload", event.Payload)
	dataPoint.AddTag("timestamp", time.Now().Format("03:04:05.000000000"))
	monitoring.WriteInfluxPoint(dataPoint)
}

// receiveAndHandleNomadAPIEvents receives events from the Nomad event stream and calls the handler function for
// each received event. It skips heartbeat events and returns an error if the received events contain an error.
func receiveAndHandleNomadAPIEvents(stream <-chan *nomadApi.Events, handler nomadAPIEventHandler) error {
	// If original context is canceled, the stream will be closed by Nomad, and we exit the for loop.
	for events := range stream {
		if err := events.Err; err != nil {
			return fmt.Errorf("error receiving events: %w", err)
		} else if events.IsHeartbeat() {
			continue
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

// handleEvaluationEvent is an event handler that returns whether the evaluation described by the event
// was successful.
func handleEvaluationEvent(evaluations storage.Storage[chan error], event *nomadApi.Event) error {
	eval, err := event.Evaluation()
	if err != nil {
		return fmt.Errorf("failed to monitor evaluation: %w", err)
	}
	switch eval.Status {
	case structs.EvalStatusComplete, structs.EvalStatusCancelled, structs.EvalStatusFailed:
		resultChannel, ok := evaluations.Get(eval.ID)
		if ok {
			evalErr := checkEvaluation(eval)
			select {
			case resultChannel <- evalErr:
				close(resultChannel)
				evaluations.Delete(eval.ID)
			case <-time.After(resultChannelWriteTimeout):
				log.WithField("length", len(resultChannel)).
					WithField("eval", eval).
					WithError(evalErr).
					Error("Sending to the evaluation channel timed out")
			}
		}
	}
	return nil
}

// checkEvaluation checks whether the given evaluation failed.
// If the evaluation failed, it returns an error with a message containing the failure information.
func checkEvaluation(eval *nomadApi.Evaluation) (err error) {
	if len(eval.FailedTGAllocs) == 0 {
		if eval.Status != structs.EvalStatusComplete {
			err = fmt.Errorf("%w: %q", ErrEvaluation, eval.Status)
		}
	} else {
		err = fmt.Errorf("evaluation %q finished with status %q but %w", eval.ID, eval.Status, ErrPlacingAllocations)
		for taskGroup, metrics := range eval.FailedTGAllocs {
			err = fmt.Errorf("%w\n%s: %#v", err, taskGroup, metrics)
		}
		if eval.BlockedEval != "" {
			err = fmt.Errorf("%w\nEvaluation %q waiting for additional capacity to place remainder", err, eval.BlockedEval)
		}
	}
	return err
}

// handleAllocationEvent is an event handler that processes allocation events.
// If a new allocation is received, onNewAllocation is called. If an allocation is deleted, onDeletedAllocation
// is called. The allocations storage is used to track pending and running allocations. Using the
// storage the state is persisted between multiple calls of this function.
func handleAllocationEvent(ctx context.Context, startTime int64, allocations storage.Storage[*allocationData],
	event *nomadApi.Event, callbacks *AllocationProcessing,
) error {
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

	if valid := filterDuplicateEvents(alloc, allocations); !valid {
		return nil
	}

	log.WithField("alloc_id", alloc.ID).
		WithField("ClientStatus", alloc.ClientStatus).
		WithField("DesiredStatus", alloc.DesiredStatus).
		WithField("PrevAllocation", alloc.PreviousAllocation).
		WithField("NextAllocation", alloc.NextAllocation).
		Debug("Handle Allocation Event")

	allocData := updateAllocationData(alloc, allocations)

	switch alloc.ClientStatus {
	case structs.AllocClientStatusPending:
		handlePendingAllocationEvent(ctx, alloc, allocData, allocations, callbacks)
	case structs.AllocClientStatusRunning:
		handleRunningAllocationEvent(ctx, alloc, allocData, allocations, callbacks)
	case structs.AllocClientStatusComplete:
		handleCompleteAllocationEvent(ctx, alloc, allocData, allocations, callbacks)
	case structs.AllocClientStatusFailed:
		handleFailedAllocationEvent(ctx, alloc, allocData, allocations, callbacks)
	case structs.AllocClientStatusLost:
		handleLostAllocationEvent(ctx, alloc, allocData, allocations, callbacks)
	default:
		log.WithField("alloc", alloc).Warn("Other Client Status")
	}
	return nil
}

// filterDuplicateEvents identifies duplicate events or events of unknown allocations.
func filterDuplicateEvents(alloc *nomadApi.Allocation, allocations storage.Storage[*allocationData]) (valid bool) {
	newAllocationExpected := alloc.ClientStatus == structs.AllocClientStatusPending &&
		alloc.DesiredStatus == structs.AllocDesiredStatusRun
	allocData, ok := allocations.Get(alloc.ID)

	switch {
	case !ok && newAllocationExpected:
		return true
	case !ok:
		// This case happens in case of an error or when an event that led to the deletion of the alloc data is duplicated.
		log.WithField("allocID", alloc.ID).Debug("Ignoring unknown allocation")
		return false
	case alloc.ClientStatus == allocData.allocClientStatus && alloc.DesiredStatus == allocData.allocDesiredStatus:
		log.WithField("allocID", alloc.ID).Debug("Ignoring duplicate event")
		return false
	default:
		return true
	}
}

// updateAllocationData updates the allocation tracking data according to the passed alloc.
// The allocation data before this allocation update is returned.
func updateAllocationData(
	alloc *nomadApi.Allocation, allocations storage.Storage[*allocationData],
) (previous *allocationData) {
	allocData, ok := allocations.Get(alloc.ID)
	if ok {
		data := *allocData
		previous = &data

		allocData.allocClientStatus = alloc.ClientStatus
		allocData.allocDesiredStatus = alloc.DesiredStatus
		allocations.Add(alloc.ID, allocData)
	}
	return previous
}

// handlePendingAllocationEvent manages allocation that are currently pending.
// This allows the handling of startups and re-placements of allocations.
func handlePendingAllocationEvent(ctx context.Context, alloc *nomadApi.Allocation, allocData *allocationData,
	allocations storage.Storage[*allocationData], callbacks *AllocationProcessing,
) {
	var stopExpected bool
	switch alloc.DesiredStatus {
	case structs.AllocDesiredStatusRun:
		if allocData != nil {
			// Handle Allocation restart.
			var reason error
			if isOOMKilled(alloc) {
				reason = ErrOOMKilled
			}
			callbacks.OnDeleted(ctx, alloc.JobID, reason)
		} else if alloc.PreviousAllocation != "" {
			// Handle Runner (/Container) re-allocations.
			if prevData, ok := allocations.Get(alloc.PreviousAllocation); ok {
				stopExpected = callbacks.OnDeleted(ctx, prevData.jobID, ErrAllocationRescheduled)
				allocations.Delete(alloc.PreviousAllocation)
			} else {
				log.WithField("alloc", alloc).Warn("Previous Allocation not found")
			}
		}

		// Store Pending Allocation - Allocation gets started, wait until it runs.
		allocations.Add(alloc.ID, &allocationData{
			allocClientStatus:  alloc.ClientStatus,
			allocDesiredStatus: alloc.DesiredStatus,
			jobID:              alloc.JobID,
			start:              time.Now(),
			allocNomadNode:     alloc.NodeName,
			stopExpected:       stopExpected,
		})
	case structs.AllocDesiredStatusStop:
		// As this allocation was still pending, we don't have to propagate its deletion.
		allocations.Delete(alloc.ID)
		// Anyway, we want to monitor the occurrences.
		if !allocData.stopExpected {
			log.WithField("alloc", alloc).Warn("Pending allocation was stopped unexpectedly")
		} else {
			// This log statement is just for measuring how common the mentioned race condition is.
			log.WithField("alloc", alloc).Warn("Pending allocation was stopped expectedly")
		}
	default:
		log.WithField("alloc", alloc).Warn("Other Desired Status")
	}
}

// handleRunningAllocationEvent calls the passed AllocationProcessor filtering similar events.
func handleRunningAllocationEvent(ctx context.Context, alloc *nomadApi.Allocation, allocData *allocationData,
	allocations storage.Storage[*allocationData], callbacks *AllocationProcessing,
) {
	switch alloc.DesiredStatus {
	case structs.AllocDesiredStatusRun:
		startupDuration := time.Since(allocData.start)
		callbacks.OnNew(ctx, alloc, startupDuration)
	case structs.AllocDesiredStatusStop:
		callbacks.OnDeleted(ctx, alloc.JobID, ErrAllocationCompleted)
		allocations.Delete(alloc.ID)
	default:
		log.WithField("alloc", alloc).Warn("Other Desired Status")
	}
}

// handleCompleteAllocationEvent handles allocations that stopped.
func handleCompleteAllocationEvent(_ context.Context, alloc *nomadApi.Allocation, _ *allocationData,
	allocations storage.Storage[*allocationData], _ *AllocationProcessing,
) {
	switch alloc.DesiredStatus {
	case structs.AllocDesiredStatusRun:
		log.WithField("alloc", alloc).Warn("Complete allocation desires to run")
	case structs.AllocDesiredStatusStop:
		// We already handled the removal when the allocation desired to stop.
		_, ok := allocations.Get(alloc.ID)
		if ok {
			log.WithField("alloc", alloc).Warn("Complete allocation not removed")
		}
	default:
		log.WithField("alloc", alloc).Warn("Other Desired Status")
	}
}

// handleFailedAllocationEvent logs only the last of the multiple failure events.
func handleFailedAllocationEvent(ctx context.Context, alloc *nomadApi.Allocation, allocData *allocationData,
	allocations storage.Storage[*allocationData], callbacks *AllocationProcessing,
) {
	// The stop is expected when the allocation desired to stop even before it failed.
	reschedulingExpected := allocData.allocDesiredStatus != structs.AllocDesiredStatusStop
	handleStoppingAllocationEvent(ctx, alloc, allocations, callbacks, reschedulingExpected)
}

// handleLostAllocationEvent logs only the last of the multiple lost events.
func handleLostAllocationEvent(ctx context.Context, alloc *nomadApi.Allocation, allocData *allocationData,
	allocations storage.Storage[*allocationData], callbacks *AllocationProcessing,
) {
	// The stop is expected when the allocation desired to stop even before it got lost.
	reschedulingExpected := allocData.allocDesiredStatus != structs.AllocDesiredStatusStop
	handleStoppingAllocationEvent(ctx, alloc, allocations, callbacks, reschedulingExpected)
}

func handleStoppingAllocationEvent(ctx context.Context, alloc *nomadApi.Allocation, allocations storage.Storage[*allocationData],
	callbacks *AllocationProcessing, reschedulingExpected bool,
) {
	replacementAllocationScheduled := alloc.NextAllocation != ""
	correctRescheduling := reschedulingExpected == replacementAllocationScheduled

	removedByPoseidon := false
	if !replacementAllocationScheduled {
		var reason error
		if correctRescheduling {
			reason = ErrAllocationStoppedUnexpectedly
		} else {
			reason = ErrAllocationRescheduledUnexpectedly
		}
		removedByPoseidon = callbacks.OnDeleted(ctx, alloc.JobID, reason)
		allocations.Delete(alloc.ID)
	}

	entry := log.WithField("job", alloc.JobID)
	if !removedByPoseidon && !correctRescheduling {
		entry.WithField("alloc", alloc).Warn("Unexpected Allocation Stopping / Restarting")
	} else {
		entry.Trace("Expected Allocation Stopping / Restarting")
	}
}
