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
	disableRecovery, cancel := context.WithCancel(context.Background())
	cancel()

	runnerManager, environmentManager := createManagerHandler(createNomadManager, true,
		runner.NewAbstractManager(disableRecovery), &environment.AbstractManager{}, disableRecovery)
	awsRunnerManager, awsEnvironmentManager := createManagerHandler(createAWSManager, false,
		runnerManager, environmentManager, disableRecovery)
	assert.Equal(t, runnerManager, awsRunnerManager)
	assert.Equal(t, environmentManager, awsEnvironmentManager)
}

func TestAWSEnabledWrappesNomadManager(t *testing.T) {
	disableRecovery, cancel := context.WithCancel(context.Background())
	cancel()

	runnerManager, environmentManager := createManagerHandler(createNomadManager, true,
		runner.NewAbstractManager(disableRecovery), &environment.AbstractManager{}, disableRecovery)
	awsRunnerManager, awsEnvironmentManager := createManagerHandler(createAWSManager,
		true, runnerManager, environmentManager, disableRecovery)
	assert.NotEqual(t, runnerManager, awsRunnerManager)
	assert.NotEqual(t, environmentManager, awsEnvironmentManager)
}

func TestShutdownOnOSSignal_Profiling(t *testing.T) {
	called := false
	disableRecovery, cancel := context.WithCancel(context.Background())
	cancel()

	server := initServer(disableRecovery)
	go shutdownOnOSSignal(server, context.Background(), func() {
		called = true
	})

	<-time.After(tests.ShortTimeout)
	err := syscall.Kill(syscall.Getpid(), syscall.SIGUSR1)
	require.NoError(t, err)
	<-time.After(tests.ShortTimeout)

	assert.True(t, called)
}
