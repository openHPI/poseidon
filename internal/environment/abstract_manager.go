package environment

import (
	"context"
	"fmt"
	"github.com/openHPI/poseidon/internal/runner"
	"github.com/openHPI/poseidon/pkg/dto"
)

// AbstractManager is used to have a fallback environment manager in the chain of responsibility
// following the null object pattern.
type AbstractManager struct {
	nextHandler   ManagerHandler
	runnerManager runner.Manager
}

func (n *AbstractManager) SetNextHandler(next ManagerHandler) {
	n.nextHandler = next
}

func (n *AbstractManager) NextHandler() ManagerHandler {
	if n.HasNextHandler() {
		return n.nextHandler
	} else {
		return &AbstractManager{}
	}
}

func (n *AbstractManager) HasNextHandler() bool {
	return n.nextHandler != nil
}

func (n *AbstractManager) List(_ bool) ([]runner.ExecutionEnvironment, error) {
	return []runner.ExecutionEnvironment{}, nil
}

func (n *AbstractManager) Get(_ dto.EnvironmentID, _ bool) (runner.ExecutionEnvironment, error) {
	return nil, runner.ErrRunnerNotFound
}

func (n *AbstractManager) CreateOrUpdate(_ dto.EnvironmentID, _ dto.ExecutionEnvironmentRequest, _ context.Context) (
	bool, error) {
	return false, nil
}

func (n *AbstractManager) Delete(id dto.EnvironmentID) (bool, error) {
	if n.runnerManager == nil {
		return false, nil
	}

	e, ok := n.runnerManager.GetEnvironment(id)
	if !ok {
		isFound, err := n.NextHandler().Delete(id)
		if err != nil {
			return false, fmt.Errorf("abstract wrapped: %w", err)
		}
		return isFound, nil
	}

	n.runnerManager.DeleteEnvironment(id)
	if err := e.Delete(false); err != nil {
		return true, fmt.Errorf("could not delete environment: %w", err)
	}
	return true, nil
}

func (n *AbstractManager) Statistics() map[dto.EnvironmentID]*dto.StatisticalExecutionEnvironmentData {
	if n.runnerManager == nil {
		return map[dto.EnvironmentID]*dto.StatisticalExecutionEnvironmentData{}
	}

	statistics := n.NextHandler().Statistics()
	for k, v := range n.runnerManager.EnvironmentStatistics() {
		statistics[k] = v
	}
	return statistics
}
