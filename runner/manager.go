package runner

import (
	"errors"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/logging"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/nomad"
	"time"
)

var (
	log                            = logging.GetLogger("runner")
	ErrUnknownExecutionEnvironment = errors.New("execution environment not found")
	ErrNoRunnersAvailable          = errors.New("no runners available for this execution environment")
	ErrRunnerNotFound              = errors.New("no runner found with this id")
)

type EnvironmentId int

func (e EnvironmentId) toString() string {
	return string(rune(e))
}

type NomadJobId string

// Manager keeps track of the used and unused runners of all execution environments in order to provide unused runners to new clients and ensure no runner is used twice.
type Manager interface {
	// RegisterEnvironment adds a new environment that should be managed.
	RegisterEnvironment(environmentId EnvironmentId, nomadJobId NomadJobId, desiredIdleRunnersCount int)

	// EnvironmentExists returns whether the environment with the given id exists.
	EnvironmentExists(id EnvironmentId) bool

	// Claim returns a new runner.
	// It makes sure that the runner is not in use yet and returns an error if no runner could be provided.
	Claim(id EnvironmentId) (Runner, error)

	// Get returns the used runner with the given runnerId.
	// If no runner with the given runnerId is currently used, it returns an error.
	Get(runnerId string) (Runner, error)

	// Return signals that the runner is no longer used by the caller and can be claimed by someone else.
	// The runner is deleted or cleaned up for reuse depending on the used executor.
	Return(r Runner) error
}

type NomadRunnerManager struct {
	apiClient   nomad.ExecutorApi
	jobs        NomadJobStorage
	usedRunners Storage
}

func NewNomadRunnerManager(apiClient nomad.ExecutorApi) *NomadRunnerManager {
	return &NomadRunnerManager{
		apiClient,
		NewLocalNomadJobStorage(),
		NewLocalRunnerStorage(),
	}
}

type NomadJob struct {
	environmentId           EnvironmentId
	jobId                   NomadJobId
	idleRunners             Storage
	desiredIdleRunnersCount int
}

func (j *NomadJob) Id() EnvironmentId {
	return j.environmentId
}

func (m *NomadRunnerManager) RegisterEnvironment(environmentId EnvironmentId, nomadJobId NomadJobId, desiredIdleRunnersCount int) {
	m.jobs.Add(&NomadJob{
		environmentId,
		nomadJobId,
		NewLocalRunnerStorage(),
		desiredIdleRunnersCount,
	})
	go m.refreshEnvironment(environmentId)
}

func (m *NomadRunnerManager) EnvironmentExists(id EnvironmentId) (ok bool) {
	_, ok = m.jobs.Get(id)
	return
}

func (m *NomadRunnerManager) Claim(environmentId EnvironmentId) (Runner, error) {
	job, ok := m.jobs.Get(environmentId)
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

func (m *NomadRunnerManager) Get(runnerId string) (Runner, error) {
	runner, ok := m.usedRunners.Get(runnerId)
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

// Refresh Big ToDo: Improve this function!! State out that it also rescales the job; Provide context to be terminable...
func (m *NomadRunnerManager) refreshEnvironment(id EnvironmentId) {
	job, ok := m.jobs.Get(id)
	if !ok {
		// this environment does not exist
		return
	}
	lastJobScaling := -1
	for {
		runners, err := m.apiClient.LoadRunners(string(job.jobId))
		if err != nil {
			log.WithError(err).Printf("Failed fetching runners")
			break
		}
		for _, r := range m.unusedRunners(id, runners) {
			// ToDo: Listen on Nomad event stream
			log.Printf("Adding allocation %+v", r)

			job.idleRunners.Add(r)
		}
		jobScale, err := m.apiClient.JobScale(string(job.jobId))
		if err != nil {
			log.WithError(err).Printf("Failed get allocation count")
			break
		}
		additionallyNeededRunners := job.desiredIdleRunnersCount - job.idleRunners.Length() + 1
		requiredRunnerCount := jobScale
		if additionallyNeededRunners > 0 {
			requiredRunnerCount += additionallyNeededRunners
		}
		time.Sleep(50 * time.Millisecond)
		if requiredRunnerCount != lastJobScaling {
			log.Printf("Set job scaling %d", requiredRunnerCount)
			err = m.apiClient.SetJobScale(string(job.jobId), requiredRunnerCount, "Runner Requested")
			if err != nil {
				log.WithError(err).Printf("Failed set allocation scaling")
				continue
			}
			lastJobScaling = requiredRunnerCount
		}
	}
}

func (m *NomadRunnerManager) unusedRunners(environmentId EnvironmentId, fetchedRunnerIds []string) (newRunners []Runner) {
	newRunners = make([]Runner, 0)
	job, ok := m.jobs.Get(environmentId)
	if !ok {
		// the environment does not exist, so it won't have any unused runners
		return
	}
	for _, runnerId := range fetchedRunnerIds {
		_, ok := m.usedRunners.Get(runnerId)
		if !ok {
			_, ok = job.idleRunners.Get(runnerId)
			if !ok {
				newRunners = append(newRunners, NewNomadAllocation(runnerId, m.apiClient))
			}
		}
	}
	return
}
