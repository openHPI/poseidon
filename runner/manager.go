package runner

import (
	"context"
	"errors"
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

type EnvironmentID int

func (e EnvironmentID) toString() string {
	return string(rune(e))
}

type NomadJobID string

// Manager keeps track of the used and unused runners of all execution environments in order to provide unused
// runners to new clients and ensure no runner is used twice.
type Manager interface {
	// RegisterEnvironment adds a new environment that should be managed.
	RegisterEnvironment(id EnvironmentID, nomadJobID NomadJobID, desiredIdleRunnersCount uint)

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
	apiClient   nomad.ExecutorAPI
	jobs        NomadJobStorage
	usedRunners Storage
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

type NomadJob struct {
	environmentID           EnvironmentID
	jobID                   NomadJobID
	idleRunners             Storage
	desiredIdleRunnersCount uint
}

func (j *NomadJob) ID() EnvironmentID {
	return j.environmentID
}

func (m *NomadRunnerManager) RegisterEnvironment(environmentID EnvironmentID, nomadJobID NomadJobID,
	desiredIdleRunnersCount uint) {
	m.jobs.Add(&NomadJob{
		environmentID,
		nomadJobID,
		NewLocalRunnerStorage(),
		desiredIdleRunnersCount,
	})
	go m.refreshEnvironment(environmentID)
}

func (m *NomadRunnerManager) EnvironmentExists(id EnvironmentID) (ok bool) {
	_, ok = m.jobs.Get(id)
	return
}

func (m *NomadRunnerManager) Claim(environmentID EnvironmentID) (Runner, error) {
	job, ok := m.jobs.Get(environmentID)
	if !ok {
		return nil, ErrUnknownExecutionEnvironment
	}
	runner, ok := job.idleRunners.Sample()
	if !ok {
		return nil, ErrNoRunnersAvailable
	}
	m.usedRunners.Add(runner)
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

	job, ok := m.jobs.Get(EnvironmentID(intJobID))
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
	job, ok := m.jobs.Get(EnvironmentID(intJobID))
	if ok {
		job.idleRunners.Delete(alloc.ID)
	}
}

// Refresh Big ToDo: Improve this function!! State out that it also rescales the job; Provide context to be terminable...
func (m *NomadRunnerManager) refreshEnvironment(id EnvironmentID) {
	job, ok := m.jobs.Get(id)
	if !ok {
		// this environment does not exist
		return
	}
	var lastJobScaling uint = 0
	for {
		runners, err := m.apiClient.LoadRunners(string(job.jobID))
		if err != nil {
			log.WithError(err).Printf("Failed fetching runners")
			break
		}
		for _, r := range m.unusedRunners(id, runners) {
			// ToDo: Listen on Nomad event stream
			log.Printf("Adding allocation %+v", r)

			job.idleRunners.Add(r)
		}
		jobScale, err := m.apiClient.JobScale(string(job.jobID))
		if err != nil {
			log.WithError(err).Printf("Failed get allocation count")
			break
		}
		additionallyNeededRunners := job.desiredIdleRunnersCount - uint(job.idleRunners.Length())
		requiredRunnerCount := jobScale
		if additionallyNeededRunners > 0 {
			requiredRunnerCount += additionallyNeededRunners
		}
		time.Sleep(50 * time.Millisecond)
		if requiredRunnerCount != lastJobScaling {
			log.Printf("Set job scaling %d", requiredRunnerCount)
			err = m.apiClient.SetJobScale(string(job.jobID), requiredRunnerCount, "Runner Requested")
			if err != nil {
				log.WithError(err).Printf("Failed set allocation scaling")
				continue
			}
			lastJobScaling = requiredRunnerCount
		}
	}
}

func (m *NomadRunnerManager) unusedRunners(environmentId EnvironmentID, fetchedRunnerIds []string) (newRunners []Runner) {
	newRunners = make([]Runner, 0)
	job, ok := m.jobs.Get(environmentId)
	if !ok {
		// the environment does not exist, so it won't have any unused runners
		return
	}
	for _, runnerID := range fetchedRunnerIds {
		_, ok := m.usedRunners.Get(runnerID)
		if !ok {
			_, ok = job.idleRunners.Get(runnerID)
			if !ok {
				newRunners = append(newRunners, NewNomadAllocation(runnerID, m.apiClient))
			}
		}
	}
	return
}
