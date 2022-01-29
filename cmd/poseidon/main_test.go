package main

import (
	"github.com/openHPI/poseidon/internal/environment"
	"github.com/openHPI/poseidon/internal/runner"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestAWSDisabledUsesNomadManager(t *testing.T) {
	runnerManager, environmentManager := createManagerHandler(createNomadManager, true,
		runner.NewAbstractManager(), &environment.AbstractManager{})
	awsRunnerManager, awsEnvironmentManager := createManagerHandler(createAWSManager, false,
		runnerManager, environmentManager)
	assert.Equal(t, runnerManager, awsRunnerManager)
	assert.Equal(t, environmentManager, awsEnvironmentManager)
}

func TestAWSEnabledWrappesNomadManager(t *testing.T) {
	runnerManager, environmentManager := createManagerHandler(createNomadManager, true,
		runner.NewAbstractManager(), &environment.AbstractManager{})
	awsRunnerManager, awsEnvironmentManager := createManagerHandler(createAWSManager,
		true, runnerManager, environmentManager)
	assert.NotEqual(t, runnerManager, awsRunnerManager)
	assert.NotEqual(t, environmentManager, awsEnvironmentManager)
}
