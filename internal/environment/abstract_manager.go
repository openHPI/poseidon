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

// NewAbstractManager creates a new abstract runner manager that keeps track of all environments of one kind.
func NewAbstractManager(runnerManager runner.Manager) *AbstractManager {
	return &AbstractManager{
		nextHandler:   nil,
		runnerManager: runnerManager,
	}
}

func (n *AbstractManager) SetNextHandler(next ManagerHandler) {
	n.nextHandler = next
}

func (n *AbstractManager) NextHandler() ManagerHandler {
	if n.HasNextHandler() {
		return n.nextHandler
	}

	return &AbstractManager{}
}

func (n *AbstractManager) HasNextHandler() bool {
	return n.nextHandler != nil
}

func (n *AbstractManager) List(_ context.Context, _ bool) ([]runner.ExecutionEnvironment, error) {
	return []runner.ExecutionEnvironment{}, nil
}

func (n *AbstractManager) Get(_ context.Context, _ dto.EnvironmentID, _ bool) (runner.ExecutionEnvironment, error) {
	return nil, runner.ErrRunnerNotFound
}

func (n *AbstractManager) CreateOrUpdate(_ context.Context, _ dto.EnvironmentID, _ dto.ExecutionEnvironmentRequest) (
	bool, error,
) {
	return false, dto.ErrNotSupported
}

func (n *AbstractManager) Delete(environmentID dto.EnvironmentID) (bool, error) {
	if n.runnerManager == nil {
		return false, nil
	}

	executionEnvironment, ok := n.runnerManager.GetEnvironment(environmentID)
	if !ok {
		isFound, err := n.NextHandler().Delete(environmentID)
		if err != nil {
			return false, fmt.Errorf("abstract wrapped: %w", err)
		}

		return isFound, nil
	}

	n.runnerManager.DeleteEnvironment(environmentID)

	err := executionEnvironment.Delete(runner.ErrDestroyedByAPIRequest)
	if err != nil {
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
