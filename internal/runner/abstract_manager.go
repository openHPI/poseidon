package runner

import (
	"context"
	"errors"
	"fmt"
	"github.com/influxdata/influxdb-client-go/v2/api/write"
	"github.com/openHPI/poseidon/pkg/dto"
	"github.com/openHPI/poseidon/pkg/monitoring"
	"github.com/openHPI/poseidon/pkg/storage"
	"time"
)

var ErrNullObject = errors.New("functionality not available for the null object")

// AbstractManager is used to have a fallback runner manager in the chain of responsibility
// following the null object pattern.
// Remember all functions that can call the NextHandler should call it (See AccessorHandler).
type AbstractManager struct {
	nextHandler  AccessorHandler
	environments storage.Storage[ExecutionEnvironment]
	usedRunners  storage.Storage[Runner]
}

// NewAbstractManager creates a new abstract runner manager that keeps track of all runners of one kind.
// Since this manager is currently directly bound to the lifespan of Poseidon, it does not need a context cancel.
func NewAbstractManager(ctx context.Context) *AbstractManager {
	return &AbstractManager{
		environments: storage.NewMonitoredLocalStorage[ExecutionEnvironment](
			monitoring.MeasurementEnvironments, monitorEnvironmentData, 0, ctx),
		usedRunners: storage.NewMonitoredLocalStorage[Runner](
			monitoring.MeasurementUsedRunner, MonitorRunnersEnvironmentID, time.Hour, ctx),
	}
}

// MonitorEnvironmentID adds the passed environment id to the monitoring Point p.
func MonitorEnvironmentID[T any](id dto.EnvironmentID) storage.WriteCallback[T] {
	return func(p *write.Point, _ T, _ storage.EventType) {
		p.AddTag(monitoring.InfluxKeyEnvironmentID, id.ToString())
	}
}

// MonitorRunnersEnvironmentID passes the id of the environment e into the monitoring Point p.
func MonitorRunnersEnvironmentID(p *write.Point, e Runner, _ storage.EventType) {
	if e != nil {
		p.AddTag(monitoring.InfluxKeyEnvironmentID, e.Environment().ToString())
	}
}

func (n *AbstractManager) SetNextHandler(next AccessorHandler) {
	n.nextHandler = next
}

func (n *AbstractManager) NextHandler() AccessorHandler {
	if n.HasNextHandler() {
		return n.nextHandler
	} else {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		return NewAbstractManager(ctx)
	}
}

func (n *AbstractManager) HasNextHandler() bool {
	return n.nextHandler != nil
}

func (n *AbstractManager) ListEnvironments() []ExecutionEnvironment {
	return n.environments.List()
}

func (n *AbstractManager) GetEnvironment(id dto.EnvironmentID) (ExecutionEnvironment, bool) {
	return n.environments.Get(id.ToString())
}

func (n *AbstractManager) StoreEnvironment(environment ExecutionEnvironment) {
	n.environments.Add(environment.ID().ToString(), environment)
}

func (n *AbstractManager) DeleteEnvironment(id dto.EnvironmentID) {
	n.environments.Delete(id.ToString())
}

func (n *AbstractManager) EnvironmentStatistics() map[dto.EnvironmentID]*dto.StatisticalExecutionEnvironmentData {
	environments := make(map[dto.EnvironmentID]*dto.StatisticalExecutionEnvironmentData)
	for _, e := range n.environments.List() {
		environments[e.ID()] = &dto.StatisticalExecutionEnvironmentData{
			ID:                 int(e.ID()),
			PrewarmingPoolSize: e.PrewarmingPoolSize(),
			IdleRunners:        e.IdleRunnerCount(),
			UsedRunners:        0, // Increased below.
		}
	}

	for _, r := range n.usedRunners.List() {
		environments[r.Environment()].UsedRunners++
	}

	return environments
}

func (n *AbstractManager) Claim(_ dto.EnvironmentID, _ int) (Runner, error) {
	return nil, ErrNullObject
}

func (n *AbstractManager) Get(runnerID string) (Runner, error) {
	runner, ok := n.usedRunners.Get(runnerID)
	if ok {
		return runner, nil
	}

	if !n.HasNextHandler() {
		return nil, ErrRunnerNotFound
	}

	r, err := n.NextHandler().Get(runnerID)
	if err != nil {
		return r, fmt.Errorf("abstract manager wrapped: %w", err)
	}
	return r, nil
}

func (n *AbstractManager) Return(_ Runner) error {
	return nil
}

func (n *AbstractManager) Load() {}
