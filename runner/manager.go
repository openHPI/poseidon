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
	log                            = logging.GetLogger("runner")
	ErrUnknownExecutionEnvironment = errors.New("execution environment not found")
	ErrNoRunnersAvailable          = errors.New("no runners available for this execution environment")
	ErrRunnerNotFound              = errors.New("no runner found with this id")
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
	CreateOrUpdateEnvironment(id EnvironmentID, desiredIdleRunnersCount uint, teplateJob *nomadApi.Job) (bool, error)

	// Claim returns a new runner.
	// It makes sure that the runner is not in use yet and returns an error if no runner could be provided.
	Claim(id EnvironmentID) (Runner, error)

	// Get returns the used runner with the given runnerId.
	// If no runner with the given runnerId is currently used, it returns an error.
	Get(runnerID string) (Runner, error)

	// Return signals that the runner is no longer used by the caller and can be claimed by someone else.
	// The runner is deleted or cleaned up for reuse depending on the used executor.
	Return(r Runner) error

	// ScaleAllEnvironments checks for all environments if enough runners are created.
	ScaleAllEnvironments() error

	// RecoverEnvironment adds a recovered Environment to the internal structure.
	// This is intended to recover environments after a restart.
	RecoverEnvironment(id EnvironmentID, templateJob *nomadApi.Job, desiredIdleRunnersCount uint)

	// RecoverRunner adds a recovered runner to the internal structure.
	// This is intended to recover runners after a restart.
	RecoverRunner(id EnvironmentID, job *nomadApi.Job, isUsed bool)
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
	go m.updateRunners(ctx)
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

func (m *NomadRunnerManager) CreateOrUpdateEnvironment(id EnvironmentID, desiredIdleRunnersCount uint, templateJob *nomadApi.Job) (bool, error) {
	_, ok := m.environments.Get(id)
	if !ok {
		return true, m.registerEnvironment(id, desiredIdleRunnersCount, templateJob)
	}
	return false, m.updateEnvironment(id, desiredIdleRunnersCount, templateJob)
}

func (m *NomadRunnerManager) registerEnvironment(environmentID EnvironmentID, desiredIdleRunnersCount uint, templateJob *nomadApi.Job) error {
	m.environments.Add(&NomadEnvironment{
		environmentID,
		NewLocalRunnerStorage(),
		desiredIdleRunnersCount,
		templateJob,
	})
	err := m.scaleEnvironment(environmentID)
	if err != nil {
		return fmt.Errorf("couldn't upscale environment %w", err)
	}
	return nil
}

// updateEnvironment updates all runners of the specified environment. This is required as attributes like the
// CPULimit or MemoryMB could be changed in the new template job.
func (m *NomadRunnerManager) updateEnvironment(id EnvironmentID, desiredIdleRunnersCount uint, newTemplateJob *nomadApi.Job) error {
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

	return m.scaleEnvironment(id)
}

func (m *NomadRunnerManager) updateRunnerSpecs(environmentID EnvironmentID, templateJob *nomadApi.Job) error {
	runners, err := m.apiClient.LoadRunners(environmentID.toString())
	if err != nil {
		return fmt.Errorf("update environment couldn't load runners: %w", err)
	}
	var occurredErrors []string
	for _, id := range runners {
		// avoid taking the address of the loop variable
		runnerID := id
		updatedRunnerJob := *templateJob
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
	return nil
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

func (m *NomadRunnerManager) ScaleAllEnvironments() error {
	for _, environmentID := range m.environments.List() {
		err := m.scaleEnvironment(environmentID)
		if err != nil {
			return fmt.Errorf("can not scale up: %w", err)
		}
	}
	return nil
}

func (m *NomadRunnerManager) RecoverEnvironment(id EnvironmentID, templateJob *nomadApi.Job,
	desiredIdleRunnersCount uint) {
	_, ok := m.environments.Get(id)
	if ok {
		log.Error("Recovering existing environment.")
		return
	}
	environment := &NomadEnvironment{
		environmentID: id,
		idleRunners:   NewLocalRunnerStorage(),
	}
	m.environments.Add(environment)
	log.WithField("environmentID", environment.environmentID).Info("Added recovered environment")
	environment.desiredIdleRunnersCount = desiredIdleRunnersCount
	environment.templateJob = templateJob
}

func (m *NomadRunnerManager) RecoverRunner(id EnvironmentID, job *nomadApi.Job, isUsed bool) {
	environment, ok := m.environments.Get(id)
	if !ok {
		log.Error("Environment missing. Can not recover runner")
		return
	}

	log.WithField("jobID", *job.ID).
		WithField("environmentID", environment.environmentID).
		Info("Added idle runner")

	newJob := NewNomadJob(*job.ID, m.apiClient)
	if isUsed {
		m.usedRunners.Add(newJob)
	} else {
		environment.idleRunners.Add(newJob)
	}
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

	if nomad.IsEnvironmentTemplateID(alloc.JobID) {
		return
	}

	environmentID, err := nomad.EnvironmentIDFromJobID(alloc.JobID)
	if err != nil {
		log.WithError(err).Warn("Allocation could not be added")
		return
	}

	job, ok := m.environments.Get(EnvironmentID(environmentID))
	if ok {
		job.idleRunners.Add(NewNomadJob(alloc.JobID, m.apiClient))
	}
}

func (m *NomadRunnerManager) onAllocationStopped(alloc *nomadApi.Allocation) {
	log.WithField("id", alloc.JobID).Debug("Runner stopped")

	environmentID, err := nomad.EnvironmentIDFromJobID(alloc.JobID)
	if err != nil {
		log.WithError(err).Warn("Stopped allocation can not be handled")
		return
	}

	m.usedRunners.Delete(alloc.JobID)
	job, ok := m.environments.Get(EnvironmentID(environmentID))
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
	newRunnerID := nomad.RunnerJobID(environment.ID().toString(), newUUID.String())

	template := *environment.templateJob
	template.ID = &newRunnerID
	template.Name = &newRunnerID

	evalID, err := m.apiClient.RegisterNomadJob(&template)
	if err != nil {
		return fmt.Errorf("couldn't register Nomad environment: %w", err)
	}
	err = m.apiClient.MonitorEvaluation(evalID, context.Background())
	if err != nil {
		return fmt.Errorf("couldn't monitor evaluation: %w", err)
	}
	return nil
}
