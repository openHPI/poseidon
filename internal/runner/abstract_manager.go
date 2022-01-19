package runner

import (
	"errors"
	"github.com/openHPI/poseidon/pkg/dto"
)

var ErrNullObject = errors.New("functionality not available for the null object")

// AbstractManager is used to have a fallback runner manager in the chain of responsibility
// following the null object pattern.
type AbstractManager struct {
	nextHandler AccessorHandler
}

func (n *AbstractManager) SetNextHandler(next AccessorHandler) {
	n.nextHandler = next
}

func (n *AbstractManager) NextHandler() AccessorHandler {
	return n.nextHandler
}

func (n *AbstractManager) ListEnvironments() []ExecutionEnvironment {
	return []ExecutionEnvironment{}
}

func (n *AbstractManager) GetEnvironment(_ dto.EnvironmentID) (ExecutionEnvironment, bool) {
	return nil, false
}

func (n *AbstractManager) StoreEnvironment(_ ExecutionEnvironment) {}

func (n *AbstractManager) DeleteEnvironment(_ dto.EnvironmentID) {}

func (n *AbstractManager) EnvironmentStatistics() map[dto.EnvironmentID]*dto.StatisticalExecutionEnvironmentData {
	return map[dto.EnvironmentID]*dto.StatisticalExecutionEnvironmentData{}
}

func (n *AbstractManager) Claim(_ dto.EnvironmentID, _ int) (Runner, error) {
	return nil, ErrNullObject
}

func (n *AbstractManager) Get(_ string) (Runner, error) {
	return nil, ErrNullObject
}

func (n *AbstractManager) Return(_ Runner) error {
	return nil
}

func (n *AbstractManager) Load() {}
