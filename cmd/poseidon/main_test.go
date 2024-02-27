package main

import (
	"context"
	"github.com/hashicorp/nomad/api"
	"github.com/openHPI/poseidon/internal/environment"
	"github.com/openHPI/poseidon/internal/nomad"
	"github.com/openHPI/poseidon/internal/runner"
	"github.com/openHPI/poseidon/tests"
	"github.com/stretchr/testify/mock"
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

	s.ExpectedGoroutineIncrease++ // The shutdownOnOSSignal waits for an exit after stopping the profiling.
	s.ExpectedGoroutineIncrease++ // The shutdownOnOSSignal triggers a os.Signal Goroutine.

	server := initServer(initRouter(disableRecovery))
	go shutdownOnOSSignal(server, context.Background(), func() {
		called = true
	})

	<-time.After(tests.ShortTimeout)
	err := unix.Kill(unix.Getpid(), unix.SIGUSR1)
	s.Require().NoError(err)
	<-time.After(tests.ShortTimeout)

	s.True(called)
}

func (s *MainTestSuite) TestLoadNomadEnvironmentsBeforeStartingWebserver() {
	apiMock := &nomad.ExecutorAPIMock{}
	apiMock.On("LoadEnvironmentJobs").Return([]*api.Job{}, nil)
	apiMock.On("WatchEventStream", mock.Anything, mock.Anything).Run(func(_ mock.Arguments) {
		<-s.TestCtx.Done()
	}).Return(nil).Maybe()

	runnerManager := runner.NewNomadRunnerManager(apiMock, s.TestCtx)
	environmentManager, err := environment.NewNomadEnvironmentManager(runnerManager, apiMock, "")
	s.Require().NoError(err)

	synchronizeNomad(s.TestCtx, environmentManager, runnerManager)
	apiMock.AssertExpectations(s.T())
}
