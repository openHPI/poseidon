package runner

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strconv"
	"sync"
	"time"

	nomadApi "github.com/hashicorp/nomad/api"
	"github.com/hashicorp/nomad/nomad/structs"
	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
	"github.com/openHPI/poseidon/internal/config"
	"github.com/openHPI/poseidon/internal/nomad"
	"github.com/openHPI/poseidon/pkg/dto"
	"github.com/openHPI/poseidon/pkg/logging"
	"github.com/openHPI/poseidon/pkg/monitoring"
	"github.com/openHPI/poseidon/pkg/storage"
	"github.com/openHPI/poseidon/pkg/util"
)

const environmentMigrationDelay = time.Minute

var (
	log                   = logging.GetLogger("runner")
	ErrNoRunnersAvailable = errors.New("no runners available for this execution environment")
	ErrRunnerNotFound     = errors.New("no runner found with this id")
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
func NewNomadRunnerManager(ctx context.Context, apiClient nomad.ExecutorAPI) *NomadRunnerManager {
	return &NomadRunnerManager{NewAbstractManager(ctx), apiClient, storage.NewLocalStorage[*alertData]()}
}

func (m *NomadRunnerManager) Claim(environmentID dto.EnvironmentID, duration int) (Runner, error) {
	environment, ok := m.GetEnvironment(environmentID)
	if !ok {
		r, err := m.NextHandler().Claim(environmentID, duration)
		if err != nil {
			return nil, fmt.Errorf("nomad wrapped: %w", err)
		}
		return r, nil
	}

	runner, ok := environment.Sample()
	if !ok {
		return nil, ErrNoRunnersAvailable
	}

	m.usedRunners.Add(runner.ID(), runner)
	go m.setRunnerMetaUsed(runner, true, duration)

	runner.SetupTimeout(time.Duration(duration) * time.Second)
	return runner, nil
}

func (m *NomadRunnerManager) setRunnerMetaUsed(runner Runner, used bool, timeoutDuration int) {
	err := util.RetryExponential(func() (err error) {
		if err = m.apiClient.SetRunnerMetaUsed(runner.ID(), used, timeoutDuration); err != nil {
			err = fmt.Errorf("cannot mark runner as used: %w", err)
		}
		return
	})
	if err != nil {
		log.WithError(err).WithField(dto.KeyRunnerID, runner.ID()).WithField("used", used).Error("cannot mark runner")
		err := m.Return(runner)
		if err != nil {
			log.WithError(err).WithField(dto.KeyRunnerID, runner.ID()).Error("can't mark runner and can't return runner")
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
	if reloadTimeout > uint(math.MaxInt64)/uint(time.Second) {
		log.WithField("timeout", reloadTimeout).Error("configured reload timeout too big")
	}
	reloadTimeoutDuration := time.Duration(reloadTimeout) * time.Second

	if reloadTimeout == 0 || float64(environment.IdleRunnerCount())/float64(environment.PrewarmingPoolSize()) >= prewarmingPoolThreshold {
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	data.cancel = cancel

	log.WithField(dto.KeyEnvironmentID, environment.ID()).Info("Prewarming Pool Alert. Checking again...")
	select {
	case <-ctx.Done():
		return
	case <-time.After(reloadTimeoutDuration):
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

func (m *NomadRunnerManager) loadSingleJob(ctx context.Context, job *nomadApi.Job, environment ExecutionEnvironment,
) (r Runner, isUsed bool, err error) {
	configTaskGroup := nomad.FindTaskGroup(job, nomad.ConfigTaskGroupName)
	if configTaskGroup == nil {
		return nil, false, fmt.Errorf("%w, %s", nomad.ErrMissingTaskGroup, *job.ID)
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
			go m.setRunnerMetaUsed(newJob, true, timeout)
		}
		newJob.SetupTimeout(time.Duration(timeout) * time.Second)
	} else {
		environment.AddRunner(newJob)
	}
	return newJob, isUsed, nil
}

// updateUsedRunners handles the updating of the used runner storage.
// This includes the update of the local reference with new/changed values and the deletion of the additional runner reference.
// Only if removeDeleted is set, the runners that are only in m.usedRunners (and not in the newUsedRunners) will be removed.
func (m *NomadRunnerManager) updateUsedRunners(newUsedRunners storage.Storage[Runner], removeDeleted bool) {
	for _, r := range m.usedRunners.List() {
		if newRunner, ok := newUsedRunners.Get(r.ID()); ok {
			updatePortMapping(r, newRunner)
		} else if removeDeleted {
			// Remove old reference
			log.WithField(dto.KeyRunnerID, r.ID()).Warn("Local runner cannot be recovered")
			m.usedRunners.Delete(r.ID())
			if err := r.Destroy(ErrLocalDestruction); err != nil {
				log.WithError(err).WithField(dto.KeyRunnerID, r.ID()).Warn("failed to destroy runner locally")
			}
		}
	}

	for _, r := range newUsedRunners.List() {
		// Add runners not already in m.usedRunners
		if _, ok := m.usedRunners.Get(r.ID()); !ok {
			m.usedRunners.Add(r.ID(), r)
		}
	}
}

// updatePortMapping sets the port mapping of target to the port mapping of updated.
// It then removes the updated reference to the runner.
func updatePortMapping(target Runner, updated Runner) {
	defer func() {
		// Remove updated reference. We keep using the old reference to not cancel running executions.
		if err := updated.Destroy(ErrLocalDestruction); err != nil {
			log.WithError(err).WithField(dto.KeyRunnerID, target.ID()).Warn("failed to destroy runner locally")
		}
	}()

	nomadRunner, ok := target.(*NomadJob)
	if !ok {
		log.WithField(dto.KeyRunnerID, target.ID()).Error("Unexpected handling of non-Nomad runner")
		return
	}
	if err := nomadRunner.UpdateMappedPorts(updated.MappedPorts()); err != nil {
		log.WithError(err).WithField(dto.KeyRunnerID, target.ID()).Error("Failed updating the port mapping")
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

		r := NewNomadJob(ctx, alloc.JobID, mappedPorts, m.apiClient, m.onRunnerDestroyed)
		if alloc.PreviousAllocation != "" {
			go m.setRunnerMetaUsed(r, false, 0)
		}
		environment.AddRunner(r)
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

// checkForMigratingEnvironmentJob checks if the Nomad environment job is still running after the delay.
func (m *NomadRunnerManager) checkForMigratingEnvironmentJob(ctx context.Context, jobID string, delay time.Duration) {
	log.WithField(dto.KeyEnvironmentID, jobID).Debug("Environment stopped unexpectedly. Checking again...")

	select {
	case <-ctx.Done():
		return
	case <-time.After(delay):
	}

	templateJobs, err := m.apiClient.LoadEnvironmentJobs()
	if err != nil {
		log.WithError(err).Warn("couldn't load template jobs")
	}

	var environmentStillRunning bool
	for _, job := range templateJobs {
		if jobID == *job.ID && *job.Status == structs.JobStatusRunning {
			environmentStillRunning = true
			break
		}
	}

	if !environmentStillRunning {
		log.WithField(dto.KeyEnvironmentID, jobID).Warn("Environment stopped unexpectedly")
	}
}

// onAllocationStopped is the callback for when Nomad stopped an allocation.
func (m *NomadRunnerManager) onAllocationStopped(ctx context.Context, runnerID string, reason error) (alreadyRemoved bool) {
	log.WithField(dto.KeyRunnerID, runnerID).Debug("Runner stopped")

	if nomad.IsEnvironmentTemplateID(runnerID) {
		environmentID, err := nomad.EnvironmentIDFromTemplateJobID(runnerID)
		if err != nil {
			log.WithError(err).WithField(dto.KeyEnvironmentID, runnerID).WithField("reason", reason).
				Error("Cannot parse environment id")
			return false
		}
		_, ok := m.environments.Get(environmentID.ToString())
		if ok {
			go m.checkForMigratingEnvironmentJob(ctx, runnerID, environmentMigrationDelay)
		}
		return !ok
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

	return !stillUsed && !stillIdle
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
