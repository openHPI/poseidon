package main

import (
	"context"
	"github.com/openHPI/poseidon/internal/environment"
	"github.com/openHPI/poseidon/internal/runner"
	"github.com/openHPI/poseidon/tests"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"syscall"
	"testing"
	"time"
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

func TestShutdownOnOSSignal_Profiling(t *testing.T) {
	called := false

	server := initServer()
	go shutdownOnOSSignal(server, context.Background(), func() {
		called = true
	})

	<-time.After(tests.ShortTimeout)
	err := syscall.Kill(syscall.Getpid(), syscall.SIGUSR1)
	require.NoError(t, err)
	<-time.After(tests.ShortTimeout)

	assert.True(t, called)
}
