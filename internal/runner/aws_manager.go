package runner

import (
	"fmt"
	"github.com/openHPI/poseidon/pkg/dto"
)

type AWSRunnerManager struct {
	*AbstractManager
}

// NewAWSRunnerManager creates a new runner manager that keeps track of all runners at AWS.
func NewAWSRunnerManager() *AWSRunnerManager {
	return &AWSRunnerManager{&AbstractManager{}}
}

func (a AWSRunnerManager) ListEnvironments() []ExecutionEnvironment {
	return []ExecutionEnvironment{}
}

func (a AWSRunnerManager) GetEnvironment(_ dto.EnvironmentID) (ExecutionEnvironment, bool) {
	return nil, false
}

func (a AWSRunnerManager) StoreEnvironment(_ ExecutionEnvironment) {}

func (a AWSRunnerManager) DeleteEnvironment(_ dto.EnvironmentID) {}

func (a AWSRunnerManager) EnvironmentStatistics() map[dto.EnvironmentID]*dto.StatisticalExecutionEnvironmentData {
	return map[dto.EnvironmentID]*dto.StatisticalExecutionEnvironmentData{}
}

func (a AWSRunnerManager) Claim(id dto.EnvironmentID, duration int) (Runner, error) {
	r, err := a.NextHandler().Claim(id, duration)
	if err != nil {
		return nil, fmt.Errorf("aws wraped: %w", err)
	}
	return r, nil
}

func (a AWSRunnerManager) Get(runnerID string) (Runner, error) {
	r, err := a.NextHandler().Get(runnerID)
	if err != nil {
		return nil, fmt.Errorf("aws wraped: %w", err)
	}
	return r, nil
}

func (a AWSRunnerManager) Return(r Runner) error {
	err := a.NextHandler().Return(r)
	if err != nil {
		return fmt.Errorf("aws wraped: %w", err)
	}
	return nil
}

func (a AWSRunnerManager) Load() {}
