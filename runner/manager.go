package runner

import (
	"context"
	"errors"
	"fmt"
	"github.com/google/uuid"
	nomadApi "github.com/hashicorp/nomad/api"
	"github.com/hashicorp/nomad/nomad/structs"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/logging"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/nomad"
	"strconv"
	"strings"
	"time"
)

var (
	log                            = logging.GetLogger("runner")
	ErrUnknownExecutionEnvironment = errors.New("execution environment not found")
	ErrNoRunnersAvailable          = errors.New("no runners available for this execution environment")
	ErrRunnerNotFound              = errors.New("no runner found with this id")
)

type EnvironmentID int

func (e EnvironmentID) toString() string {
	return strconv.Itoa(int(e))
}

type NomadJobID string

// Manager keeps track of the used and unused runners of all execution environments in order to provide unused
// runners to new clients and ensure no runner is used twice.
type Manager interface {
	// CreateOrUpdateEnvironment creates the given environment if it does not exist. Otherwise, it updates
	// the existing environment and all runners.
	CreateOrUpdateEnvironment(id EnvironmentID, desiredIdleRunnersCount uint) (bool, error)

	// Claim returns a new runner.
	// It makes sure that the runner is not in use yet and returns an error if no runner could be provided.
	Claim(id EnvironmentID) (Runner, error)

	// Get returns the used runner with the given runnerId.
	// If no runner with the given runnerId is currently used, it returns an error.
	Get(runnerID string) (Runner, error)

	// Return signals that the runner is no longer used by the caller and can be claimed by someone else.
	// The runner is deleted or cleaned up for reuse depending on the used executor.
	Return(r Runner) error
}

type NomadRunnerManager struct {
	apiClient    nomad.ExecutorAPI
	environments NomadEnvironmentStorage
	usedRunners  Storage
}

// NewNomadRunnerManager creates a new runner manager that keeps track of all runners.
// It uses the apiClient for all requests and runs a background task to keep the runners in sync with Nomad.
// If you cancel the context the background synchronization will be stopped.
func NewNomadRunnerManager(apiClient nomad.ExecutorAPI, ctx context.Context) (*NomadRunnerManager, error) {
	m := &NomadRunnerManager{
		apiClient,
		NewLocalNomadJobStorage(),
		NewLocalRunnerStorage(),
	}
	err := m.loadExistingEnvironments()
	if err != nil {
		return nil, err
	}
	go m.updateRunners(ctx)
	return m, nil
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

func (m *NomadRunnerManager) CreateOrUpdateEnvironment(id EnvironmentID, desiredIdleRunnersCount uint) (bool, error) {
	_, ok := m.environments.Get(id)
	if !ok {
		return true, m.registerEnvironment(id, desiredIdleRunnersCount)
	}
	return false, m.updateEnvironment(id, desiredIdleRunnersCount)
}

func (m *NomadRunnerManager) registerEnvironment(environmentID EnvironmentID, desiredIdleRunnersCount uint) error {
	templateJob, err := m.apiClient.LoadTemplateJob(environmentID.toString())
	if err != nil {
		return fmt.Errorf("couldn't register environment: %w", err)
	}

	m.environments.Add(&NomadEnvironment{
		environmentID,
		NewLocalRunnerStorage(),
		desiredIdleRunnersCount,
		templateJob,
	})
	err = m.scaleEnvironment(environmentID)
	if err != nil {
		return fmt.Errorf("couldn't upscale environment %w", err)
	}
	return nil
}

func (m *NomadRunnerManager) updateEnvironment(id EnvironmentID, desiredIdleRunnersCount uint) error {
	environment, ok := m.environments.Get(id)
	if !ok {
		return ErrUnknownExecutionEnvironment
	}
	environment.desiredIdleRunnersCount = desiredIdleRunnersCount

	templateJob, err := m.apiClient.LoadTemplateJob(id.toString())
	if err != nil {
		return fmt.Errorf("update environment couldn't load template job: %w", err)
	}
	environment.templateJob = templateJob

	runners, err := m.apiClient.LoadRunners(id.toString())
	if err != nil {
		return fmt.Errorf("update environment couldn't load runners: %w", err)
	}
	var occurredErrors []string
	for _, id := range runners {
		// avoid taking the address of the loop variable
		runnerID := id
		updatedRunnerJob := *environment.templateJob
		updatedRunnerJob.ID = &runnerID
		updatedRunnerJob.Name = &runnerID
		_, err := m.apiClient.RegisterNomadJob(&updatedRunnerJob)
		if err != nil {
			occurredErrors = append(occurredErrors, err.Error())
		}
	}
	if len(occurredErrors) > 0 {
		errorResult := strings.Join(occurredErrors, "\n")
		return fmt.Errorf("%d errors occurred when updating environment: %s", len(occurredErrors), errorResult)
	}
	return m.scaleEnvironment(id)
}

func (m *NomadRunnerManager) Claim(environmentID EnvironmentID) (Runner, error) {
	job, ok := m.environments.Get(environmentID)
	if !ok {
		return nil, ErrUnknownExecutionEnvironment
	}
	runner, ok := job.idleRunners.Sample()
	if !ok {
		return nil, ErrNoRunnersAvailable
	}
	m.usedRunners.Add(runner)
	err := m.apiClient.MarkRunnerAsUsed(runner.Id())
	if err != nil {
		return nil, fmt.Errorf("can't mark runner as used: %w", err)
	}

	err = m.scaleEnvironment(environmentID)
	if err != nil {
		log.WithError(err).WithField("environmentID", environmentID).Error("Couldn't scale environment")
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
	err = m.apiClient.DeleteRunner(r.Id())
	if err != nil {
		return
	}
	m.usedRunners.Delete(r.Id())
	return
}

func (m *NomadRunnerManager) updateRunners(ctx context.Context) {
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

	if nomad.IsDefaultJobID(alloc.JobID) {
		return
	}

	environmentID := nomad.EnvironmentIDFromJobID(alloc.JobID)
	intEnvironmentID, err := strconv.Atoi(environmentID)
	if err != nil {
		return
	}

	job, ok := m.environments.Get(EnvironmentID(intEnvironmentID))
	if ok {
		job.idleRunners.Add(NewNomadJob(alloc.JobID, m.apiClient))
	}
}

func (m *NomadRunnerManager) onAllocationStopped(alloc *nomadApi.Allocation) {
	log.WithField("id", alloc.JobID).Debug("Runner stopped")

	environmentID := nomad.EnvironmentIDFromJobID(alloc.JobID)
	intEnvironmentID, err := strconv.Atoi(environmentID)
	if err != nil {
		return
	}

	m.usedRunners.Delete(alloc.JobID)
	job, ok := m.environments.Get(EnvironmentID(intEnvironmentID))
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

	log.WithField("required", required).Debug("Scaling environment")

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
	newRunnerID := nomad.RunnerJobID(environment.ID().toString(), newUUID.String())

	template := *environment.templateJob
	template.ID = &newRunnerID
	template.Name = &newRunnerID

	evalID, err := m.apiClient.RegisterNomadJob(&template)
	if err != nil {
		return fmt.Errorf("couldn't register Nomad job: %w", err)
	}
	err = m.apiClient.MonitorEvaluation(evalID, context.Background())
	if err != nil {
		return fmt.Errorf("couldn't monitor evaluation: %w", err)
	}
	return nil
}

func (m *NomadRunnerManager) unusedRunners(
	environmentID EnvironmentID, fetchedRunnerIds []string) (newRunners []Runner) {
	newRunners = make([]Runner, 0)
	job, ok := m.environments.Get(environmentID)
	if !ok {
		// the environment does not exist, so it won't have any unused runners
		return
	}
	for _, runnerID := range fetchedRunnerIds {
		_, ok := m.usedRunners.Get(runnerID)
		if !ok {
			_, ok = job.idleRunners.Get(runnerID)
			if !ok {
				newRunners = append(newRunners, NewNomadJob(runnerID, m.apiClient))
			}
		}
	}
	return newRunners
}

func (m *NomadRunnerManager) loadExistingEnvironments() error {
	jobs, err := m.apiClient.LoadAllJobs()
	if err != nil {
		return fmt.Errorf("can't load template jobs: %w", err)
	}

	for _, job := range jobs {
		m.loadExistingJob(job)
	}

	for _, environmentID := range m.environments.List() {
		err := m.scaleEnvironment(environmentID)
		if err != nil {
			return fmt.Errorf("can not scale up: %w", err)
		}
	}

	return nil
}

func (m *NomadRunnerManager) loadExistingJob(job *nomadApi.Job) {
	if *job.Status != structs.JobStatusRunning {
		return
	}

	jobLogger := log.WithField("jobID", *job.ID)

	configTaskGroup := nomad.FindConfigTaskGroup(job)
	if configTaskGroup == nil {
		jobLogger.Info("Couldn't find config task group in job, skipping ...")
		return
	}

	if configTaskGroup.Meta[nomad.ConfigMetaUsedKey] == nomad.ConfigMetaUsedValue {
		m.usedRunners.Add(NewNomadJob(*job.ID, m.apiClient))
		jobLogger.Info("Added job to usedRunners")
		return
	}

	environmentID := configTaskGroup.Meta[nomad.ConfigMetaEnvironmentKey]
	environmentIDInt, err := strconv.Atoi(environmentID)
	if err != nil {
		jobLogger.WithField("environmentID", environmentID).
			WithError(err).
			Error("Couldn't convert environment id of template job to int")
		return
	}

	environment, ok := m.environments.Get(EnvironmentID(environmentIDInt))
	if !ok {
		desiredIdleRunnersCount, err := strconv.Atoi(configTaskGroup.Meta[nomad.ConfigMetaPoolSizeKey])
		if err != nil {
			jobLogger.WithError(err).Error("Couldn't convert pool size to int")
			return
		}

		environment = &NomadEnvironment{
			environmentID:           EnvironmentID(environmentIDInt),
			idleRunners:             NewLocalRunnerStorage(),
			desiredIdleRunnersCount: uint(desiredIdleRunnersCount),
		}
		m.environments.Add(environment)
		log.WithField("environmentID", environment.environmentID).Info("Added existing environment")
	}

	if nomad.IsDefaultJobID(*job.ID) {
		environment.templateJob = job
	} else {
		log.WithField("jobID", *job.ID).
			WithField("environmentID", environment.environmentID).
			Info("Added idle runner")
		environment.idleRunners.Add(NewNomadJob(*job.ID, m.apiClient))
	}
}
