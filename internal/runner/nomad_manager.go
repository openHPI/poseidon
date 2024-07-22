package runner

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"time"

	nomadApi "github.com/hashicorp/nomad/api"
	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
	"github.com/openHPI/poseidon/internal/config"
	"github.com/openHPI/poseidon/internal/nomad"
	"github.com/openHPI/poseidon/pkg/dto"
	"github.com/openHPI/poseidon/pkg/logging"
	"github.com/openHPI/poseidon/pkg/monitoring"
	"github.com/openHPI/poseidon/pkg/storage"
	"github.com/openHPI/poseidon/pkg/util"
)

var (
	log                            = logging.GetLogger("runner")
	ErrUnknownExecutionEnvironment = errors.New("execution environment not found")
	ErrNoRunnersAvailable          = errors.New("no runners available for this execution environment")
	ErrRunnerNotFound              = errors.New("no runner found with this id")
)

type alertData struct {
	*sync.Mutex
	cancel context.CancelFunc
}

type NomadRunnerManager struct {
	*AbstractManager
	apiClient            nomad.ExecutorAPI
	reloadingEnvironment storage.Storage[*alertData]
}

// NewNomadRunnerManager creates a new runner manager that keeps track of all runners.
// KeepRunnersSynced has to be started separately.
func NewNomadRunnerManager(apiClient nomad.ExecutorAPI, ctx context.Context) *NomadRunnerManager {
	return &NomadRunnerManager{NewAbstractManager(ctx), apiClient, storage.NewLocalStorage[*alertData]()}
}

func (m *NomadRunnerManager) Claim(environmentID dto.EnvironmentID, duration int) (Runner, error) {
	environment, ok := m.GetEnvironment(environmentID)
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

// Load recovers all runners for all existing environments.
func (m *NomadRunnerManager) Load(ctx context.Context) {
	log.Info("Loading runners")
	newUsedRunners := storage.NewLocalStorage[Runner]()
	for _, environment := range m.ListEnvironments() {
		usedRunners, err := m.loadEnvironment(ctx, environment)
		if err != nil {
			log.WithError(err).WithField(dto.KeyEnvironmentID, environment.ID().ToString()).
				Warn("Failed loading environment. Skipping...")
			continue
		}
		for _, r := range usedRunners.List() {
			newUsedRunners.Add(r.ID(), r)
		}
	}

	m.updateUsedRunners(newUsedRunners, true)
}

// SynchronizeRunners connect once (without retry) to Nomad to receive status updates regarding runners.
func (m *NomadRunnerManager) SynchronizeRunners(ctx context.Context) error {
	// Watch for changes regarding the existing or new runners.
	log.Info("Watching Event Stream")
	err := m.apiClient.WatchEventStream(ctx,
		&nomad.AllocationProcessing{OnNew: m.onAllocationAdded, OnDeleted: m.onAllocationStopped})

	if err != nil && ctx.Err() == nil {
		err = fmt.Errorf("nomad Event Stream failed!: %w", err)
	}
	return err
}

func (m *NomadRunnerManager) StoreEnvironment(environment ExecutionEnvironment) {
	m.AbstractManager.StoreEnvironment(environment)
	m.reloadingEnvironment.Add(environment.ID().ToString(), &alertData{Mutex: &sync.Mutex{}, cancel: nil})
}

func (m *NomadRunnerManager) DeleteEnvironment(id dto.EnvironmentID) {
	m.AbstractManager.DeleteEnvironment(id)
	m.reloadingEnvironment.Delete(id.ToString())
}

// checkPrewarmingPoolAlert checks if the prewarming pool contains enough idle runners as specified by the PrewarmingPoolThreshold.
// If not, it starts an environment reload mechanism according to the PrewarmingPoolReloadTimeout.
func (m *NomadRunnerManager) checkPrewarmingPoolAlert(ctx context.Context, environment ExecutionEnvironment, runnerAdded bool) {
	data, ok := m.reloadingEnvironment.Get(environment.ID().ToString())
	if !ok {
		log.WithField(dto.KeyEnvironmentID, environment.ID()).Error("reloadingEnvironment not initialized")
		return
	}

	if runnerAdded && data.cancel != nil {
		data.cancel()
		data.cancel = nil
		m.checkPrewarmingPoolAlert(ctx, environment, false)
		return
	}

	// With this hard lock, we collect/block goroutines waiting for one reload to be done.
	// However, in practice, it's likely that only up to PrewarmingPoolSize/2 goroutines are waiting.
	// We could avoid the waiting, but we use it to solve the race conditions of the recursive call above.
	data.Lock()
	defer data.Unlock()

	prewarmingPoolThreshold := config.Config.Server.Alert.PrewarmingPoolThreshold
	reloadTimeout := config.Config.Server.Alert.PrewarmingPoolReloadTimeout

	if reloadTimeout == 0 || float64(environment.IdleRunnerCount())/float64(environment.PrewarmingPoolSize()) >= prewarmingPoolThreshold {
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	data.cancel = cancel

	log.WithField(dto.KeyEnvironmentID, environment.ID()).Info("Prewarming Pool Alert. Checking again...")
	select {
	case <-ctx.Done():
		return
	case <-time.After(time.Duration(reloadTimeout) * time.Second):
	}

	if float64(environment.IdleRunnerCount())/float64(environment.PrewarmingPoolSize()) >= prewarmingPoolThreshold {
		return
	}

	log.WithField(dto.KeyEnvironmentID, environment.ID()).Warn("Prewarming Pool Alert. Reloading environment")
	err := util.RetryExponential(func() error {
		usedRunners, err := m.loadEnvironment(ctx, environment)
		if err != nil {
			return err
		}
		m.updateUsedRunners(usedRunners, false)
		return nil
	})
	if err != nil {
		log.WithField(dto.KeyEnvironmentID, environment.ID()).Error("Failed to reload environment")
	}
}

func (m *NomadRunnerManager) loadEnvironment(ctx context.Context, environment ExecutionEnvironment) (used storage.Storage[Runner], err error) {
	used = storage.NewLocalStorage[Runner]()
	runnerJobs, err := m.apiClient.LoadRunnerJobs(environment.ID())
	if err != nil {
		return nil, fmt.Errorf("failed fetching the runner jobs: %w", err)
	}
	for _, job := range runnerJobs {
		r, isUsed, err := m.loadSingleJob(ctx, job, environment)
		if err != nil {
			log.WithError(err).WithField(dto.KeyEnvironmentID, environment.ID().ToString()).
				WithField("used", isUsed).Warn("Failed loading job. Skipping...")
			continue
		} else if isUsed {
			used.Add(r.ID(), r)
		}
	}
	err = environment.ApplyPrewarmingPoolSize()
	if err != nil {
		return used, fmt.Errorf("couldn't scale environment: %w", err)
	}
	return used, nil
}

func (m *NomadRunnerManager) loadSingleJob(ctx context.Context, job *nomadApi.Job, environment ExecutionEnvironment) (r Runner, isUsed bool, err error) {
	configTaskGroup := nomad.FindTaskGroup(job, nomad.ConfigTaskGroupName)
	if configTaskGroup == nil {
		return nil, false, fmt.Errorf("%w, %s", nomad.ErrorMissingTaskGroup, *job.ID)
	}
	isUsed = configTaskGroup.Meta[nomad.ConfigMetaUsedKey] == nomad.ConfigMetaUsedValue
	portMappings, err := m.apiClient.LoadRunnerPortMappings(*job.ID)
	if err != nil {
		return nil, false, fmt.Errorf("error loading runner portMappings: %w", err)
	}

	newJob := NewNomadJob(ctx, *job.ID, portMappings, m.apiClient, m.onRunnerDestroyed)
	log.WithField("isUsed", isUsed).WithField(dto.KeyRunnerID, newJob.ID()).Debug("Recovered Runner")
	if isUsed {
		timeout, err := strconv.Atoi(configTaskGroup.Meta[nomad.ConfigMetaTimeoutKey])
		if err != nil {
			log.WithField(dto.KeyRunnerID, newJob.ID()).WithError(err).Warn("failed loading timeout from meta values")
			timeout = int(nomad.RunnerTimeoutFallback.Seconds())
			go m.markRunnerAsUsed(newJob, timeout)
		}
		newJob.SetupTimeout(time.Duration(timeout) * time.Second)
	} else {
		environment.AddRunner(newJob)
	}
	return newJob, isUsed, nil
}

// updateUsedRunners handles the cleanup process of updating the used runner storage.
// This includes the clean deletion of the local references to the (replaced/deleted) runners.
// Only if removeDeleted is set, the runners that are only in newUsedRunners (and not in the main m.usedRunners) will be removed.
func (m *NomadRunnerManager) updateUsedRunners(newUsedRunners storage.Storage[Runner], removeDeleted bool) {
	for _, r := range m.usedRunners.List() {
		var reason DestroyReason
		if _, ok := newUsedRunners.Get(r.ID()); ok {
			reason = ErrDestroyedAndReplaced
		} else if removeDeleted {
			reason = ErrLocalDestruction
			log.WithError(reason).WithField(dto.KeyRunnerID, r.ID()).Warn("Local runner cannot be recovered")
		}
		if reason != nil {
			m.usedRunners.Delete(r.ID())
			if err := r.Destroy(reason); err != nil {
				log.WithError(err).WithField(dto.KeyRunnerID, r.ID()).Warn("failed to destroy runner locally")
			}
		}
	}

	for _, r := range newUsedRunners.List() {
		m.usedRunners.Add(r.ID(), r)
	}
}

// onAllocationAdded is the callback for when Nomad started an allocation.
func (m *NomadRunnerManager) onAllocationAdded(ctx context.Context, alloc *nomadApi.Allocation, startup time.Duration) {
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

	environment, ok := m.GetEnvironment(environmentID)
	if ok {
		var mappedPorts []nomadApi.PortMapping
		if alloc.AllocatedResources != nil {
			mappedPorts = alloc.AllocatedResources.Shared.Ports
		}
		environment.AddRunner(NewNomadJob(ctx, alloc.JobID, mappedPorts, m.apiClient, m.onRunnerDestroyed))
		go m.checkPrewarmingPoolAlert(ctx, environment, true)
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
func (m *NomadRunnerManager) onAllocationStopped(ctx context.Context, runnerID string, reason error) (alreadyRemoved bool) {
	log.WithField(dto.KeyRunnerID, runnerID).Debug("Runner stopped")

	if nomad.IsEnvironmentTemplateID(runnerID) {
		return false
	}

	environmentID, err := nomad.EnvironmentIDFromRunnerID(runnerID)
	if err != nil {
		log.WithError(err).Warn("Stopped allocation can not be handled")
		return false
	}

	r, stillUsed := m.usedRunners.Get(runnerID)
	if stillUsed {
		m.usedRunners.Delete(runnerID)
		if err := r.Destroy(reason); err != nil {
			log.WithError(err).Warn("Runner of stopped allocation cannot be destroyed")
		}
	}

	environment, ok := m.GetEnvironment(environmentID)
	if !ok {
		return !stillUsed
	}

	r, stillIdle := environment.DeleteRunner(runnerID)
	if stillIdle {
		if err := r.Destroy(reason); err != nil {
			log.WithError(err).Warn("Runner of stopped allocation cannot be destroyed")
		}
	}
	go m.checkPrewarmingPoolAlert(ctx, environment, false)

	return !(stillUsed || stillIdle)
}

// onRunnerDestroyed is the callback when the runner destroys itself.
// The main use of this callback is to remove the runner from the used runners, when its timeout exceeds.
func (m *NomadRunnerManager) onRunnerDestroyed(r Runner) error {
	m.usedRunners.Delete(r.ID())

	environment, ok := m.GetEnvironment(r.Environment())
	if ok {
		environment.DeleteRunner(r.ID())
	}
	return nil
}
