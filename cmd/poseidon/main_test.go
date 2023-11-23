package main

import (
	"context"
	"github.com/openHPI/poseidon/internal/environment"
	"github.com/openHPI/poseidon/internal/runner"
	"github.com/openHPI/poseidon/tests"
	"github.com/stretchr/testify/suite"
	"golang.org/x/sys/unix"
	"testing"
	"time"
)

type MainTestSuite struct {
	tests.MemoryLeakTestSuite
}

func TestMainTestSuite(t *testing.T) {
	suite.Run(t, new(MainTestSuite))
}

func (s *MainTestSuite) TestAWSDisabledUsesNomadManager() {
	disableRecovery, cancel := context.WithCancel(context.Background())
	cancel()

	runnerManager, environmentManager := createManagerHandler(createNomadManager, true,
		runner.NewAbstractManager(s.TestCtx), &environment.AbstractManager{}, disableRecovery)
	awsRunnerManager, awsEnvironmentManager := createManagerHandler(createAWSManager, false,
		runnerManager, environmentManager, s.TestCtx)
	s.Equal(runnerManager, awsRunnerManager)
	s.Equal(environmentManager, awsEnvironmentManager)
}

func (s *MainTestSuite) TestAWSEnabledWrappesNomadManager() {
	disableRecovery, cancel := context.WithCancel(context.Background())
	cancel()

	runnerManager, environmentManager := createManagerHandler(createNomadManager, true,
		runner.NewAbstractManager(s.TestCtx), &environment.AbstractManager{}, disableRecovery)
	awsRunnerManager, awsEnvironmentManager := createManagerHandler(createAWSManager,
		true, runnerManager, environmentManager, s.TestCtx)
	s.NotEqual(runnerManager, awsRunnerManager)
	s.NotEqual(environmentManager, awsEnvironmentManager)
}

func (s *MainTestSuite) TestShutdownOnOSSignal_Profiling() {
	called := false
	disableRecovery, cancel := context.WithCancel(context.Background())
	cancel()

	s.ExpectedGoroutingIncrease++ // The shutdownOnOSSignal waits for an exit after stopping the profiling.
	s.ExpectedGoroutingIncrease++ // The shutdownOnOSSignal triggers a os.Signal Goroutine.

	server := initServer(disableRecovery)
	go shutdownOnOSSignal(server, context.Background(), func() {
		called = true
	})

	<-time.After(tests.ShortTimeout)
	err := unix.Kill(unix.Getpid(), unix.SIGUSR1)
	s.Require().NoError(err)
	<-time.After(tests.ShortTimeout)

	s.True(called)
}
