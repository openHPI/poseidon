package runner

import (
	"context"
	"errors"
	nomadApi "github.com/hashicorp/nomad/api"
	"github.com/sirupsen/logrus"
	"github.com/sirupsen/logrus/hooks/test"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/nomad"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/tests"
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
	var err error

	s.nomadRunnerManager, err = NewNomadRunnerManager(s.apiMock, ctx)
	s.Require().NoError(err)

	s.exerciseRunner = NewRunner(tests.DefaultRunnerID)
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
	apiMock.On("LoadAllJobs").Return([]*nomadApi.Job{}, nil)
	apiMock.On("MarkRunnerAsUsed", mock.AnythingOfType("string")).Return(nil)
	apiMock.On("LoadRunners", tests.DefaultJobID).Return(returnedRunnerIds, nil)
	apiMock.On("JobScale", tests.DefaultJobID).Return(uint(len(returnedRunnerIds)), nil)
	apiMock.On("SetJobScale", tests.DefaultJobID, mock.AnythingOfType("uint"), "Runner Requested").Return(nil)
	apiMock.On("LoadTemplateJob", mock.AnythingOfType("string")).Return(&nomadApi.Job{}, nil)
	apiMock.On("RegisterNomadJob", mock.Anything).Return("", nil)
	apiMock.On("MonitorEvaluation", mock.Anything, mock.Anything).Return(nil)
}

func (s *ManagerTestSuite) registerDefaultEnvironment() {
	err := s.nomadRunnerManager.registerEnvironment(defaultEnvironmentID, 0)
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
	err := s.nomadRunnerManager.registerEnvironment(anotherEnvironmentID, defaultDesiredRunnersCount)
	s.Require().NoError(err)
	job, ok := s.nomadRunnerManager.environments.Get(defaultEnvironmentID)
	s.True(ok)
	s.NotNil(job)
}

func (s *ManagerTestSuite) TestClaimReturnsNotFoundErrorIfEnvironmentNotFound() {
	runner, err := s.nomadRunnerManager.Claim(EnvironmentID(42))
	s.Nil(runner)
	s.Equal(ErrUnknownExecutionEnvironment, err)
}

func (s *ManagerTestSuite) TestClaimReturnsRunnerIfAvailable() {
	s.AddIdleRunnerForDefaultEnvironment(s.exerciseRunner)
	receivedRunner, err := s.nomadRunnerManager.Claim(defaultEnvironmentID)
	s.NoError(err)
	s.Equal(s.exerciseRunner, receivedRunner)
}

func (s *ManagerTestSuite) TestClaimReturnsErrorIfNoRunnerAvailable() {
	s.waitForRunnerRefresh()
	runner, err := s.nomadRunnerManager.Claim(defaultEnvironmentID)
	s.Nil(runner)
	s.Equal(ErrNoRunnersAvailable, err)
}

func (s *ManagerTestSuite) TestClaimReturnsNoRunnerOfDifferentEnvironment() {
	s.AddIdleRunnerForDefaultEnvironment(s.exerciseRunner)
	receivedRunner, err := s.nomadRunnerManager.Claim(anotherEnvironmentID)
	s.Nil(receivedRunner)
	s.Error(err)
}

func (s *ManagerTestSuite) TestClaimDoesNotReturnTheSameRunnerTwice() {
	s.AddIdleRunnerForDefaultEnvironment(s.exerciseRunner)
	s.AddIdleRunnerForDefaultEnvironment(NewRunner(tests.AnotherRunnerID))

	firstReceivedRunner, err := s.nomadRunnerManager.Claim(defaultEnvironmentID)
	s.NoError(err)
	secondReceivedRunner, err := s.nomadRunnerManager.Claim(defaultEnvironmentID)
	s.NoError(err)
	s.NotEqual(firstReceivedRunner, secondReceivedRunner)
}

func (s *ManagerTestSuite) TestClaimThrowsAnErrorIfNoRunnersAvailable() {
	receivedRunner, err := s.nomadRunnerManager.Claim(defaultEnvironmentID)
	s.Nil(receivedRunner)
	s.Error(err)
}

func (s *ManagerTestSuite) TestClaimAddsRunnerToUsedRunners() {
	s.AddIdleRunnerForDefaultEnvironment(s.exerciseRunner)
	receivedRunner, _ := s.nomadRunnerManager.Claim(defaultEnvironmentID)
	savedRunner, ok := s.nomadRunnerManager.usedRunners.Get(receivedRunner.Id())
	s.True(ok)
	s.Equal(savedRunner, receivedRunner)
}

func (s *ManagerTestSuite) TestGetReturnsRunnerIfRunnerIsUsed() {
	s.nomadRunnerManager.usedRunners.Add(s.exerciseRunner)
	savedRunner, err := s.nomadRunnerManager.Get(s.exerciseRunner.Id())
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
	_, ok := s.nomadRunnerManager.usedRunners.Get(s.exerciseRunner.Id())
	s.False(ok)
}

func (s *ManagerTestSuite) TestReturnCallsDeleteRunnerApiMethod() {
	s.apiMock.On("DeleteRunner", mock.AnythingOfType("string")).Return(nil)
	err := s.nomadRunnerManager.Return(s.exerciseRunner)
	s.Nil(err)
	s.apiMock.AssertCalled(s.T(), "DeleteRunner", s.exerciseRunner.Id())
}

func (s *ManagerTestSuite) TestReturnReturnsErrorWhenApiCallFailed() {
	s.apiMock.On("DeleteRunner", mock.AnythingOfType("string")).Return(errors.New("return failed"))
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
	go s.nomadRunnerManager.updateRunners(ctx)
	<-time.After(10 * time.Millisecond)

	s.Require().Equal(1, len(hook.Entries))
	s.Equal(logrus.ErrorLevel, hook.LastEntry().Level)
	s.Equal(hook.LastEntry().Data[logrus.ErrorKey], tests.ErrDefault)
}

func (s *ManagerTestSuite) TestUpdateRunnersAddsIdleRunner() {
	allocation := &nomadApi.Allocation{ID: tests.DefaultRunnerID}
	defaultJob, ok := s.nomadRunnerManager.environments.Get(defaultEnvironmentID)
	s.Require().True(ok)
	allocation.JobID = defaultJob.environmentID.toString()

	_, ok = defaultJob.idleRunners.Get(allocation.ID)
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
	go s.nomadRunnerManager.updateRunners(ctx)
	<-time.After(10 * time.Millisecond)

	_, ok = defaultJob.idleRunners.Get(allocation.ID)
	s.True(ok)
}

func (s *ManagerTestSuite) TestUpdateRunnersRemovesIdleAndUsedRunner() {
	allocation := &nomadApi.Allocation{ID: tests.DefaultRunnerID}
	defaultJob, ok := s.nomadRunnerManager.environments.Get(defaultEnvironmentID)
	s.Require().True(ok)
	allocation.JobID = defaultJob.environmentID.toString()

	testRunner := NewRunner(allocation.ID)
	defaultJob.idleRunners.Add(testRunner)
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
	go s.nomadRunnerManager.updateRunners(ctx)
	<-time.After(10 * time.Millisecond)

	_, ok = defaultJob.idleRunners.Get(allocation.ID)
	s.False(ok)
	_, ok = s.nomadRunnerManager.usedRunners.Get(allocation.ID)
	s.False(ok)
}

func modifyMockedCall(apiMock *nomad.ExecutorAPIMock, method string, modifier func(call *mock.Call)) {
	for _, c := range apiMock.ExpectedCalls {
		if c.Method == method {
			modifier(c)
		}
	}
}
