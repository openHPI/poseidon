package runner

import (
	"context"
	nomadApi "github.com/hashicorp/nomad/api"
	"github.com/sirupsen/logrus"
	"github.com/sirupsen/logrus/hooks/test"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/internal/nomad"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/tests"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/tests/helpers"
	"strconv"
	"testing"
	"time"
)

const (
	defaultDesiredRunnersCount uint = 5
)

func TestGetNextRunnerTestSuite(t *testing.T) {
	suite.Run(t, new(ManagerTestSuite))
}

type ManagerTestSuite struct {
	suite.Suite
	apiMock            *nomad.ExecutorAPIMock
	nomadRunnerManager *NomadRunnerManager
	exerciseRunner     Runner
}

func (s *ManagerTestSuite) SetupTest() {
	s.apiMock = &nomad.ExecutorAPIMock{}
	mockRunnerQueries(s.apiMock, []string{})
	// Instantly closed context to manually start the update process in some cases
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	s.nomadRunnerManager = NewNomadRunnerManager(s.apiMock, ctx)

	s.exerciseRunner = NewRunner(tests.DefaultRunnerID, s.nomadRunnerManager)
	s.registerDefaultEnvironment()
}

func mockRunnerQueries(apiMock *nomad.ExecutorAPIMock, returnedRunnerIds []string) {
	// reset expected calls to allow new mocked return values
	apiMock.ExpectedCalls = []*mock.Call{}
	call := apiMock.On("WatchAllocations", mock.Anything, mock.Anything, mock.Anything)
	call.Run(func(args mock.Arguments) {
		<-time.After(10 * time.Minute) // 10 minutes is the default test timeout
		call.ReturnArguments = mock.Arguments{nil}
	})
	apiMock.On("LoadEnvironmentJobs").Return([]*nomadApi.Job{}, nil)
	apiMock.On("MarkRunnerAsUsed", mock.AnythingOfType("string"), mock.AnythingOfType("int")).Return(nil)
	apiMock.On("LoadRunnerIDs", tests.DefaultJobID).Return(returnedRunnerIds, nil)
	apiMock.On("JobScale", tests.DefaultJobID).Return(uint(len(returnedRunnerIds)), nil)
	apiMock.On("SetJobScale", tests.DefaultJobID, mock.AnythingOfType("uint"), "Runner Requested").Return(nil)
	apiMock.On("RegisterRunnerJob", mock.Anything).Return(nil)
	apiMock.On("MonitorEvaluation", mock.Anything, mock.Anything).Return(nil)
}

func (s *ManagerTestSuite) registerDefaultEnvironment() {
	err := s.nomadRunnerManager.registerEnvironment(defaultEnvironmentID, 0, &nomadApi.Job{}, true)
	s.Require().NoError(err)
}

func (s *ManagerTestSuite) AddIdleRunnerForDefaultEnvironment(r Runner) {
	job, _ := s.nomadRunnerManager.environments.Get(defaultEnvironmentID)
	job.idleRunners.Add(r)
}

func (s *ManagerTestSuite) waitForRunnerRefresh() {
	<-time.After(100 * time.Millisecond)
}

func (s *ManagerTestSuite) TestRegisterEnvironmentAddsNewJob() {
	err := s.nomadRunnerManager.
		registerEnvironment(anotherEnvironmentID, defaultDesiredRunnersCount, &nomadApi.Job{}, true)
	s.Require().NoError(err)
	job, ok := s.nomadRunnerManager.environments.Get(defaultEnvironmentID)
	s.True(ok)
	s.NotNil(job)
}

func (s *ManagerTestSuite) TestClaimReturnsNotFoundErrorIfEnvironmentNotFound() {
	runner, err := s.nomadRunnerManager.Claim(EnvironmentID(42), defaultInactivityTimeout)
	s.Nil(runner)
	s.Equal(ErrUnknownExecutionEnvironment, err)
}

func (s *ManagerTestSuite) TestClaimReturnsRunnerIfAvailable() {
	s.AddIdleRunnerForDefaultEnvironment(s.exerciseRunner)
	receivedRunner, err := s.nomadRunnerManager.Claim(defaultEnvironmentID, defaultInactivityTimeout)
	s.NoError(err)
	s.Equal(s.exerciseRunner, receivedRunner)
}

func (s *ManagerTestSuite) TestClaimReturnsErrorIfNoRunnerAvailable() {
	s.waitForRunnerRefresh()
	runner, err := s.nomadRunnerManager.Claim(defaultEnvironmentID, defaultInactivityTimeout)
	s.Nil(runner)
	s.Equal(ErrNoRunnersAvailable, err)
}

func (s *ManagerTestSuite) TestClaimReturnsNoRunnerOfDifferentEnvironment() {
	s.AddIdleRunnerForDefaultEnvironment(s.exerciseRunner)
	receivedRunner, err := s.nomadRunnerManager.Claim(anotherEnvironmentID, defaultInactivityTimeout)
	s.Nil(receivedRunner)
	s.Error(err)
}

func (s *ManagerTestSuite) TestClaimDoesNotReturnTheSameRunnerTwice() {
	s.AddIdleRunnerForDefaultEnvironment(s.exerciseRunner)
	s.AddIdleRunnerForDefaultEnvironment(NewRunner(tests.AnotherRunnerID, s.nomadRunnerManager))

	firstReceivedRunner, err := s.nomadRunnerManager.Claim(defaultEnvironmentID, defaultInactivityTimeout)
	s.NoError(err)
	secondReceivedRunner, err := s.nomadRunnerManager.Claim(defaultEnvironmentID, defaultInactivityTimeout)
	s.NoError(err)
	s.NotEqual(firstReceivedRunner, secondReceivedRunner)
}

func (s *ManagerTestSuite) TestClaimThrowsAnErrorIfNoRunnersAvailable() {
	receivedRunner, err := s.nomadRunnerManager.Claim(defaultEnvironmentID, defaultInactivityTimeout)
	s.Nil(receivedRunner)
	s.Error(err)
}

func (s *ManagerTestSuite) TestClaimAddsRunnerToUsedRunners() {
	s.AddIdleRunnerForDefaultEnvironment(s.exerciseRunner)
	receivedRunner, err := s.nomadRunnerManager.Claim(defaultEnvironmentID, defaultInactivityTimeout)
	s.Require().NoError(err)
	savedRunner, ok := s.nomadRunnerManager.usedRunners.Get(receivedRunner.ID())
	s.True(ok)
	s.Equal(savedRunner, receivedRunner)
}

func (s *ManagerTestSuite) TestTwoClaimsAddExactlyTwoRunners() {
	s.AddIdleRunnerForDefaultEnvironment(s.exerciseRunner)
	s.AddIdleRunnerForDefaultEnvironment(NewRunner(tests.AnotherRunnerID, s.nomadRunnerManager))
	_, err := s.nomadRunnerManager.Claim(defaultEnvironmentID, defaultInactivityTimeout)
	s.Require().NoError(err)
	_, err = s.nomadRunnerManager.Claim(defaultEnvironmentID, defaultInactivityTimeout)
	s.Require().NoError(err)
	s.apiMock.AssertNumberOfCalls(s.T(), "RegisterRunnerJob", 2)
}

func (s *ManagerTestSuite) TestGetReturnsRunnerIfRunnerIsUsed() {
	s.nomadRunnerManager.usedRunners.Add(s.exerciseRunner)
	savedRunner, err := s.nomadRunnerManager.Get(s.exerciseRunner.ID())
	s.NoError(err)
	s.Equal(savedRunner, s.exerciseRunner)
}

func (s *ManagerTestSuite) TestGetReturnsErrorIfRunnerNotFound() {
	savedRunner, err := s.nomadRunnerManager.Get(tests.DefaultRunnerID)
	s.Nil(savedRunner)
	s.Error(err)
}

func (s *ManagerTestSuite) TestReturnRemovesRunnerFromUsedRunners() {
	s.apiMock.On("DeleteRunner", mock.AnythingOfType("string")).Return(nil)
	s.nomadRunnerManager.usedRunners.Add(s.exerciseRunner)
	err := s.nomadRunnerManager.Return(s.exerciseRunner)
	s.Nil(err)
	_, ok := s.nomadRunnerManager.usedRunners.Get(s.exerciseRunner.ID())
	s.False(ok)
}

func (s *ManagerTestSuite) TestReturnCallsDeleteRunnerApiMethod() {
	s.apiMock.On("DeleteRunner", mock.AnythingOfType("string")).Return(nil)
	err := s.nomadRunnerManager.Return(s.exerciseRunner)
	s.Nil(err)
	s.apiMock.AssertCalled(s.T(), "DeleteRunner", s.exerciseRunner.ID())
}

func (s *ManagerTestSuite) TestReturnReturnsErrorWhenApiCallFailed() {
	s.apiMock.On("DeleteRunner", mock.AnythingOfType("string")).Return(tests.ErrDefault)
	err := s.nomadRunnerManager.Return(s.exerciseRunner)
	s.Error(err)
}

func (s *ManagerTestSuite) TestUpdateRunnersLogsErrorFromWatchAllocation() {
	var hook *test.Hook
	logger, hook := test.NewNullLogger()
	log = logger.WithField("pkg", "runner")
	modifyMockedCall(s.apiMock, "WatchAllocations", func(call *mock.Call) {
		call.Run(func(args mock.Arguments) {
			call.ReturnArguments = mock.Arguments{tests.ErrDefault}
		})
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go s.nomadRunnerManager.keepRunnersSynced(ctx)
	<-time.After(10 * time.Millisecond)

	s.Require().Equal(1, len(hook.Entries))
	s.Equal(logrus.ErrorLevel, hook.LastEntry().Level)
	s.Equal(hook.LastEntry().Data[logrus.ErrorKey], tests.ErrDefault)
}

func (s *ManagerTestSuite) TestUpdateRunnersAddsIdleRunner() {
	allocation := &nomadApi.Allocation{ID: tests.DefaultRunnerID}
	environment, ok := s.nomadRunnerManager.environments.Get(defaultEnvironmentID)
	s.Require().True(ok)
	allocation.JobID = environment.environmentID.toString()

	_, ok = environment.idleRunners.Get(allocation.ID)
	s.Require().False(ok)

	modifyMockedCall(s.apiMock, "WatchAllocations", func(call *mock.Call) {
		call.Run(func(args mock.Arguments) {
			onCreate, ok := args.Get(1).(nomad.AllocationProcessor)
			s.Require().True(ok)
			onCreate(allocation)
			call.ReturnArguments = mock.Arguments{nil}
		})
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go s.nomadRunnerManager.keepRunnersSynced(ctx)
	<-time.After(10 * time.Millisecond)

	_, ok = environment.idleRunners.Get(allocation.JobID)
	s.True(ok)
}

func (s *ManagerTestSuite) TestUpdateRunnersRemovesIdleAndUsedRunner() {
	allocation := &nomadApi.Allocation{JobID: tests.DefaultJobID}
	environment, ok := s.nomadRunnerManager.environments.Get(defaultEnvironmentID)
	s.Require().True(ok)

	testRunner := NewRunner(allocation.JobID, s.nomadRunnerManager)
	environment.idleRunners.Add(testRunner)
	s.nomadRunnerManager.usedRunners.Add(testRunner)

	modifyMockedCall(s.apiMock, "WatchAllocations", func(call *mock.Call) {
		call.Run(func(args mock.Arguments) {
			onDelete, ok := args.Get(2).(nomad.AllocationProcessor)
			s.Require().True(ok)
			onDelete(allocation)
			call.ReturnArguments = mock.Arguments{nil}
		})
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go s.nomadRunnerManager.keepRunnersSynced(ctx)
	<-time.After(10 * time.Millisecond)

	_, ok = environment.idleRunners.Get(allocation.JobID)
	s.False(ok)
	_, ok = s.nomadRunnerManager.usedRunners.Get(allocation.JobID)
	s.False(ok)
}

func (s *ManagerTestSuite) TestUpdateEnvironmentRemovesIdleRunnersWhenScalingDown() {
	_, job := helpers.CreateTemplateJob()
	initialRunners := uint(40)
	updatedRunners := uint(10)
	err := s.nomadRunnerManager.registerEnvironment(anotherEnvironmentID, initialRunners, job, true)
	s.Require().NoError(err)
	s.apiMock.AssertNumberOfCalls(s.T(), "RegisterRunnerJob", int(initialRunners))
	environment, ok := s.nomadRunnerManager.environments.Get(anotherEnvironmentID)
	s.Require().True(ok)
	for i := 0; i < int(initialRunners); i++ {
		environment.idleRunners.Add(NewRunner("active-runner-"+strconv.Itoa(i), s.nomadRunnerManager))
	}

	s.apiMock.On("LoadRunnerIDs", anotherEnvironmentID.toString()).Return([]string{}, nil)
	s.apiMock.On("DeleteRunner", mock.AnythingOfType("string")).Return(nil)

	err = s.nomadRunnerManager.updateEnvironment(tests.AnotherEnvironmentIDAsInteger, updatedRunners, job, true)
	s.Require().NoError(err)
	s.apiMock.AssertNumberOfCalls(s.T(), "DeleteRunner", int(initialRunners-updatedRunners))
}

func modifyMockedCall(apiMock *nomad.ExecutorAPIMock, method string, modifier func(call *mock.Call)) {
	for _, c := range apiMock.ExpectedCalls {
		if c.Method == method {
			modifier(c)
		}
	}
}

func (s *ManagerTestSuite) TestOnAllocationAdded() {
	s.registerDefaultEnvironment()
	s.Run("does not add environment template id job", func() {
		alloc := &nomadApi.Allocation{JobID: TemplateJobID(tests.DefaultEnvironmentIDAsInteger)}
		s.nomadRunnerManager.onAllocationAdded(alloc)
		job, ok := s.nomadRunnerManager.environments.Get(tests.DefaultEnvironmentIDAsInteger)
		s.True(ok)
		s.Zero(job.idleRunners.Length())
	})
	s.Run("does not panic when environment id cannot be parsed", func() {
		alloc := &nomadApi.Allocation{JobID: ""}
		s.NotPanics(func() {
			s.nomadRunnerManager.onAllocationAdded(alloc)
		})
	})
	s.Run("does not panic when environment does not exist", func() {
		nonExistentEnvironment := EnvironmentID(1234)
		_, ok := s.nomadRunnerManager.environments.Get(nonExistentEnvironment)
		s.Require().False(ok)

		alloc := &nomadApi.Allocation{JobID: RunnerJobID(nonExistentEnvironment, "1-1-1-1")}
		s.NotPanics(func() {
			s.nomadRunnerManager.onAllocationAdded(alloc)
		})
	})
	s.Run("adds correct job", func() {
		s.Run("without allocated resources", func() {
			alloc := &nomadApi.Allocation{
				JobID:              tests.DefaultJobID,
				AllocatedResources: nil,
			}
			s.nomadRunnerManager.onAllocationAdded(alloc)
			job, ok := s.nomadRunnerManager.environments.Get(tests.DefaultEnvironmentIDAsInteger)
			s.True(ok)
			runner, ok := job.idleRunners.Get(tests.DefaultJobID)
			s.True(ok)
			nomadJob, ok := runner.(*NomadJob)
			s.True(ok)
			s.Equal(nomadJob.id, tests.DefaultJobID)
			s.Empty(nomadJob.portMappings)
		})
		s.Run("with mapped ports", func() {
			alloc := &nomadApi.Allocation{
				JobID: tests.DefaultJobID,
				AllocatedResources: &nomadApi.AllocatedResources{
					Shared: nomadApi.AllocatedSharedResources{Ports: tests.DefaultPortMappings},
				},
			}
			s.nomadRunnerManager.onAllocationAdded(alloc)
			job, ok := s.nomadRunnerManager.environments.Get(tests.DefaultEnvironmentIDAsInteger)
			s.True(ok)
			runner, ok := job.idleRunners.Get(tests.DefaultJobID)
			s.True(ok)
			nomadJob, ok := runner.(*NomadJob)
			s.True(ok)
			s.Equal(nomadJob.id, tests.DefaultJobID)
			s.Equal(nomadJob.portMappings, tests.DefaultPortMappings)
		})
	})
}
