package environment

import (
	"fmt"
	"github.com/openHPI/poseidon/internal/runner"
	"github.com/openHPI/poseidon/pkg/dto"
)

// AWSEnvironmentManager contains no functionality at the moment.
// IMPROVE: Create Lambda functions dynamically.
type AWSEnvironmentManager struct {
	*AbstractManager
	runnerManager runner.Accessor
}

func NewAWSEnvironmentManager(runnerManager runner.Accessor) *AWSEnvironmentManager {
	m := &AWSEnvironmentManager{&AbstractManager{nil}, runnerManager}
	runnerManager.Load()
	return m
}

func (a *AWSEnvironmentManager) List(fetch bool) ([]runner.ExecutionEnvironment, error) {
	list, err := a.NextHandler().List(fetch)
	if err != nil {
		return nil, fmt.Errorf("aws wraped: %w", err)
	}
	return list, nil
}

func (a *AWSEnvironmentManager) Get(id dto.EnvironmentID, fetch bool) (runner.ExecutionEnvironment, error) {
	e, err := a.NextHandler().Get(id, fetch)
	if err != nil {
		return nil, fmt.Errorf("aws wraped: %w", err)
	}
	return e, nil
}

func (a *AWSEnvironmentManager) CreateOrUpdate(
	id dto.EnvironmentID, request dto.ExecutionEnvironmentRequest) (bool, error) {
	isCreated, err := a.NextHandler().CreateOrUpdate(id, request)
	if err != nil {
		return false, fmt.Errorf("aws wraped: %w", err)
	}
	return isCreated, nil
}

func (a *AWSEnvironmentManager) Delete(id dto.EnvironmentID) (bool, error) {
	isFound, err := a.NextHandler().Delete(id)
	if err != nil {
		return false, fmt.Errorf("aws wraped: %w", err)
	}
	return isFound, nil
}

func (a *AWSEnvironmentManager) Statistics() map[dto.EnvironmentID]*dto.StatisticalExecutionEnvironmentData {
	return a.NextHandler().Statistics()
}
