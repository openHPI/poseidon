package nomad

import (
	"context"
	"errors"
	"fmt"
	nomadApi "github.com/hashicorp/nomad/api"
	"github.com/hashicorp/nomad/nomad/structs"
	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
	"github.com/influxdata/influxdb-client-go/v2/api/write"
	"github.com/openHPI/poseidon/internal/config"
	"github.com/openHPI/poseidon/pkg/dto"
	"github.com/openHPI/poseidon/pkg/logging"
	"github.com/openHPI/poseidon/pkg/monitoring"
	"github.com/openHPI/poseidon/pkg/nullio"
	"github.com/openHPI/poseidon/pkg/storage"
	"io"
	"strconv"
	"strings"
	"time"
)

var (
	log                              = logging.GetLogger("nomad")
	ErrorExecutorCommunicationFailed = errors.New("communication with executor failed")
	ErrorEvaluation                  = errors.New("evaluation could not complete")
	ErrorPlacingAllocations          = errors.New("failed to place all allocations")
	ErrorLoadingJob                  = errors.New("failed to load job")
	ErrorNoAllocatedResourcesFound   = errors.New("no allocated resources found")
)

// resultChannelWriteTimeout is to detect the error when more element are written into a channel than expected.
const resultChannelWriteTimeout = 10 * time.Millisecond

// AllocationProcessing includes the callbacks to interact with allocation events.
type AllocationProcessing struct {
	OnNew     NewAllocationProcessor
	OnDeleted DeletedAllocationProcessor
}
type DeletedAllocationProcessor func(jobID string) (removedByPoseidon bool)
type NewAllocationProcessor func(*nomadApi.Allocation, time.Duration)

type allocationData struct {
	// allocClientStatus defines the state defined by Nomad.
	allocClientStatus string
	// allocDesiredStatus defines if the allocation wants to be running or being stopped.
	allocDesiredStatus string
	jobID              string
	start              time.Time
	// Just debugging information
	allocNomadNode string
}

// ExecutorAPI provides access to a container orchestration solution.
type ExecutorAPI interface {
	apiQuerier

	// LoadEnvironmentJobs loads all environment jobs.
	LoadEnvironmentJobs() ([]*nomadApi.Job, error)

	// LoadRunnerJobs loads all runner jobs specific for the environment.
	LoadRunnerJobs(environmentID dto.EnvironmentID) ([]*nomadApi.Job, error)

	// LoadRunnerIDs returns the IDs of all runners with the specified id prefix which are not about to
	// get stopped.
	LoadRunnerIDs(prefix string) (runnerIds []string, err error)

	// LoadRunnerPortMappings returns the mapped ports of the runner.
	LoadRunnerPortMappings(runnerID string) ([]nomadApi.PortMapping, error)

	// RegisterRunnerJob creates a runner job based on the template job.
	// It registers the job and waits until the registration completes.
	RegisterRunnerJob(template *nomadApi.Job) error

	// MonitorEvaluation monitors the given evaluation ID.
	// It waits until the evaluation reaches one of the states complete, canceled or failed.
	// If the evaluation was not successful, an error containing the failures is returned.
	// See also https://github.com/hashicorp/nomad/blob/7d5a9ecde95c18da94c9b6ace2565afbfdd6a40d/command/monitor.go#L175
	MonitorEvaluation(evaluationID string, ctx context.Context) error

	// WatchEventStream listens on the Nomad event stream for allocation and evaluation events.
	// Depending on the incoming event, any of the given function is executed.
	// Do not run multiple times simultaneously.
	WatchEventStream(ctx context.Context, callbacks *AllocationProcessing) error

	// ExecuteCommand executes the given command in the job/runner with the given id.
	// It writes the output of the command to stdout/stderr and reads input from stdin.
	// If tty is true, the command will run with a tty.
	// Iff privilegedExecution is true, the command will be executed privileged.
	// The command is passed in the shell form (not the exec array form) and will be executed in a shell.
	ExecuteCommand(jobID string, ctx context.Context, command string, tty bool, privilegedExecution bool,
		stdin io.Reader, stdout, stderr io.Writer) (int, error)

	// MarkRunnerAsUsed marks the runner with the given ID as used. It also stores the timeout duration in the metadata.
	MarkRunnerAsUsed(runnerID string, duration int) error
}

// APIClient implements the ExecutorAPI interface and can be used to perform different operations on the real
// Executor API and its return values.
type APIClient struct {
	apiQuerier
	evaluations map[string]chan error
	// allocations contain management data for all pending and running allocations.
	allocations storage.Storage[*allocationData]
	isListening bool
}

// NewExecutorAPI creates a new api client.
// One client is usually sufficient for the complete runtime of the API.
func NewExecutorAPI(nomadConfig *config.Nomad) (ExecutorAPI, error) {
	client := &APIClient{
		apiQuerier:  &nomadAPIClient{},
		evaluations: map[string]chan error{},
		allocations: storage.NewMonitoredLocalStorage[*allocationData](monitoring.MeasurementNomadAllocations,
			func(p *write.Point, object *allocationData, _ storage.EventType) {
				p.AddTag(monitoring.InfluxKeyJobID, object.jobID)
				p.AddTag(monitoring.InfluxKeyClientStatus, object.allocClientStatus)
				p.AddTag(monitoring.InfluxKeyNomadNode, object.allocNomadNode)
			}, 0, nil),
	}
	err := client.init(nomadConfig)
	return client, err
}

// init prepares an apiClient to be able to communicate to a provided Nomad API.
func (a *APIClient) init(nomadConfig *config.Nomad) error {
	if err := a.apiQuerier.init(nomadConfig); err != nil {
		return fmt.Errorf("error initializing API querier: %w", err)
	}
	return nil
}

func (a *APIClient) LoadRunnerIDs(prefix string) (runnerIDs []string, err error) {
	list, err := a.listJobs(prefix)
	if err != nil {
		return nil, err
	}
	for _, jobListStub := range list {
		// Filter out dead ("complete", "failed" or "lost") jobs
		if jobListStub.Status != structs.JobStatusDead {
			runnerIDs = append(runnerIDs, jobListStub.ID)
		}
	}
	return runnerIDs, nil
}

func (a *APIClient) LoadRunnerPortMappings(runnerID string) ([]nomadApi.PortMapping, error) {
	alloc, err := a.apiQuerier.allocation(runnerID)
	if err != nil {
		return nil, fmt.Errorf("error querying allocation for runner %s: %w", runnerID, err)
	}
	if alloc.AllocatedResources == nil {
		return nil, ErrorNoAllocatedResourcesFound
	}
	return alloc.AllocatedResources.Shared.Ports, nil
}

func (a *APIClient) LoadRunnerJobs(environmentID dto.EnvironmentID) ([]*nomadApi.Job, error) {
	go a.initializeAllocations(environmentID)

	runnerIDs, err := a.LoadRunnerIDs(RunnerJobID(environmentID, ""))
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

func (a *APIClient) MonitorEvaluation(evaluationID string, ctx context.Context) (err error) {
	a.evaluations[evaluationID] = make(chan error, 1)
	defer delete(a.evaluations, evaluationID)

	if !a.isListening {
		var cancel context.CancelFunc
		ctx, cancel = context.WithCancel(ctx)
		defer cancel() // cancel the WatchEventStream when the evaluation result was read.

		go func() {
			err = a.WatchEventStream(ctx, &AllocationProcessing{
				OnNew:     func(_ *nomadApi.Allocation, _ time.Duration) {},
				OnDeleted: func(_ string) bool { return false },
			})
			cancel() // cancel the waiting for an evaluation result if watching the event stream ends.
		}()
	}

	select {
	case <-ctx.Done():
		return err
	case err := <-a.evaluations[evaluationID]:
		// At the moment we expect only one error to be sent via this channel.
		return err
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
			return false, handleAllocationEvent(startTime, a.allocations, event, callbacks)
		default:
			return false, nil
		}
	}

	a.isListening = true
	err = receiveAndHandleNomadAPIEvents(stream, handler)
	a.isListening = false
	return err
}

func dumpNomadEventToInflux(event *nomadApi.Event) {
	p := influxdb2.NewPointWithMeasurement(monitoring.MeasurementNomadEvents)
	p.AddTag("topic", event.Topic.String())
	p.AddTag("type", event.Type)
	p.AddTag("key", event.Key)
	p.AddField("payload", event.Payload)
	p.AddTag("timestamp", time.Now().Format("03:04:05.000000000"))
	monitoring.WriteInfluxPoint(p)
}

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
func handleEvaluationEvent(evaluations map[string]chan error, event *nomadApi.Event) error {
	eval, err := event.Evaluation()
	if err != nil {
		return fmt.Errorf("failed to monitor evaluation: %w", err)
	}
	switch eval.Status {
	case structs.EvalStatusComplete, structs.EvalStatusCancelled, structs.EvalStatusFailed:
		resultChannel, ok := evaluations[eval.ID]
		if ok {
			select {
			case resultChannel <- checkEvaluation(eval):
				close(resultChannel)
			case <-time.After(resultChannelWriteTimeout):
				log.WithField("eval", eval).Error("Full evaluation channel")
			}
		}
	}
	return nil
}

// handleAllocationEvent is an event handler that processes allocation events.
// If a new allocation is received, onNewAllocation is called. If an allocation is deleted, onDeletedAllocation
// is called. The allocations storage is used to track pending and running allocations. Using the
// storage the state is persisted between multiple calls of this function.
func handleAllocationEvent(startTime int64, allocations storage.Storage[*allocationData],
	event *nomadApi.Event, callbacks *AllocationProcessing) error {
	alloc, err := event.Allocation()
	if err != nil {
		return fmt.Errorf("failed to retrieve allocation from event: %w", err)
	} else if alloc == nil {
		return nil
	}

	log.WithField("alloc_id", alloc.ID).
		WithField("ClientStatus", alloc.ClientStatus).
		WithField("DesiredStatus", alloc.DesiredStatus).
		WithField("PrevAllocation", alloc.PreviousAllocation).
		WithField("NextAllocation", alloc.NextAllocation).
		Debug("Handle Allocation Event")

	// When starting the API and listening on the Nomad event stream we might get events that already
	// happened from Nomad as it seems to buffer them for a certain duration.
	// Ignore old events here.
	if alloc.ModifyTime < startTime {
		return nil
	}

	if valid := filterDuplicateEvents(alloc, allocations); !valid {
		return nil
	}

	allocData := updateAllocationData(alloc, allocations)

	switch alloc.ClientStatus {
	case structs.AllocClientStatusPending:
		handlePendingAllocationEvent(alloc, allocData, allocations, callbacks)
	case structs.AllocClientStatusRunning:
		handleRunningAllocationEvent(alloc, allocData, allocations, callbacks)
	case structs.AllocClientStatusComplete:
		handleCompleteAllocationEvent(alloc, allocData, allocations, callbacks)
	case structs.AllocClientStatusFailed:
		handleFailedAllocationEvent(alloc, allocData, allocations, callbacks)
	case structs.AllocClientStatusLost:
		handleLostAllocationEvent(alloc, allocData, allocations, callbacks)
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
		log.WithField("alloc", alloc).Trace("Ignoring unknown allocation")
		return false
	case alloc.ClientStatus == allocData.allocClientStatus && alloc.DesiredStatus == allocData.allocDesiredStatus:
		log.WithField("alloc", alloc).Trace("Ignoring duplicate event")
		return false
	default:
		return true
	}
}

// updateAllocationData updates the allocation tracking data according to the passed alloc.
// The allocation data before this allocation update is returned.
func updateAllocationData(
	alloc *nomadApi.Allocation, allocations storage.Storage[*allocationData]) (previous *allocationData) {
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
func handlePendingAllocationEvent(alloc *nomadApi.Allocation, allocData *allocationData,
	allocations storage.Storage[*allocationData], callbacks *AllocationProcessing) {
	switch alloc.DesiredStatus {
	case structs.AllocDesiredStatusRun:
		if allocData != nil {
			// Handle Allocation restart.
			callbacks.OnDeleted(alloc.JobID)
		} else if alloc.PreviousAllocation != "" {
			// Handle Runner (/Container) re-allocations.
			if prevData, ok := allocations.Get(alloc.PreviousAllocation); ok {
				if removedByPoseidon := callbacks.OnDeleted(prevData.jobID); removedByPoseidon {
					// This case handles a race condition between the overdue runner inactivity timeout and the rescheduling of a
					// lost allocation. The race condition leads first to the rescheduling of the runner, but right after to it
					// being stopped. Instead of reporting an unexpected stop of the pending runner, we just not start tracking it.
					return
				}
				// Improve: When returning in the step before the allocation data will never be removed (data leak).
				// But it also shouldn't be removed as the current event could be received multiple times.
				// If we removed the allocation data the previous case handling wouldn't be triggered for the replicated event.
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
		})
	case structs.AllocDesiredStatusStop:
		handleStoppingAllocationEvent(alloc, allocations, callbacks, false)
	default:
		log.WithField("alloc", alloc).Warn("Other Desired Status")
	}
}

// handleRunningAllocationEvent calls the passed AllocationProcessor filtering similar events.
func handleRunningAllocationEvent(alloc *nomadApi.Allocation, allocData *allocationData,
	_ storage.Storage[*allocationData], callbacks *AllocationProcessing) {
	switch alloc.DesiredStatus {
	case structs.AllocDesiredStatusRun:
		startupDuration := time.Since(allocData.start)
		callbacks.OnNew(alloc, startupDuration)
	case structs.AllocDesiredStatusStop:
		// It is normal that running allocations will stop. We will handle it when it is stopped.
	default:
		log.WithField("alloc", alloc).Warn("Other Desired Status")
	}
}

// handleCompleteAllocationEvent handles allocations that stopped.
func handleCompleteAllocationEvent(alloc *nomadApi.Allocation, _ *allocationData,
	allocations storage.Storage[*allocationData], callbacks *AllocationProcessing) {
	switch alloc.DesiredStatus {
	case structs.AllocDesiredStatusRun:
		log.WithField("alloc", alloc).Warn("Complete allocation desires to run")
	case structs.AllocDesiredStatusStop:
		callbacks.OnDeleted(alloc.JobID)
		allocations.Delete(alloc.ID)
	default:
		log.WithField("alloc", alloc).Warn("Other Desired Status")
	}
}

// handleFailedAllocationEvent logs only the last of the multiple failure events.
func handleFailedAllocationEvent(alloc *nomadApi.Allocation, allocData *allocationData,
	allocations storage.Storage[*allocationData], callbacks *AllocationProcessing) {
	// The stop is expected when the allocation desired to stop even before it failed.
	expectedStop := allocData.allocDesiredStatus == structs.AllocDesiredStatusStop
	handleStoppingAllocationEvent(alloc, allocations, callbacks, expectedStop)
}

// handleLostAllocationEvent logs only the last of the multiple lost events.
func handleLostAllocationEvent(alloc *nomadApi.Allocation, allocData *allocationData,
	allocations storage.Storage[*allocationData], callbacks *AllocationProcessing) {
	// The stop is expected when the allocation desired to stop even before it got lost.
	expectedStop := allocData.allocDesiredStatus == structs.AllocDesiredStatusStop
	handleStoppingAllocationEvent(alloc, allocations, callbacks, expectedStop)
}

func handleStoppingAllocationEvent(alloc *nomadApi.Allocation, allocations storage.Storage[*allocationData],
	callbacks *AllocationProcessing, expectedStop bool) {
	removedByPoseidon := false
	if alloc.NextAllocation == "" {
		removedByPoseidon = callbacks.OnDeleted(alloc.JobID)
		allocations.Delete(alloc.ID)
	}

	entry := log.WithField("job", alloc.JobID)
	replacementAllocationScheduled := alloc.NextAllocation != ""
	if !removedByPoseidon && expectedStop == replacementAllocationScheduled {
		entry.WithField("alloc", alloc).Warn("Unexpected Allocation Stopping / Restarting")
	} else {
		entry.Debug("Expected Allocation Stopping / Restarting")
	}
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

func (a *APIClient) MarkRunnerAsUsed(runnerID string, duration int) error {
	job, err := a.job(runnerID)
	if err != nil {
		return fmt.Errorf("couldn't retrieve job info: %w", err)
	}
	configTaskGroup := FindAndValidateConfigTaskGroup(job)
	configTaskGroup.Meta[ConfigMetaUsedKey] = ConfigMetaUsedValue
	configTaskGroup.Meta[ConfigMetaTimeoutKey] = strconv.Itoa(duration)

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

// ExecuteCommand executes the given command in the given job.
// If tty is true, Nomad would normally write stdout and stderr of the command
// both on the stdout stream. However, if the InteractiveStderr server config option is true,
// we make sure that stdout and stderr are split correctly.
func (a *APIClient) ExecuteCommand(jobID string,
	ctx context.Context, command string, tty bool, privilegedExecution bool,
	stdin io.Reader, stdout, stderr io.Writer) (int, error) {
	if tty && config.Config.Server.InteractiveStderr {
		return a.executeCommandInteractivelyWithStderr(jobID, ctx, command, privilegedExecution, stdin, stdout, stderr)
	}
	command = prepareCommandWithoutTTY(command, privilegedExecution)
	exitCode, err := a.apiQuerier.Execute(jobID, ctx, command, tty, stdin, stdout, stderr)
	if err != nil {
		return 1, fmt.Errorf("error executing command in job %s: %w", jobID, err)
	}
	return exitCode, nil
}

// executeCommandInteractivelyWithStderr executes the given command interactively and splits stdout
// and stderr correctly. Normally, using Nomad to execute a command with tty=true (in order to have
// an interactive connection and possibly a fully working shell), would result in stdout and stderr
// to be served both over stdout. This function circumvents this by creating a fifo for the stderr
// of the command and starting a second execution that reads the stderr from that fifo.
func (a *APIClient) executeCommandInteractivelyWithStderr(allocationID string, ctx context.Context,
	command string, privilegedExecution bool, stdin io.Reader, stdout, stderr io.Writer) (int, error) {
	// Use current nano time to make the stderr fifo kind of unique.
	currentNanoTime := time.Now().UnixNano()

	stderrExitChan := make(chan int)
	go func() {
		readingContext, cancel := context.WithCancel(ctx)
		defer cancel()

		// Catch stderr in separate execution.
		logging.StartSpan("nomad.execute.stderr", "Execution for separate StdErr", ctx, func(ctx context.Context) {
			exit, err := a.Execute(allocationID, ctx, prepareCommandTTYStdErr(currentNanoTime, privilegedExecution), true,
				nullio.Reader{Ctx: readingContext}, stderr, io.Discard)
			if err != nil {
				log.WithContext(ctx).WithError(err).WithField("runner", allocationID).Warn("Stderr task finished with error")
			}
			stderrExitChan <- exit
		})
	}()

	command = prepareCommandTTY(command, currentNanoTime, privilegedExecution)
	var exit int
	var err error
	logging.StartSpan("nomad.execute.tty", "Interactive Execution", ctx, func(ctx context.Context) {
		exit, err = a.Execute(allocationID, ctx, command, true, stdin, stdout, io.Discard)
	})

	// Wait until the stderr catch command finished to make sure we receive all output.
	<-stderrExitChan
	return exit, err
}

const (
	// unsetEnvironmentVariablesFormat prepends the call to unset the passed variables before the actual command.
	unsetEnvironmentVariablesFormat = "unset %s && %s"
	// unsetEnvironmentVariablesPrefix is the prefix of all environment variables that will be filtered.
	unsetEnvironmentVariablesPrefix = "NOMAD_"
	// unsetEnvironmentVariablesShell is the shell functionality to get all environment variables starting with the prefix.
	unsetEnvironmentVariablesShell = "${!" + unsetEnvironmentVariablesPrefix + "@}"

	// stderrFifoFormat represents the format we use for our stderr fifos. The %d should be unique for the execution
	// as otherwise multiple executions are not possible.
	// Example: "/tmp/stderr_1623330777825234133.fifo".
	stderrFifoFormat = "/tmp/stderr_%d.fifo"
	// stderrFifoCommandFormat, if executed, is supposed to create a fifo, read from it and remove it in the end.
	// Example: "mkfifo my.fifo && (cat my.fifo; rm my.fifo)".
	stderrFifoCommandFormat = "mkfifo %s && (cat %s; rm %s)"
	// stderrWrapperCommandFormat, if executed, is supposed to wait until a fifo exists (it sleeps 10ms to reduce load
	// cause by busy waiting on the system). Once the fifo exists, the given command is executed and its stderr
	// redirected to the fifo.
	// Example: "until [ -e my.fifo ]; do sleep 0.01; done; (echo \"my.fifo exists\") 2> my.fifo".
	stderrWrapperCommandFormat = "until [ -e %s ]; do sleep 0.01; done; (%s) 2> %s"

	// setUserBinaryPath is due to Poseidon requires the setuser script for Nomad environments.
	setUserBinaryPath = "/sbin/setuser"
	// setUserBinaryUser is the user that is used and required by Poseidon for Nomad environments.
	setUserBinaryUser = "user"
	// PrivilegedExecution is to indicate the privileged execution of the passed command.
	PrivilegedExecution = true
	// UnprivilegedExecution is to indicate the unprivileged execution of the passed command.
	UnprivilegedExecution = false
)

func prepareCommandWithoutTTY(command string, privilegedExecution bool) string {
	const commandFieldAfterEnv = 4 // instead of "env CODEOCEAN=true /bin/bash -c sleep infinity" just "sleep infinity".
	command = setInnerDebugMessages(command, commandFieldAfterEnv, -1)

	command = setUserCommand(command, privilegedExecution)
	command = unsetEnvironmentVariables(command)
	return command
}

func prepareCommandTTY(command string, currentNanoTime int64, privilegedExecution bool) string {
	const commandFieldAfterSettingEnvVariables = 4
	command = setInnerDebugMessages(command, commandFieldAfterSettingEnvVariables, -1)

	// Take the command to be executed and wrap it to redirect stderr.
	stderrFifoPath := stderrFifo(currentNanoTime)
	command = fmt.Sprintf(stderrWrapperCommandFormat, stderrFifoPath, command, stderrFifoPath)

	command = setUserCommand(command, privilegedExecution)
	command = unsetEnvironmentVariables(command)
	return command
}

func prepareCommandTTYStdErr(currentNanoTime int64, privilegedExecution bool) string {
	stderrFifoPath := stderrFifo(currentNanoTime)
	command := fmt.Sprintf(stderrFifoCommandFormat, stderrFifoPath, stderrFifoPath, stderrFifoPath)
	command = setInnerDebugMessages(command, 0, 1)
	command = setUserCommand(command, privilegedExecution)
	return command
}

func stderrFifo(id int64) string {
	return fmt.Sprintf(stderrFifoFormat, id)
}

func unsetEnvironmentVariables(command string) string {
	command = dto.WrapBashCommand(command)
	command = fmt.Sprintf(unsetEnvironmentVariablesFormat, unsetEnvironmentVariablesShell, command)

	// Debug Message
	const commandFieldBeforeBash = 2 // e.g. instead of "unset ${!NOMAD_@} && /bin/bash -c [...]" just "unset ${!NOMAD_@}".
	command = injectStartDebugMessage(command, 0, commandFieldBeforeBash)
	return command
}

// setUserCommand prefixes the passed command with the setUser command.
func setUserCommand(command string, privilegedExecution bool) string {
	// Wrap the inner command first so that the setUserBinary applies to the whole inner command.
	command = dto.WrapBashCommand(command)

	if !privilegedExecution {
		command = fmt.Sprintf("%s %s %s", setUserBinaryPath, setUserBinaryUser, command)
	}

	// Debug Message
	const commandFieldBeforeBash = 2 // e.g. instead of "/sbin/setuser user /bin/bash -c [...]" just "/sbin/setuser user".
	command = injectStartDebugMessage(command, 0, commandFieldBeforeBash)
	return command
}

func injectStartDebugMessage(command string, start uint, end int) string {
	commandFields := strings.Fields(command)
	if start < uint(len(commandFields)) {
		commandFields = commandFields[start:]
		end -= int(start)
	}
	if end >= 0 && end < len(commandFields) {
		commandFields = commandFields[:end]
	}

	description := strings.Join(commandFields, " ")
	if strings.HasPrefix(description, "\"") && strings.HasSuffix(description, "\"") {
		description = description[1 : len(description)-1]
	}

	description = dto.BashEscapeCommand(description)
	description = description[1 : len(description)-1] // The most outer quotes are not escaped!
	return fmt.Sprintf(timeDebugMessageFormatStart, description, command)
}

// setInnerDebugMessages injects debug commands into the bash command.
// The debug messages are parsed by the SentryDebugWriter.
func setInnerDebugMessages(command string, descriptionStart uint, descriptionEnd int) (result string) {
	result = injectStartDebugMessage(command, descriptionStart, descriptionEnd)

	result = strings.TrimSuffix(result, ";")
	return fmt.Sprintf(timeDebugMessageFormatEnd, result, "exit $ec")
}
