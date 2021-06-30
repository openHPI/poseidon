package runner

import (
	"context"
	"errors"
	"fmt"
	"github.com/google/uuid"
	nomadApi "github.com/hashicorp/nomad/api"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/logging"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/nomad"
	"strconv"
	"strings"
	"time"
)

var (
	log                               = logging.GetLogger("runner")
	ErrUnknownExecutionEnvironment    = errors.New("execution environment not found")
	ErrNoRunnersAvailable             = errors.New("no runners available for this execution environment")
	ErrRunnerNotFound                 = errors.New("no runner found with this id")
	ErrorUpdatingExecutionEnvironment = errors.New("errors occurred when updating environment")
	ErrorInvalidJobID                 = errors.New("invalid job id")
)

type EnvironmentID int

func NewEnvironmentID(id string) (EnvironmentID, error) {
	environment, err := strconv.Atoi(id)
	return EnvironmentID(environment), err
}

func (e EnvironmentID) toString() string {
	return strconv.Itoa(int(e))
}

type NomadJobID string

// Manager keeps track of the used and unused runners of all execution environments in order to provide unused
// runners to new clients and ensure no runner is used twice.
type Manager interface {
	// CreateOrUpdateEnvironment creates the given environment if it does not exist. Otherwise, it updates
	// the existing environment and all runners. Iff a new Environment has been created, it returns true.
	// Iff scale is true, runners are created until the desiredIdleRunnersCount is reached.
	CreateOrUpdateEnvironment(id EnvironmentID, desiredIdleRunnersCount uint, templateJob *nomadApi.Job,
		scale bool) (bool, error)

	// Claim returns a new runner. The runner is deleted after duration seconds if duration is not 0.
	// It makes sure that the runner is not in use yet and returns an error if no runner could be provided.
	Claim(id EnvironmentID, duration int) (Runner, error)

	// Get returns the used runner with the given runnerId.
	// If no runner with the given runnerId is currently used, it returns an error.
	Get(runnerID string) (Runner, error)

	// Return signals that the runner is no longer used by the caller and can be claimed by someone else.
	// The runner is deleted or cleaned up for reuse depending on the used executor.
	Return(r Runner) error

	// Load fetches all already created runners from the executor and registers them.
	// It should be called during the startup process (e.g. on creation of the Manager).
	Load()
}

type NomadRunnerManager struct {
	apiClient    nomad.ExecutorAPI
	environments NomadEnvironmentStorage
	usedRunners  Storage
}

// NewNomadRunnerManager creates a new runner manager that keeps track of all runners.
// It uses the apiClient for all requests and runs a background task to keep the runners in sync with Nomad.
// If you cancel the context the background synchronization will be stopped.
func NewNomadRunnerManager(apiClient nomad.ExecutorAPI, ctx context.Context) *NomadRunnerManager {
	m := &NomadRunnerManager{
		apiClient,
		NewLocalNomadEnvironmentStorage(),
		NewLocalRunnerStorage(),
	}
	go m.keepRunnersSynced(ctx)
	return m
}

type NomadEnvironment struct {
	environmentID           EnvironmentID
	idleRunners             Storage
	desiredIdleRunnersCount uint
	templateJob             *nomadApi.Job
}

func (j *NomadEnvironment) ID() EnvironmentID {
	return j.environmentID
}

func (m *NomadRunnerManager) CreateOrUpdateEnvironment(id EnvironmentID, desiredIdleRunnersCount uint,
	templateJob *nomadApi.Job, scale bool) (bool, error) {
	_, ok := m.environments.Get(id)
	if !ok {
		return true, m.registerEnvironment(id, desiredIdleRunnersCount, templateJob, scale)
	}
	return false, m.updateEnvironment(id, desiredIdleRunnersCount, templateJob, scale)
}

func (m *NomadRunnerManager) registerEnvironment(environmentID EnvironmentID, desiredIdleRunnersCount uint,
	templateJob *nomadApi.Job, scale bool) error {
	m.environments.Add(&NomadEnvironment{
		environmentID,
		NewLocalRunnerStorage(),
		desiredIdleRunnersCount,
		templateJob,
	})
	if scale {
		err := m.scaleEnvironment(environmentID)
		if err != nil {
			return fmt.Errorf("couldn't upscale environment %w", err)
		}
	}
	return nil
}

// updateEnvironment updates all runners of the specified environment. This is required as attributes like the
// CPULimit or MemoryMB could be changed in the new template job.
func (m *NomadRunnerManager) updateEnvironment(id EnvironmentID, desiredIdleRunnersCount uint,
	newTemplateJob *nomadApi.Job, scale bool) error {
	environment, ok := m.environments.Get(id)
	if !ok {
		return ErrUnknownExecutionEnvironment
	}
	environment.desiredIdleRunnersCount = desiredIdleRunnersCount
	environment.templateJob = newTemplateJob
	err := nomad.SetMetaConfigValue(newTemplateJob, nomad.ConfigMetaPoolSizeKey, strconv.Itoa(int(desiredIdleRunnersCount)))
	if err != nil {
		return fmt.Errorf("update environment couldn't update template environment: %w", err)
	}

	err = m.updateRunnerSpecs(id, newTemplateJob)
	if err != nil {
		return err
	}

	if scale {
		err = m.scaleEnvironment(id)
	}
	return err
}

func (m *NomadRunnerManager) updateRunnerSpecs(environmentID EnvironmentID, templateJob *nomadApi.Job) error {
	runners, err := m.apiClient.LoadRunnerIDs(environmentID.toString())
	if err != nil {
		return fmt.Errorf("update environment couldn't load runners: %w", err)
	}

	var occurredError error
	for _, id := range runners {
		// avoid taking the address of the loop variable
		runnerID := id
		updatedRunnerJob := *templateJob
		updatedRunnerJob.ID = &runnerID
		updatedRunnerJob.Name = &runnerID
		err := m.apiClient.RegisterRunnerJob(&updatedRunnerJob)
		if err != nil {
			if occurredError == nil {
				occurredError = ErrorUpdatingExecutionEnvironment
			}
			occurredError = fmt.Errorf("%w; new api error for runner %s - %v", occurredError, id, err)
		}
	}
	return occurredError
}

func (m *NomadRunnerManager) Claim(environmentID EnvironmentID, duration int) (Runner, error) {
	environment, ok := m.environments.Get(environmentID)
	if !ok {
		return nil, ErrUnknownExecutionEnvironment
	}
	runner, ok := environment.idleRunners.Sample()
	if !ok {
		return nil, ErrNoRunnersAvailable
	}
	m.usedRunners.Add(runner)
	err := m.apiClient.MarkRunnerAsUsed(runner.Id(), duration)
	if err != nil {
		return nil, fmt.Errorf("can't mark runner as used: %w", err)
	}

	runner.SetupTimeout(time.Duration(duration) * time.Second)

	err = m.createRunner(environment)
	if err != nil {
		log.WithError(err).WithField("environmentID", environmentID).Error("Couldn't create new runner for claimed one")
	}

	return runner, nil
}

func (m *NomadRunnerManager) Get(runnerID string) (Runner, error) {
	runner, ok := m.usedRunners.Get(runnerID)
	if !ok {
		return nil, ErrRunnerNotFound
	}
	return runner, nil
}

func (m *NomadRunnerManager) Return(r Runner) (err error) {
	r.StopTimeout()
	err = m.apiClient.DeleteRunner(r.Id())
	if err != nil {
		return
	}
	m.usedRunners.Delete(r.Id())
	return
}

func (m *NomadRunnerManager) Load() {
	for _, environment := range m.environments.List() {
		environmentLogger := log.WithField("environmentID", environment.ID())
		runnerJobs, err := m.apiClient.LoadRunnerJobs(environment.ID().toString())
		if err != nil {
			environmentLogger.WithError(err).Warn("Error fetching the runner jobs")
		}
		for _, job := range runnerJobs {
			configTaskGroup := nomad.FindConfigTaskGroup(job)
			if configTaskGroup == nil {
				environmentLogger.Infof("Couldn't find config task group in job %s, skipping ...", *job.ID)
				continue
			}
			isUsed := configTaskGroup.Meta[nomad.ConfigMetaUsedKey] == nomad.ConfigMetaUsedValue
			newJob := NewNomadJob(*job.ID, m.apiClient, m)
			if isUsed {
				m.usedRunners.Add(newJob)
				timeout, err := strconv.Atoi(configTaskGroup.Meta[nomad.ConfigMetaTimeoutKey])
				if err != nil {
					log.WithError(err).Warn("Error loading timeout from meta values")
				} else {
					newJob.SetupTimeout(time.Duration(timeout) * time.Second)
				}
			} else {
				environment.idleRunners.Add(newJob)
			}
		}
		err = m.scaleEnvironment(environment.ID())
		if err != nil {
			environmentLogger.Error("Couldn't scale environment")
		}
	}
}

func (m *NomadRunnerManager) keepRunnersSynced(ctx context.Context) {
	retries := 0
	for ctx.Err() == nil {
		err := m.apiClient.WatchAllocations(ctx, m.onAllocationAdded, m.onAllocationStopped)
		retries += 1
		log.WithError(err).Errorf("Stopped updating the runners! Retry %v", retries)
		<-time.After(time.Second)
	}
}

func (m *NomadRunnerManager) onAllocationAdded(alloc *nomadApi.Allocation) {
	log.WithField("id", alloc.JobID).Debug("Runner started")

	if IsEnvironmentTemplateID(alloc.JobID) {
		return
	}

	environmentID, err := EnvironmentIDFromJobID(alloc.JobID)
	if err != nil {
		log.WithError(err).Warn("Allocation could not be added")
		return
	}

	job, ok := m.environments.Get(environmentID)
	if ok {
		job.idleRunners.Add(NewNomadJob(alloc.JobID, m.apiClient, m))
	}
}

func (m *NomadRunnerManager) onAllocationStopped(alloc *nomadApi.Allocation) {
	log.WithField("id", alloc.JobID).Debug("Runner stopped")

	environmentID, err := EnvironmentIDFromJobID(alloc.JobID)
	if err != nil {
		log.WithError(err).Warn("Stopped allocation can not be handled")
		return
	}

	m.usedRunners.Delete(alloc.JobID)
	job, ok := m.environments.Get(environmentID)
	if ok {
		job.idleRunners.Delete(alloc.JobID)
	}
}

// scaleEnvironment makes sure that the amount of idle runners is at least the desiredIdleRunnersCount.
func (m *NomadRunnerManager) scaleEnvironment(id EnvironmentID) error {
	environment, ok := m.environments.Get(id)
	if !ok {
		return ErrUnknownExecutionEnvironment
	}

	required := int(environment.desiredIdleRunnersCount) - environment.idleRunners.Length()

	log.WithField("runnersRequired", required).WithField("id", id).Debug("Scaling environment")

	for i := 0; i < required; i++ {
		err := m.createRunner(environment)
		if err != nil {
			return fmt.Errorf("couldn't create new runner: %w", err)
		}
	}
	return nil
}

func (m *NomadRunnerManager) createRunner(environment *NomadEnvironment) error {
	newUUID, err := uuid.NewUUID()
	if err != nil {
		return fmt.Errorf("failed generating runner id")
	}
	newRunnerID := RunnerJobID(environment.ID(), newUUID.String())

	template := *environment.templateJob
	template.ID = &newRunnerID
	template.Name = &newRunnerID

	return m.apiClient.RegisterRunnerJob(&template)
}

// RunnerJobID returns the nomad job id of the runner with the given environment id and uuid.
func RunnerJobID(environmentID EnvironmentID, uuid string) string {
	return fmt.Sprintf("%d-%s", environmentID, uuid)
}

// EnvironmentIDFromJobID returns the environment id that is part of the passed job id.
func EnvironmentIDFromJobID(jobID string) (EnvironmentID, error) {
	parts := strings.Split(jobID, "-")
	if len(parts) == 0 {
		return 0, fmt.Errorf("empty job id: %w", ErrorInvalidJobID)
	}
	environmentID, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, fmt.Errorf("invalid environment id par %v: %w", err, ErrorInvalidJobID)
	}
	return EnvironmentID(environmentID), nil
}

// TemplateJobID returns the id of the template job for the environment with the given id.
func TemplateJobID(id EnvironmentID) string {
	return fmt.Sprintf("%s-%d", nomad.TemplateJobPrefix, id)
}

// IsEnvironmentTemplateID checks if the passed job id belongs to a template job.
func IsEnvironmentTemplateID(jobID string) bool {
	parts := strings.Split(jobID, "-")
	return len(parts) == 2 && parts[0] == nomad.TemplateJobPrefix
}

func EnvironmentIDFromTemplateJobID(id string) (string, error) {
	parts := strings.Split(id, "-")
	if len(parts) < 2 {
		return "", fmt.Errorf("invalid template job id: %w", ErrorInvalidJobID)
	}
	return parts[1], nil
}
