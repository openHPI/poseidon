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

// Manager keeps track of the used and unused runners of all execution environments in order to provide unused runners to new clients and ensure no runner is used twice.
type Manager interface {
	// RegisterEnvironment adds a new environment for being managed.
	RegisterEnvironment(environmentId EnvironmentId, nomadJobId NomadJobId, desiredIdleRunnersCount int)

	// Use returns a new runner.
	// It makes sure that runner is not in use yet and returns an error if no runner could be provided.
	Use(id EnvironmentId) (Runner, error)

	// Get returns the used runner with the given runnerId.
	// If no runner with the given runnerId is currently used, it returns an error.
	Get(runnerId string) (Runner, error)

	// Return hands back the runner.
	// The runner is deleted or cleaned up for reuse depending on the used executor.
	Return(r Runner) error
}

func NewNomadRunnerManager(apiClient nomad.ExecutorApi) *NomadRunnerManager {
	return &NomadRunnerManager{
		apiClient,
		make(map[EnvironmentId]*NomadJob),
		NewLocalRunnerPool(),
	}
}

type EnvironmentId int
type NomadJobId string

type NomadJob struct {
	jobId                   NomadJobId
	idleRunners             Pool
	desiredIdleRunnersCount int
}

type NomadRunnerManager struct {
	apiClient   nomad.ExecutorApi
	jobs        map[EnvironmentId]*NomadJob
	usedRunners Pool
}

func (m *NomadRunnerManager) RegisterEnvironment(environmentId EnvironmentId, nomadJobId NomadJobId, desiredIdleRunnersCount int) {
	m.jobs[environmentId] = &NomadJob{
		nomadJobId,
		NewLocalRunnerPool(),
		desiredIdleRunnersCount,
	}
	go m.refreshEnvironment(environmentId)
}

func (m *NomadRunnerManager) Use(environmentId EnvironmentId) (Runner, error) {
	job, ok := m.jobs[environmentId]
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

func (m *NomadRunnerManager) Get(runnerId string) (r Runner, err error) {
	runner, ok := m.usedRunners.Get(runnerId)
	if !ok {
		return nil, ErrRunnerNotFound
	}
	return runner.(Runner), nil
}

func (m *NomadRunnerManager) Return(r Runner) (err error) {
	err = m.apiClient.DeleteRunner(r.Id())
	if err != nil {
		return err
	}
	m.usedRunners.Delete(r.Id())
	return
}

// Refresh Big ToDo: Improve this function!! State out that it also rescales the job; Provide context to be terminable...
func (m *NomadRunnerManager) refreshEnvironment(id EnvironmentId) {
	job := m.jobs[id]
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
		neededRunners := job.desiredIdleRunnersCount - job.idleRunners.Len() + 1
		runnerCount := jobScale + neededRunners
		time.Sleep(50 * time.Millisecond)
		if runnerCount != lastJobScaling {
			log.Printf("Set job scaling %d", runnerCount)
			err = m.apiClient.SetJobScale(string(job.jobId), runnerCount, "Runner Requested")
			if err != nil {
				log.WithError(err).Printf("Failed set allocation scaling")
				continue
			}
			lastJobScaling = runnerCount
		}
	}
}

func (m *NomadRunnerManager) unusedRunners(environmentId EnvironmentId, fetchedRunnerIds []string) (newRunners []Runner) {
	newRunners = make([]Runner, 0)
	for _, runnerId := range fetchedRunnerIds {
		_, ok := m.usedRunners.Get(runnerId)
		if !ok {
			_, ok = m.jobs[environmentId].idleRunners.Get(runnerId)
			if !ok {
				newRunners = append(newRunners, NewRunner(runnerId))
			}
		}
	}
	return
}
