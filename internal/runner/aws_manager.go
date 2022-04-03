package runner

import (
	"fmt"
	"github.com/openHPI/poseidon/pkg/dto"
	"time"
)

type AWSRunnerManager struct {
	*AbstractManager
}

// NewAWSRunnerManager creates a new runner manager that keeps track of all runners at AWS.
func NewAWSRunnerManager() *AWSRunnerManager {
	return &AWSRunnerManager{NewAbstractManager()}
}

func (a AWSRunnerManager) Claim(id dto.EnvironmentID, duration int) (Runner, error) {
	environment, ok := a.environments.Get(id)
	if !ok {
		r, err := a.NextHandler().Claim(id, duration)
		if err != nil {
			return nil, fmt.Errorf("aws wrapped: %w", err)
		}
		return r, nil
	}

	runner, ok := environment.Sample()
	if !ok {
		log.Warn("no aws runner available")
		return nil, ErrNoRunnersAvailable
	}

	a.usedRunners.Add(runner)
	runner.SetupTimeout(time.Duration(duration) * time.Second)
	return runner, nil
}

func (a AWSRunnerManager) Return(r Runner) error {
	_, isAWSRunner := r.(*AWSFunctionWorkload)
	if isAWSRunner {
		a.usedRunners.Delete(r.ID())
	} else if err := a.NextHandler().Return(r); err != nil {
		return fmt.Errorf("aws wrapped: %w", err)
	}
	return nil
}

// EnvironmentStatistics returns only the used runner for each environment as the prewarming is handled
// by AWS transparently.
func (a AWSRunnerManager) EnvironmentStatistics() map[dto.EnvironmentID]*dto.StatisticalExecutionEnvironmentData {
	environments := a.AbstractManager.EnvironmentStatistics()

	for _, r := range a.usedRunners.List() {
		workload, isAWSRunner := r.(*AWSFunctionWorkload)
		if !isAWSRunner {
			log.WithField("workload", workload).Error("Stored runners must be AWS runner")
			continue
		}

		environmentID := workload.environment.ID()
		environments[environmentID].UsedRunners++
	}

	return environments
}
