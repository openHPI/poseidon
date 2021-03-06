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
// It uses the apiClient for all requests and runs a background task to keep the runners in sync with Nomad.
// If you cancel the context the background synchronization will be stopped.
func NewNomadRunnerManager(apiClient nomad.ExecutorAPI, ctx context.Context) *NomadRunnerManager {
	m := &NomadRunnerManager{NewAbstractManager(), apiClient}
	go m.keepRunnersSynced(ctx)
	return m
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
	err := m.apiClient.MarkRunnerAsUsed(runner.ID(), timeoutDuration)
	if err != nil {
		err = m.Return(runner)
		if err != nil {
			log.WithError(err).WithField("runnerID", runner.ID()).Error("can't mark runner as used and can't return runner")
		}
	}
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
		err = environment.ApplyPrewarmingPoolSize()
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
	newJob := NewNomadJob(*job.ID, portMappings, m.apiClient, m.Return)
	if isUsed {
		m.usedRunners.Add(newJob.ID(), newJob)
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
		err := m.apiClient.WatchEventStream(ctx,
			&nomad.AllocationProcessoring{OnNew: m.onAllocationAdded, OnDeleted: m.onAllocationStopped})
		retries += 1
		log.WithError(err).Errorf("Stopped updating the runners! Retry %v", retries)
		<-time.After(time.Second)
	}
}

func (m *NomadRunnerManager) onAllocationAdded(alloc *nomadApi.Allocation, startup time.Duration) {
	log.WithField("id", alloc.JobID).WithField("startupDuration", startup).Debug("Runner started")

	if nomad.IsEnvironmentTemplateID(alloc.JobID) {
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
		environment.AddRunner(NewNomadJob(alloc.JobID, mappedPorts, m.apiClient, m.Return))
		monitorAllocationStartupDuration(startup, alloc.JobID, environmentID)
	}
}

func monitorAllocationStartupDuration(startup time.Duration, runnerID string, environmentID dto.EnvironmentID) {
	p := influxdb2.NewPointWithMeasurement(monitoring.MeasurementIdleRunnerNomad)
	p.AddField(monitoring.InfluxKeyDuration, startup)
	p.AddTag(monitoring.InfluxKeyEnvironmentID, environmentID.ToString())
	p.AddTag(monitoring.InfluxKeyRunnerID, runnerID)
	monitoring.WriteInfluxPoint(p)
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
	environment, ok := m.environments.Get(environmentID.ToString())
	if ok {
		environment.DeleteRunner(alloc.JobID)
	}
}
