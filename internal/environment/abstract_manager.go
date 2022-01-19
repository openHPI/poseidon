package environment

import (
	"github.com/openHPI/poseidon/internal/runner"
	"github.com/openHPI/poseidon/pkg/dto"
)

// AbstractManager is used to have a fallback environment manager in the chain of responsibility
// following the null object pattern.
type AbstractManager struct {
	nextHandler ManagerHandler
}

func (n *AbstractManager) SetNextHandler(next ManagerHandler) {
	n.nextHandler = next
}

func (n *AbstractManager) NextHandler() ManagerHandler {
	return n.nextHandler
}

func (n *AbstractManager) List(_ bool) ([]runner.ExecutionEnvironment, error) {
	return []runner.ExecutionEnvironment{}, nil
}

func (n *AbstractManager) Get(_ dto.EnvironmentID, _ bool) (runner.ExecutionEnvironment, error) {
	return nil, runner.ErrNullObject
}

func (n *AbstractManager) CreateOrUpdate(_ dto.EnvironmentID, _ dto.ExecutionEnvironmentRequest) (bool, error) {
	return false, nil
}

func (n *AbstractManager) Delete(_ dto.EnvironmentID) (bool, error) {
	return false, nil
}

func (n *AbstractManager) Statistics() map[dto.EnvironmentID]*dto.StatisticalExecutionEnvironmentData {
	return map[dto.EnvironmentID]*dto.StatisticalExecutionEnvironmentData{}
}
