package runner

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	nomadApi "github.com/hashicorp/nomad/api"
	"github.com/openHPI/poseidon/internal/nomad"
	"github.com/openHPI/poseidon/pkg/dto"
	"github.com/openHPI/poseidon/pkg/logging"
	"github.com/sirupsen/logrus"
	"strconv"
	"time"
)

var (
	log                            = logging.GetLogger("runner")
	ErrUnknownExecutionEnvironment = errors.New("execution environment not found")
	ErrNoRunnersAvailable          = errors.New("no runners available for this execution environment")
	ErrRunnerNotFound              = errors.New("no runner found with this id")
)

// ExecutionEnvironment are groups of runner that share the configuration stored in the environment.
type ExecutionEnvironment interface {
	json.Marshaler

	// ID returns the id of the environment.
	ID() dto.EnvironmentID
	SetID(id dto.EnvironmentID)
	// PrewarmingPoolSize sets the number of idle runner of this environment that should be prewarmed.
	PrewarmingPoolSize() uint
	SetPrewarmingPoolSize(count uint)
	// CPULimit sets the share of cpu that a runner should receive at minimum.
	CPULimit() uint
	SetCPULimit(limit uint)
	// MemoryLimit sets the amount of memory that should be available for each runner.
	MemoryLimit() uint
	SetMemoryLimit(limit uint)
	// Image sets the image of the runner, e.g. Docker image.
	Image() string
	SetImage(image string)
	// NetworkAccess sets if a runner should have network access and if ports should be mapped.
	NetworkAccess() (bool, []uint16)
	SetNetworkAccess(allow bool, ports []uint16)
	// SetConfigFrom copies all above attributes from the passed environment to the object itself.
	SetConfigFrom(environment ExecutionEnvironment)

	// Register saves this environment at the executor.
	Register(apiClient nomad.ExecutorAPI) error
	// Delete removes this environment and all it's runner from the executor and Poseidon itself.
	Delete(apiClient nomad.ExecutorAPI) error
	// Scale manages if the executor has enough idle runner according to the PrewarmingPoolSize.
	Scale(apiClient nomad.ExecutorAPI) error

	// Sample returns and removes an arbitrary available runner.
	// ok is true iff a runner was returned.
	Sample(apiClient nomad.ExecutorAPI) (r Runner, ok bool)
	// AddRunner adds an existing runner to the idle runners of the environment.
	AddRunner(r Runner)
	// DeleteRunner removes an idle runner from the environment.
	DeleteRunner(id string)
	// IdleRunnerCount returns the number of idle runners of the environment.
	IdleRunnerCount() int
}

// Manager keeps track of the used and unused runners of all execution environments in order to provide unused
// runners to new clients and ensure no runner is used twice.
type Manager interface {
	// ListEnvironments returns all execution environments known by Poseidon.
	ListEnvironments() []ExecutionEnvironment

	// GetEnvironment returns the details of the requested environment.
	// Iff the requested environment is not stored it returns false.
	GetEnvironment(id dto.EnvironmentID) (ExecutionEnvironment, bool)

	// SetEnvironment stores the environment in Poseidons memory.
	// It returns true iff a new environment is stored and false iff an existing environment was updated.
	SetEnvironment(environment ExecutionEnvironment) bool

	// DeleteEnvironment removes the specified execution environment in Poseidons memory.
	// It does nothing if the specified environment can not be found.
	DeleteEnvironment(id dto.EnvironmentID)

	// EnvironmentStatistics returns statistical data for each execution environment.
	EnvironmentStatistics() map[dto.EnvironmentID]*dto.StatisticalExecutionEnvironmentData

	// Claim returns a new runner. The runner is deleted after duration seconds if duration is not 0.
	// It makes sure that the runner is not in use yet and returns an error if no runner could be provided.
	Claim(id dto.EnvironmentID, duration int) (Runner, error)

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
	environments EnvironmentStorage
	usedRunners  Storage
}

// NewNomadRunnerManager creates a new runner manager that keeps track of all runners.
// It uses the apiClient for all requests and runs a background task to keep the runners in sync with Nomad.
// If you cancel the context the background synchronization will be stopped.
func NewNomadRunnerManager(apiClient nomad.ExecutorAPI, ctx context.Context) *NomadRunnerManager {
	m := &NomadRunnerManager{
		apiClient,
		NewLocalEnvironmentStorage(),
		NewLocalRunnerStorage(),
	}
	go m.keepRunnersSynced(ctx)
	return m
}

func (m *NomadRunnerManager) ListEnvironments() []ExecutionEnvironment {
	return m.environments.List()
}

func (m *NomadRunnerManager) GetEnvironment(id dto.EnvironmentID) (ExecutionEnvironment, bool) {
	return m.environments.Get(id)
}

func (m *NomadRunnerManager) SetEnvironment(environment ExecutionEnvironment) bool {
	_, ok := m.environments.Get(environment.ID())
	m.environments.Add(environment)
	return !ok
}

func (m *NomadRunnerManager) DeleteEnvironment(id dto.EnvironmentID) {
	m.environments.Delete(id)
}

func (m *NomadRunnerManager) EnvironmentStatistics() map[dto.EnvironmentID]*dto.StatisticalExecutionEnvironmentData {
	environments := make(map[dto.EnvironmentID]*dto.StatisticalExecutionEnvironmentData)
	for _, e := range m.environments.List() {
		environments[e.ID()] = &dto.StatisticalExecutionEnvironmentData{
			ID:                 int(e.ID()),
			PrewarmingPoolSize: e.PrewarmingPoolSize(),
			IdleRunners:        uint(e.IdleRunnerCount()),
			UsedRunners:        0,
		}
	}

	for _, r := range m.usedRunners.List() {
		id, err := nomad.EnvironmentIDFromRunnerID(r.ID())
		if err != nil {
			log.WithError(err).Error("Stored runners must have correct IDs")
		}
		environments[id].UsedRunners++
	}
	return environments
}

func (m *NomadRunnerManager) Claim(environmentID dto.EnvironmentID, duration int) (Runner, error) {
	environment, ok := m.environments.Get(environmentID)
	if !ok {
		return nil, ErrUnknownExecutionEnvironment
	}
	runner, ok := environment.Sample(m.apiClient)
	if !ok {
		return nil, ErrNoRunnersAvailable
	}

	m.usedRunners.Add(runner)
	go m.markRunnerAsUsed(runner, duration)

	runner.SetupTimeout(time.Duration(duration) * time.Second)
	return runner, nil
}

func (m *NomadRunnerManager) markRunnerAsUsed(runner Runner, timeoutDuration int) {
	err := m.apiClient.MarkRunnerAsUsed(runner.ID(), timeoutDuration)
	if err != nil {
		err = m.Return(runner)
		if err != nil {
			log.WithError(err).WithField("runnerID", runner.ID()).Error("can't mark runner as used and can't return runner")
		}
	}
}

func (m *NomadRunnerManager) Get(runnerID string) (Runner, error) {
	runner, ok := m.usedRunners.Get(runnerID)
	if !ok {
		return nil, ErrRunnerNotFound
	}
	return runner, nil
}

func (m *NomadRunnerManager) Return(r Runner) error {
	r.StopTimeout()
	err := m.apiClient.DeleteJob(r.ID())
	if err != nil {
		return fmt.Errorf("error deleting runner in Nomad: %w", err)
	}
	m.usedRunners.Delete(r.ID())
	return nil
}

func (m *NomadRunnerManager) Load() {
	for _, environment := range m.environments.List() {
		environmentLogger := log.WithField("environmentID", environment.ID())
		runnerJobs, err := m.apiClient.LoadRunnerJobs(environment.ID())
		if err != nil {
			environmentLogger.WithError(err).Warn("Error fetching the runner jobs")
		}
		for _, job := range runnerJobs {
			m.loadSingleJob(job, environmentLogger, environment)
		}
		err = environment.Scale(m.apiClient)
		if err != nil {
			environmentLogger.WithError(err).Error("Couldn't scale environment")
		}
	}
}

func (m *NomadRunnerManager) loadSingleJob(job *nomadApi.Job, environmentLogger *logrus.Entry,
	environment ExecutionEnvironment) {
	configTaskGroup := nomad.FindTaskGroup(job, nomad.ConfigTaskGroupName)
	if configTaskGroup == nil {
		environmentLogger.Infof("Couldn't find config task group in job %s, skipping ...", *job.ID)
		return
	}
	isUsed := configTaskGroup.Meta[nomad.ConfigMetaUsedKey] == nomad.ConfigMetaUsedValue
	portMappings, err := m.apiClient.LoadRunnerPortMappings(*job.ID)
	if err != nil {
		environmentLogger.WithError(err).Warn("Error loading runner portMappings")
		return
	}
	newJob := NewNomadJob(*job.ID, portMappings, m.apiClient, m)
	if isUsed {
		m.usedRunners.Add(newJob)
		timeout, err := strconv.Atoi(configTaskGroup.Meta[nomad.ConfigMetaTimeoutKey])
		if err != nil {
			environmentLogger.WithError(err).Warn("Error loading timeout from meta values")
		} else {
			newJob.SetupTimeout(time.Duration(timeout) * time.Second)
		}
	} else {
		environment.AddRunner(newJob)
	}
}

func (m *NomadRunnerManager) keepRunnersSynced(ctx context.Context) {
	retries := 0
	for ctx.Err() == nil {
		err := m.apiClient.WatchEventStream(ctx, m.onAllocationAdded, m.onAllocationStopped)
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

	environmentID, err := nomad.EnvironmentIDFromRunnerID(alloc.JobID)
	if err != nil {
		log.WithError(err).Warn("Allocation could not be added")
		return
	}

	environment, ok := m.environments.Get(environmentID)
	if ok {
		var mappedPorts []nomadApi.PortMapping
		if alloc.AllocatedResources != nil {
			mappedPorts = alloc.AllocatedResources.Shared.Ports
		}
		environment.AddRunner(NewNomadJob(alloc.JobID, mappedPorts, m.apiClient, m))
	}
}

func (m *NomadRunnerManager) onAllocationStopped(alloc *nomadApi.Allocation) {
	log.WithField("id", alloc.JobID).Debug("Runner stopped")

	if nomad.IsEnvironmentTemplateID(alloc.JobID) {
		return
	}

	environmentID, err := nomad.EnvironmentIDFromRunnerID(alloc.JobID)
	if err != nil {
		log.WithError(err).Warn("Stopped allocation can not be handled")
		return
	}

	m.usedRunners.Delete(alloc.JobID)
	environment, ok := m.environments.Get(environmentID)
	if ok {
		environment.DeleteRunner(alloc.JobID)
	}
}
