package runner

import (
	"context"
	"fmt"
	"time"

	"github.com/openHPI/poseidon/pkg/dto"
)

type AWSRunnerManager struct {
	*AbstractManager
}

// NewAWSRunnerManager creates a new runner manager that keeps track of all runners at AWS.
func NewAWSRunnerManager(ctx context.Context) *AWSRunnerManager {
	return &AWSRunnerManager{NewAbstractManager(ctx)}
}

func (a AWSRunnerManager) Claim(id dto.EnvironmentID, duration int) (Runner, error) {
	environment, ok := a.GetEnvironment(id)
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

	a.usedRunners.Add(runner.ID(), runner)
	runner.SetupTimeout(time.Duration(duration) * time.Second)

	return runner, nil
}

func (a AWSRunnerManager) Return(r Runner) error {
	_, isAWSRunner := r.(*AWSFunctionWorkload)
	if isAWSRunner {
		a.usedRunners.Delete(r.ID())
	} else {
		err := a.NextHandler().Return(r)
		if err != nil {
			return fmt.Errorf("aws wrapped: %w", err)
		}
	}

	return nil
}
