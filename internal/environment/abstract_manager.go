package environment

import (
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
	if n.nextHandler != nil {
		return n.nextHandler
	} else {
		return &AbstractManager{}
	}
}

func (n *AbstractManager) List(_ bool) ([]runner.ExecutionEnvironment, error) {
	return []runner.ExecutionEnvironment{}, nil
}

func (n *AbstractManager) Get(_ dto.EnvironmentID, _ bool) (runner.ExecutionEnvironment, error) {
	return nil, runner.ErrRunnerNotFound
}

func (n *AbstractManager) CreateOrUpdate(_ dto.EnvironmentID, _ dto.ExecutionEnvironmentRequest) (bool, error) {
	return false, nil
}

func (n *AbstractManager) Delete(id dto.EnvironmentID) (bool, error) {
	e, ok := n.runnerManager.GetEnvironment(id)
	if !ok {
		if n.nextHandler != nil {
			isFound, err := n.NextHandler().Delete(id)
			if err != nil {
				return false, fmt.Errorf("aws wrapped: %w", err)
			}
			return isFound, nil
		} else {
			return false, nil
		}
	}

	n.runnerManager.DeleteEnvironment(id)
	if err := e.Delete(); err != nil {
		return true, fmt.Errorf("could not delete environment: %w", err)
	}
	return true, nil
}

func (n *AbstractManager) Statistics() map[dto.EnvironmentID]*dto.StatisticalExecutionEnvironmentData {
	return map[dto.EnvironmentID]*dto.StatisticalExecutionEnvironmentData{}
}
