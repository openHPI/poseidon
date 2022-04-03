package runner

import (
	"errors"
	"fmt"
	"github.com/openHPI/poseidon/pkg/dto"
)

var ErrNullObject = errors.New("functionality not available for the null object")

// AbstractManager is used to have a fallback runner manager in the chain of responsibility
// following the null object pattern.
// Remember all functions that can call the NextHandler should call it (See AccessorHandler).
type AbstractManager struct {
	nextHandler  AccessorHandler
	environments EnvironmentStorage
	usedRunners  Storage
}

// NewAbstractManager creates a new abstract runner manager that keeps track of all runners of one kind.
func NewAbstractManager() *AbstractManager {
	return &AbstractManager{
		environments: NewLocalEnvironmentStorage(),
		usedRunners:  NewLocalRunnerStorage(),
	}
}

func (n *AbstractManager) SetNextHandler(next AccessorHandler) {
	n.nextHandler = next
}

func (n *AbstractManager) NextHandler() AccessorHandler {
	if n.nextHandler != nil {
		return n.nextHandler
	} else {
		return NewAbstractManager()
	}
}

func (n *AbstractManager) HasNextHandler() bool {
	return n.nextHandler != nil
}

func (n *AbstractManager) ListEnvironments() []ExecutionEnvironment {
	return n.environments.List()
}

func (n *AbstractManager) GetEnvironment(id dto.EnvironmentID) (ExecutionEnvironment, bool) {
	return n.environments.Get(id)
}

func (n *AbstractManager) StoreEnvironment(environment ExecutionEnvironment) {
	n.environments.Add(environment)
}

func (n *AbstractManager) DeleteEnvironment(id dto.EnvironmentID) {
	n.environments.Delete(id)
}

func (n *AbstractManager) EnvironmentStatistics() map[dto.EnvironmentID]*dto.StatisticalExecutionEnvironmentData {
	environments := make(map[dto.EnvironmentID]*dto.StatisticalExecutionEnvironmentData)
	for _, e := range n.environments.List() {
		environments[e.ID()] = &dto.StatisticalExecutionEnvironmentData{
			ID:                 int(e.ID()),
			PrewarmingPoolSize: e.PrewarmingPoolSize(),
			IdleRunners:        uint(e.IdleRunnerCount()),
			UsedRunners:        0,
		}
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
