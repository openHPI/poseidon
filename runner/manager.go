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
	"time"
)

var (
	log                            = logging.GetLogger("runner")
	ErrUnknownExecutionEnvironment = errors.New("execution environment not found")
	ErrNoRunnersAvailable          = errors.New("no runners available for this execution environment")
	ErrRunnerNotFound              = errors.New("no runner found with this id")
)

const runnerNameFormat = "%s-%s"

type EnvironmentID int

func (e EnvironmentID) toString() string {
	return strconv.Itoa(int(e))
}

type NomadJobID string

// Manager keeps track of the used and unused runners of all execution environments in order to provide unused
// runners to new clients and ensure no runner is used twice.
type Manager interface {
	// RegisterEnvironment adds a new environment that should be managed.
	RegisterEnvironment(id EnvironmentID, desiredIdleRunnersCount uint) error

	// EnvironmentExists returns whether the environment with the given id exists.
	EnvironmentExists(id EnvironmentID) bool

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
func NewNomadRunnerManager(apiClient nomad.ExecutorAPI, ctx context.Context) *NomadRunnerManager {
	m := &NomadRunnerManager{
		apiClient,
		NewLocalNomadJobStorage(),
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

func (m *NomadRunnerManager) RegisterEnvironment(environmentID EnvironmentID, desiredIdleRunnersCount uint) error {
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

func (m *NomadRunnerManager) EnvironmentExists(id EnvironmentID) (ok bool) {
	_, ok = m.environments.Get(id)
	return
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
	err := m.scaleEnvironment(environmentID)
	if err != nil {
		return nil, fmt.Errorf("can not scale up: %w", err)
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
	log.WithField("id", alloc.ID).Debug("Allocation started")

	intJobID, err := strconv.Atoi(alloc.JobID)
	if err != nil {
		return
	}

	job, ok := m.environments.Get(EnvironmentID(intJobID))
	if ok {
		job.idleRunners.Add(NewNomadAllocation(alloc.ID, m.apiClient))
	}
}

func (m *NomadRunnerManager) onAllocationStopped(alloc *nomadApi.Allocation) {
	log.WithField("id", alloc.ID).Debug("Allocation stopped")

	intJobID, err := strconv.Atoi(alloc.JobID)
	if err != nil {
		return
	}

	m.usedRunners.Delete(alloc.ID)
	job, ok := m.environments.Get(EnvironmentID(intJobID))
	if ok {
		job.idleRunners.Delete(alloc.ID)
	}
}

// scaleEnvironment makes sure that the amount of idle runners is at least the desiredIdleRunnersCount.
func (m *NomadRunnerManager) scaleEnvironment(id EnvironmentID) error {
	environment, ok := m.environments.Get(id)
	if !ok {
		return ErrUnknownExecutionEnvironment
	}

	required := int(environment.desiredIdleRunnersCount) - environment.idleRunners.Length()
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
	newRunnerID := fmt.Sprintf(runnerNameFormat, environment.ID().toString(), newUUID.String())

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
	environment.idleRunners.Add(NewNomadJob(newRunnerID, m.apiClient))
	return nil
}

func (m *NomadRunnerManager) unusedRunners(environmentID EnvironmentID, fetchedRunnerIds []string) (newRunners []Runner) {
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
	return
}
