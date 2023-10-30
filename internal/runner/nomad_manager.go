package runner

import (
	"context"
	"errors"
	"fmt"
	nomadApi "github.com/hashicorp/nomad/api"
	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
	"github.com/openHPI/poseidon/internal/nomad"
	"github.com/openHPI/poseidon/pkg/dto"
	"github.com/openHPI/poseidon/pkg/logging"
	"github.com/openHPI/poseidon/pkg/monitoring"
	"github.com/openHPI/poseidon/pkg/storage"
	"github.com/openHPI/poseidon/pkg/util"
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

type NomadRunnerManager struct {
	*AbstractManager
	apiClient nomad.ExecutorAPI
}

// NewNomadRunnerManager creates a new runner manager that keeps track of all runners.
// KeepRunnersSynced has to be started separately.
func NewNomadRunnerManager(apiClient nomad.ExecutorAPI, ctx context.Context) *NomadRunnerManager {
	return &NomadRunnerManager{NewAbstractManager(ctx), apiClient}
}

func (m *NomadRunnerManager) Claim(environmentID dto.EnvironmentID, duration int) (Runner, error) {
	environment, ok := m.environments.Get(environmentID.ToString())
	if !ok {
		return nil, ErrUnknownExecutionEnvironment
	}
	runner, ok := environment.Sample()
	if !ok {
		return nil, ErrNoRunnersAvailable
	}

	m.usedRunners.Add(runner.ID(), runner)
	go m.markRunnerAsUsed(runner, duration)

	runner.SetupTimeout(time.Duration(duration) * time.Second)
	return runner, nil
}

func (m *NomadRunnerManager) markRunnerAsUsed(runner Runner, timeoutDuration int) {
	err := util.RetryExponential(func() (err error) {
		if err = m.apiClient.MarkRunnerAsUsed(runner.ID(), timeoutDuration); err != nil {
			err = fmt.Errorf("cannot mark runner as used: %w", err)
		}
		return
	})
	if err != nil {
		log.WithError(err).WithField(dto.KeyRunnerID, runner.ID()).Error("cannot mark runner as used")
		err := m.Return(runner)
		if err != nil {
			log.WithError(err).WithField(dto.KeyRunnerID, runner.ID()).Error("can't mark runner as used and can't return runner")
		}
	}
}

func (m *NomadRunnerManager) Return(r Runner) error {
	m.usedRunners.Delete(r.ID())
	err := r.Destroy(ErrDestroyedByAPIRequest)
	if err != nil {
		err = fmt.Errorf("cannot return runner: %w", err)
	}
	return err
}

// SynchronizeRunners loads all runners and keeps them synchronized (without a retry mechanism).
func (m *NomadRunnerManager) SynchronizeRunners(ctx context.Context) error {
	log.Info("Loading runners")
	if err := m.load(); err != nil {
		return fmt.Errorf("failed loading runners: %w", err)
	}

	// Watch for changes regarding the existing or new runners.
	log.Info("Watching Event Stream")
	err := m.apiClient.WatchEventStream(ctx,
		&nomad.AllocationProcessing{OnNew: m.onAllocationAdded, OnDeleted: m.onAllocationStopped})

	if err != nil && ctx.Err() == nil {
		err = fmt.Errorf("nomad Event Stream failed!: %w", err)
	}
	return err
}

// Load recovers all runners for all existing environments.
func (m *NomadRunnerManager) load() error {
	newUsedRunners := storage.NewLocalStorage[Runner]()
	for _, environment := range m.environments.List() {
		environmentLogger := log.WithField(dto.KeyEnvironmentID, environment.ID().ToString())

		runnerJobs, err := m.apiClient.LoadRunnerJobs(environment.ID())
		if err != nil {
			return fmt.Errorf("failed fetching the runner jobs: %w", err)
		}
		for _, job := range runnerJobs {
			m.loadSingleJob(job, environmentLogger, environment, newUsedRunners)
		}
		err = environment.ApplyPrewarmingPoolSize()
		if err != nil {
			return fmt.Errorf("couldn't scale environment: %w", err)
		}
	}

	m.updateUsedRunners(newUsedRunners)
	return nil
}

func (m *NomadRunnerManager) loadSingleJob(job *nomadApi.Job, environmentLogger *logrus.Entry,
	environment ExecutionEnvironment, newUsedRunners storage.Storage[Runner]) {
	configTaskGroup := nomad.FindTaskGroup(job, nomad.ConfigTaskGroupName)
	if configTaskGroup == nil {
		environmentLogger.Warnf("Couldn't find config task group in job %s, skipping ...", *job.ID)
		return
	}
	isUsed := configTaskGroup.Meta[nomad.ConfigMetaUsedKey] == nomad.ConfigMetaUsedValue
	portMappings, err := m.apiClient.LoadRunnerPortMappings(*job.ID)
	if err != nil {
		environmentLogger.WithError(err).Warn("Error loading runner portMappings, skipping ...")
		return
	}
	newJob := NewNomadJob(*job.ID, portMappings, m.apiClient, m.onRunnerDestroyed)
	log.WithField("isUsed", isUsed).WithField(dto.KeyRunnerID, newJob.ID()).Debug("Recovered Runner")
	if isUsed {
		newUsedRunners.Add(newJob.ID(), newJob)
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

func (m *NomadRunnerManager) updateUsedRunners(newUsedRunners storage.Storage[Runner]) {
	for _, r := range m.usedRunners.List() {
		var reason DestroyReason
		if _, ok := newUsedRunners.Get(r.ID()); ok {
			reason = ErrDestroyedAndReplaced
		} else {
			reason = ErrLocalDestruction
			log.WithError(reason).WithField(dto.KeyRunnerID, r.ID()).Warn("Local runner cannot be recovered")
		}
		m.usedRunners.Delete(r.ID())
		if err := r.Destroy(reason); err != nil {
			log.WithError(err).WithField(dto.KeyRunnerID, r.ID()).Warn("failed to destroy runner locally")
		}
	}

	for _, r := range newUsedRunners.List() {
		m.usedRunners.Add(r.ID(), r)
	}
}

// onAllocationAdded is the callback for when Nomad started an allocation.
func (m *NomadRunnerManager) onAllocationAdded(alloc *nomadApi.Allocation, startup time.Duration) {
	log.WithField(dto.KeyRunnerID, alloc.JobID).WithField("startupDuration", startup).Debug("Runner started")

	if nomad.IsEnvironmentTemplateID(alloc.JobID) {
		return
	}

	if _, ok := m.usedRunners.Get(alloc.JobID); ok {
		log.WithField(dto.KeyRunnerID, alloc.JobID).WithField("states", alloc.TaskStates).
			Error("Started Runner is already in use")
		return
	}

	environmentID, err := nomad.EnvironmentIDFromRunnerID(alloc.JobID)
	if err != nil {
		log.WithError(err).Warn("Allocation could not be added")
		return
	}

	environment, ok := m.environments.Get(environmentID.ToString())
	if ok {
		var mappedPorts []nomadApi.PortMapping
		if alloc.AllocatedResources != nil {
			mappedPorts = alloc.AllocatedResources.Shared.Ports
		}
		environment.AddRunner(NewNomadJob(alloc.JobID, mappedPorts, m.apiClient, m.onRunnerDestroyed))
		monitorAllocationStartupDuration(startup, alloc.JobID, environmentID)
	}
}

func monitorAllocationStartupDuration(startup time.Duration, runnerID string, environmentID dto.EnvironmentID) {
	p := influxdb2.NewPointWithMeasurement(monitoring.MeasurementIdleRunnerNomad)
	p.AddField(monitoring.InfluxKeyStartupDuration, startup.Nanoseconds())
	p.AddTag(monitoring.InfluxKeyEnvironmentID, environmentID.ToString())
	p.AddTag(monitoring.InfluxKeyRunnerID, runnerID)
	monitoring.WriteInfluxPoint(p)
}

// onAllocationStopped is the callback for when Nomad stopped an allocation.
func (m *NomadRunnerManager) onAllocationStopped(runnerID string, reason error) (alreadyRemoved bool) {
	log.WithField(dto.KeyRunnerID, runnerID).Debug("Runner stopped")

	if nomad.IsEnvironmentTemplateID(runnerID) {
		return false
	}

	environmentID, err := nomad.EnvironmentIDFromRunnerID(runnerID)
	if err != nil {
		log.WithError(err).Warn("Stopped allocation can not be handled")
		return false
	}

	r, stillActive := m.usedRunners.Get(runnerID)
	if stillActive {
		m.usedRunners.Delete(runnerID)
		if err := r.Destroy(reason); err != nil {
			log.WithError(err).Warn("Runner of stopped allocation cannot be destroyed")
		}
	}

	environment, ok := m.environments.Get(environmentID.ToString())
	if ok {
		stillActive = stillActive || environment.DeleteRunner(runnerID)
	}

	return !stillActive
}

// onRunnerDestroyed is the callback when the runner destroys itself.
// The main use of this callback is to remove the runner from the used runners, when its timeout exceeds.
func (m *NomadRunnerManager) onRunnerDestroyed(r Runner) error {
	m.usedRunners.Delete(r.ID())

	environment, ok := m.environments.Get(r.Environment().ToString())
	if ok {
		environment.DeleteRunner(r.ID())
	}
	return nil
}
