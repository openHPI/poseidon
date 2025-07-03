package nomad

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"

	nomadApi "github.com/hashicorp/nomad/api"
	"github.com/hashicorp/nomad/nomad/structs"
	"github.com/influxdata/influxdb-client-go/v2/api/write"
	"github.com/openHPI/poseidon/internal/config"
	"github.com/openHPI/poseidon/pkg/dto"
	"github.com/openHPI/poseidon/pkg/logging"
	"github.com/openHPI/poseidon/pkg/monitoring"
	"github.com/openHPI/poseidon/pkg/storage"
)

var (
	log                                                      = logging.GetLogger("nomad")
	ErrExecutorCommunicationFailed                           = errors.New("communication with executor failed")
	ErrEvaluation                                            = errors.New("evaluation could not complete")
	ErrPlacingAllocations                                    = errors.New("failed to place all allocations")
	ErrLoadingJob                                            = errors.New("failed to load job")
	ErrNoAllocatedResourcesFound                             = errors.New("no allocated resources found")
	ErrLocalDestruction                  RunnerDeletedReason = errors.New("the destruction should not cause external changes")
	ErrOOMKilled                         RunnerDeletedReason = fmt.Errorf("%s: %w", dto.ErrOOMKilled.Error(), ErrLocalDestruction)
	ErrAllocationRescheduled             RunnerDeletedReason = fmt.Errorf("the allocation was rescheduled: %w", ErrLocalDestruction)
	ErrAllocationStopped                 RunnerDeletedReason = errors.New("the allocation was stopped")
	ErrAllocationStoppedUnexpectedly     RunnerDeletedReason = fmt.Errorf("%w unexpectedly", ErrAllocationStopped)
	ErrAllocationRescheduledUnexpectedly RunnerDeletedReason = fmt.Errorf(
		"%w correctly but rescheduled", ErrAllocationStopped)
	// ErrAllocationCompleted is for reporting the reason for the stopped allocation.
	// We do not consider it as an error but add it anyway for a complete reporting.
	// It is a ErrLocalDestruction because another allocation might be replacing the allocation in the same job.
	ErrAllocationCompleted RunnerDeletedReason = fmt.Errorf("the allocation completed: %w", ErrLocalDestruction)
	ErrJobDeregistered     RunnerDeletedReason = fmt.Errorf("the job got deregistered: %w", ErrLocalDestruction)
)

type RunnerDeletedReason error

// ExecutorAPI provides access to a container orchestration solution.
type ExecutorAPI interface {
	apiQuerier

	// LoadEnvironmentJobs loads all environment jobs.
	LoadEnvironmentJobs() ([]*nomadApi.Job, error)

	// LoadRunnerJobs loads all runner jobs specific for the environment.
	LoadRunnerJobs(environmentID dto.EnvironmentID) ([]*nomadApi.Job, error)

	// LoadRunnerIDs returns the IDs of all runners with the specified id prefix which are not about to
	// get stopped.
	LoadRunnerIDs(prefix string) (runnerIDs []string, err error)

	// LoadRunnerPortMappings returns the mapped ports of the runner.
	LoadRunnerPortMappings(runnerID string) ([]nomadApi.PortMapping, error)

	// RegisterRunnerJob creates a runner job based on the template job.
	// It registers the job and waits until the registration completes.
	RegisterRunnerJob(template *nomadApi.Job) error

	// MonitorEvaluation monitors the given evaluation ID.
	// It waits until the evaluation reaches one of the states complete, canceled or failed.
	// If the evaluation was not successful, an error containing the failures is returned.
	// See also https://github.com/hashicorp/nomad/blob/7d5a9ecde95c18da94c9b6ace2565afbfdd6a40d/command/monitor.go#L175
	MonitorEvaluation(ctx context.Context, evaluationID string) error

	// WatchEventStream listens on the Nomad event stream for allocation and evaluation events.
	// Depending on the incoming event, any of the given function is executed.
	// Do not run multiple times simultaneously.
	WatchEventStream(ctx context.Context, callbacks *AllocationProcessing) error

	// ExecuteCommand executes the given command in the job/runner with the given id.
	// It writes the output of the command to stdout/stderr and reads input from stdin.
	// If tty is true, the command will run with a tty.
	// Iff privilegedExecution is true, the command will be executed privileged.
	// The command is passed in the shell form (not the exec array form) and will be executed in a shell.
	ExecuteCommand(ctx context.Context, jobID string, command string, tty bool, privilegedExecution bool,
		stdin io.Reader, stdout, stderr io.Writer) (int, error)

	// SetRunnerMetaUsed marks the runner with the given ID as used or unused.
	// If used, also the timeout duration is stored in the metadata.
	SetRunnerMetaUsed(runnerID string, used bool, duration int) error
}

// APIClient implements the ExecutorAPI interface and can be used to perform different operations on the real
// Executor API and its return values.
type APIClient struct {
	apiQuerier

	evaluations storage.Storage[chan error]
	// jobAllocationMapping maps a Job ID to the most recent Allocation ID.
	jobAllocationMapping storage.Storage[string]
	// allocations contain management data for all pending and running allocations.
	allocations storage.Storage[*allocationData]
	isListening bool
}

// NewExecutorAPI creates a new api client.
// One client is usually sufficient for the complete runtime of the API.
func NewExecutorAPI(ctx context.Context, nomadConfig *config.Nomad) (ExecutorAPI, error) {
	client := &APIClient{
		apiQuerier:  &nomadAPIClient{},
		evaluations: storage.NewLocalStorage[chan error](),
		jobAllocationMapping: storage.NewMonitoredLocalStorage[string](ctx, monitoring.MeasurementNomadJobs,
			func(p *write.Point, allocationID string, _ storage.EventType) {
				p.AddTag(monitoring.InfluxKeyAllocationID, allocationID)
			}, 0),
		allocations: storage.NewMonitoredLocalStorage[*allocationData](ctx, monitoring.MeasurementNomadAllocations,
			func(p *write.Point, object *allocationData, _ storage.EventType) {
				p.AddTag(monitoring.InfluxKeyJobID, object.jobID)
				p.AddTag(monitoring.InfluxKeyClientStatus, object.allocClientStatus)
				p.AddTag(monitoring.InfluxKeyNomadNode, object.allocNomadNode)
			}, 0),
	}
	err := client.init(nomadConfig)

	return client, err
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
	alloc, err := a.allocation(runnerID)
	if err != nil {
		return nil, fmt.Errorf("error querying allocation for runner %s: %w", runnerID, err)
	}

	if alloc.AllocatedResources == nil {
		return nil, ErrNoAllocatedResourcesFound
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
	for _, runnerID := range runnerIDs {
		job, err := a.job(runnerID)
		if err != nil {
			if occurredError == nil {
				occurredError = ErrLoadingJob
			}

			occurredError = fmt.Errorf("%w: couldn't load job info for runner %s - %w", occurredError, runnerID, err)

			continue
		}

		jobs = append(jobs, job)
	}

	return jobs, occurredError
}

func (a *APIClient) SetRunnerMetaUsed(runnerID string, used bool, duration int) error {
	newMetaUsedValue := ConfigMetaUsedValue
	if !used {
		newMetaUsedValue = ConfigMetaUnusedValue
	}

	newMetaTimeoutValue := strconv.Itoa(duration)

	job, err := a.job(runnerID)
	if err != nil {
		return fmt.Errorf("couldn't retrieve job info: %w", err)
	}

	configTaskGroup := FindAndValidateConfigTaskGroup(job)
	metaUsedDiffers := configTaskGroup.Meta[ConfigMetaUsedKey] != newMetaUsedValue
	metaTimeoutDiffers := configTaskGroup.Meta[ConfigMetaTimeoutKey] != newMetaTimeoutValue

	if metaUsedDiffers || (used && metaTimeoutDiffers) {
		configTaskGroup.Meta[ConfigMetaUsedKey] = newMetaUsedValue
		if used {
			configTaskGroup.Meta[ConfigMetaTimeoutKey] = newMetaTimeoutValue
		}

		_, err = a.RegisterNomadJob(job)
		if err != nil {
			return fmt.Errorf("couldn't update runner config: %w", err)
		}
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
		job, err := a.job(jobStub.ID)
		if err != nil {
			return []*nomadApi.Job{}, fmt.Errorf("couldn't load job info for job %v: %w", jobStub.ID, err)
		}

		jobs = append(jobs, job)
	}

	return jobs, nil
}

// init prepares an apiClient to be able to communicate to a provided Nomad API.
func (a *APIClient) init(nomadConfig *config.Nomad) error {
	if err := a.apiQuerier.init(nomadConfig); err != nil {
		return fmt.Errorf("error initializing API querier: %w", err)
	}

	return nil
}
