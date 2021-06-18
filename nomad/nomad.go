package nomad

import (
	"context"
	"errors"
	"fmt"
	nomadApi "github.com/hashicorp/nomad/api"
	"github.com/hashicorp/nomad/nomad/structs"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/config"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/logging"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/util"
	"io"
	"net/url"
	"time"
)

var (
	log                              = logging.GetLogger("nomad")
	ErrorExecutorCommunicationFailed = errors.New("communication with executor failed")
	ErrorEvaluation                  = errors.New("evaluation could not complete")
	ErrorPlacingAllocations          = errors.New("failed to place all allocations")
	ErrorLoadingJob                  = errors.New("failed to load job")
)

type AllocationProcessor func(*nomadApi.Allocation)

// ExecutorAPI provides access to an container orchestration solution.
type ExecutorAPI interface {
	apiQuerier

	// LoadEnvironmentJobs loads all environment jobs.
	LoadEnvironmentJobs() ([]*nomadApi.Job, error)

	// LoadRunnerJobs loads all runner jobs specific for the environment.
	LoadRunnerJobs(environmentID string) ([]*nomadApi.Job, error)

	// LoadRunnerIDs returns the IDs of all runners of the specified environment which are running and not about to
	// get stopped.
	LoadRunnerIDs(environmentID string) (runnerIds []string, err error)

	// RegisterTemplateJob creates a template job based on the default job configuration and the given parameters.
	// It registers the job and waits until the registration completes.
	RegisterTemplateJob(defaultJob *nomadApi.Job, id string,
		prewarmingPoolSize, cpuLimit, memoryLimit uint,
		image string, networkAccess bool, exposedPorts []uint16) (*nomadApi.Job, error)

	// RegisterRunnerJob creates a runner job based on the template job.
	// It registers the job and waits until the registration completes.
	RegisterRunnerJob(template *nomadApi.Job) error

	// MonitorEvaluation monitors the given evaluation ID.
	// It waits until the evaluation reaches one of the states complete, canceled or failed.
	// If the evaluation was not successful, an error containing the failures is returned.
	// See also https://github.com/hashicorp/nomad/blob/7d5a9ecde95c18da94c9b6ace2565afbfdd6a40d/command/monitor.go#L175
	MonitorEvaluation(evaluationID string, ctx context.Context) error

	// WatchAllocations listens on the Nomad event stream for allocation events.
	// Depending on the incoming event, any of the given function is executed.
	WatchAllocations(ctx context.Context, onNewAllocation, onDeletedAllocation AllocationProcessor) error

	// ExecuteCommand executes the given command in the allocation with the given id.
	// It writes the output of the command to stdout/stderr and reads input from stdin.
	// If tty is true, the command will run with a tty.
	ExecuteCommand(allocationID string, ctx context.Context, command []string, tty bool,
		stdin io.Reader, stdout, stderr io.Writer) (int, error)

	// MarkRunnerAsUsed marks the runner with the given ID as used. It also stores the timeout duration in the metadata.
	MarkRunnerAsUsed(runnerID string, duration int) error
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

func (a *APIClient) LoadRunnerIDs(environmentID string) (runnerIDs []string, err error) {
	list, err := a.listJobs(environmentID)
	if err != nil {
		return nil, err
	}
	for _, jobListStub := range list {
		allocationRunning := jobListStub.JobSummary.Summary[TaskGroupName].Running > 0
		if jobListStub.Status == structs.JobStatusRunning && allocationRunning {
			runnerIDs = append(runnerIDs, jobListStub.ID)
		}
	}
	return runnerIDs, nil
}

func (a *APIClient) LoadRunnerJobs(environmentID string) ([]*nomadApi.Job, error) {
	runnerIDs, err := a.LoadRunnerIDs(environmentID)
	if err != nil {
		return []*nomadApi.Job{}, fmt.Errorf("couldn't load jobs: %w", err)
	}

	var occurredError error
	jobs := make([]*nomadApi.Job, 0, len(runnerIDs))
	for _, id := range runnerIDs {
		job, err := a.apiQuerier.job(id)
		if err != nil {
			if occurredError == nil {
				occurredError = ErrorLoadingJob
			}
			occurredError = fmt.Errorf("%w: couldn't load job info for runner %s - %v", occurredError, id, err)
			continue
		}
		jobs = append(jobs, job)
	}
	return jobs, occurredError
}

func (a *APIClient) MonitorEvaluation(evaluationID string, ctx context.Context) error {
	stream, err := a.apiQuerier.EvaluationStream(evaluationID, ctx)
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
			err = fmt.Errorf("%w: %q", ErrorEvaluation, eval.Status)
		}
	} else {
		err = fmt.Errorf("evaluation %q finished with status %q but %w", eval.ID, eval.Status, ErrorPlacingAllocations)
		for taskGroup, metrics := range eval.FailedTGAllocs {
			err = fmt.Errorf("%w\n%s: %#v", err, taskGroup, metrics)
		}
		if eval.BlockedEval != "" {
			err = fmt.Errorf("%w\nEvaluation %q waiting for additional capacity to place remainder", err, eval.BlockedEval)
		}
	}
	return err
}

func (a *APIClient) MarkRunnerAsUsed(runnerID string) error {
	job, err := a.job(runnerID)
	if err != nil {
		return fmt.Errorf("couldn't retrieve job info: %w", err)
	}
	err = SetMetaConfigValue(job, ConfigMetaUsedKey, ConfigMetaUsedValue)
	if err != nil {
		return fmt.Errorf("couldn't update runner in job as used: %w", err)
	}
	_, err = a.RegisterNomadJob(job)
	if err != nil {
		return fmt.Errorf("couldn't update runner config: %w", err)
	}
	return nil
}

func (a *APIClient) LoadEnvironmentJobs() ([]*nomadApi.Job, error) {
	jobStubs, err := a.listJobs(TemplateJobPrefix)
	if err != nil {
		return []*nomadApi.Job{}, fmt.Errorf("couldn't load jobs: %w", err)
	}

	jobs := make([]*nomadApi.Job, 0, len(jobStubs))
	for _, jobStub := range jobStubs {
		job, err := a.apiQuerier.job(jobStub.ID)
		if err != nil {
			return []*nomadApi.Job{}, fmt.Errorf("couldn't load job info for job %v: %w", jobStub.ID, err)
		}
		jobs = append(jobs, job)
	}
	return jobs, nil
}

// ExecuteCommand executes the given command in the given allocation.
// If tty is true, Nomad would normally write stdout and stderr of the command
// both on the stdout stream. However, if the InteractiveStderr server config option is true,
// we make sure that stdout and stderr are split correctly.
// In order for the stderr splitting to work, the command must have the structure
// []string{..., "sh", "-c", "my-command"}.
func (a *APIClient) ExecuteCommand(allocationID string,
	ctx context.Context, command []string, tty bool,
	stdin io.Reader, stdout, stderr io.Writer) (int, error) {
	if tty && config.Config.Server.InteractiveStderr {
		return a.executeCommandInteractivelyWithStderr(allocationID, ctx, command, stdin, stdout, stderr)
	}
	return a.apiQuerier.Execute(allocationID, ctx, command, tty, stdin, stdout, stderr)
}

// executeCommandInteractivelyWithStderr executes the given command interactively and splits stdout
// and stderr correctly. Normally, using Nomad to execute a command with tty=true (in order to have
// an interactive connection and possibly a fully working shell), would result in stdout and stderr
// to be served both over stdout. This function circumvents this by creating a fifo for the stderr
// of the command and starting a second execution that reads the stderr from that fifo.
func (a *APIClient) executeCommandInteractivelyWithStderr(allocationID string, ctx context.Context,
	command []string, stdin io.Reader, stdout, stderr io.Writer) (int, error) {
	// Use current nano time to make the stderr fifo kind of unique.
	currentNanoTime := time.Now().UnixNano()
	// We expect the command to be like []string{..., "sh", "-c", "my-command"}.
	oldCommand := command[len(command)-1]
	// Take the last command which is the one to be executed and wrap it to redirect stderr.
	command[len(command)-1] = wrapCommandForStderrFifo(currentNanoTime, oldCommand)

	stderrExitChan := make(chan int)
	go func() {
		// Catch stderr in separate execution.
		exit, err := a.Execute(allocationID, ctx, stderrFifoCommand(currentNanoTime), true, util.NullReader{}, stderr, io.Discard)
		if err != nil {
			log.WithError(err).WithField("runner", allocationID).Warn("Stderr task finished with error")
		}
		stderrExitChan <- exit
	}()

	exit, err := a.Execute(allocationID, ctx, command, true, stdin, stdout, io.Discard)

	// Wait until the stderr catch command finished to make sure we receive all output.
	<-stderrExitChan
	return exit, err
}

const (
	// stderrFifoFormat represents the format we use for our stderr fifos. The %d should be unique for the execution
	// as otherwise multiple executions are not possible.
	// Example: /tmp/stderr_1623330777825234133.fifo
	stderrFifoFormat = "/tmp/stderr_%d.fifo"
	// stderrFifoCommandFormat, if executed, is supposed to create a fifo, read from it and remove it in the end.
	// Example: mkfifo my.fifo && (cat my.fifo; rm my.fifo)
	stderrFifoCommandFormat = "mkfifo %s && (cat %s; rm %s)"
	// stderrWrapperCommandFormat, if executed, is supposed to wait until a fifo exists (it sleeps 10ms to reduce load
	// cause by busy waiting on the system). Once the fifo exists, the given command is executed and its stderr
	// redirected to the fifo.
	// Example: until [ -e my.fifo ]; do sleep 0.01; done; (echo "my.fifo exists") 2> my.fifo
	stderrWrapperCommandFormat = "until [ -e %s ]; do sleep 0.01; done; (%s) 2> %s"
)

func stderrFifoCommand(id int64) []string {
	stderrFifoPath := stderrFifo(id)
	return []string{"sh", "-c", fmt.Sprintf(stderrFifoCommandFormat, stderrFifoPath, stderrFifoPath, stderrFifoPath)}
}

func wrapCommandForStderrFifo(id int64, command string) string {
	stderrFifoPath := stderrFifo(id)
	return fmt.Sprintf(stderrWrapperCommandFormat, stderrFifoPath, command, stderrFifoPath)
}

func stderrFifo(id int64) string {
	return fmt.Sprintf(stderrFifoFormat, id)
}
