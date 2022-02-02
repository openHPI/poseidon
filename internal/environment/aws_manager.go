package environment

import (
	"fmt"
	"github.com/openHPI/poseidon/internal/config"
	"github.com/openHPI/poseidon/internal/runner"
	"github.com/openHPI/poseidon/pkg/dto"
	"strings"
)

// AWSEnvironmentManager contains no functionality at the moment.
// IMPROVE: Create Lambda functions dynamically.
type AWSEnvironmentManager struct {
	*AbstractManager
	runnerManager runner.Manager
}

func NewAWSEnvironmentManager(runnerManager runner.Manager) *AWSEnvironmentManager {
	m := &AWSEnvironmentManager{&AbstractManager{nil}, runnerManager}
	runnerManager.Load()
	return m
}

func (a *AWSEnvironmentManager) List(fetch bool) ([]runner.ExecutionEnvironment, error) {
	list, err := a.NextHandler().List(fetch)
	if err != nil {
		return nil, fmt.Errorf("aws wrapped: %w", err)
	}
	return append(list, a.runnerManager.ListEnvironments()...), nil
}

func (a *AWSEnvironmentManager) Get(id dto.EnvironmentID, fetch bool) (runner.ExecutionEnvironment, error) {
	e, ok := a.runnerManager.GetEnvironment(id)
	if ok {
		return e, nil
	} else {
		e, err := a.NextHandler().Get(id, fetch)
		if err != nil {
			return nil, fmt.Errorf("aws wrapped: %w", err)
		}
		return e, nil
	}
}

func (a *AWSEnvironmentManager) CreateOrUpdate(
	id dto.EnvironmentID, request dto.ExecutionEnvironmentRequest) (bool, error) {
	if !isAWSEnvironment(request) {
		isCreated, err := a.NextHandler().CreateOrUpdate(id, request)
		if err != nil {
			return false, fmt.Errorf("aws wrapped: %w", err)
		}
		return isCreated, nil
	}

	_, ok := a.runnerManager.GetEnvironment(id)
	e := NewAWSEnvironment()
	e.SetID(id)
	e.SetImage(request.Image)
	a.runnerManager.StoreEnvironment(e)
	return !ok, nil
}

func isAWSEnvironment(request dto.ExecutionEnvironmentRequest) bool {
	for _, function := range strings.Fields(config.Config.AWS.Functions) {
		if request.Image == function {
			return true
		}
	}
	return false
}

func (a *AWSEnvironmentManager) Delete(id dto.EnvironmentID) (bool, error) {
	e, ok := a.runnerManager.GetEnvironment(id)
	if !ok {
		isFound, err := a.NextHandler().Delete(id)
		if err != nil {
			return false, fmt.Errorf("aws wrapped: %w", err)
		}
		return isFound, nil
	}

	a.runnerManager.DeleteEnvironment(id)
	if err := e.Delete(); err != nil {
		return true, fmt.Errorf("could not delete environment: %w", err)
	}
	return true, nil
}

func (a *AWSEnvironmentManager) Statistics() map[dto.EnvironmentID]*dto.StatisticalExecutionEnvironmentData {
	return a.NextHandler().Statistics()
}
