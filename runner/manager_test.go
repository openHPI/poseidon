package runner

import (
	"context"
	"errors"
	nomadApi "github.com/hashicorp/nomad/api"
	"github.com/sirupsen/logrus"
	"github.com/sirupsen/logrus/hooks/test"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
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
	apiMock            *nomad.ExecutorApiMock
	nomadRunnerManager *NomadRunnerManager
	exerciseRunner     Runner
}

func (s *ManagerTestSuite) SetupTest() {
	s.apiMock = &nomad.ExecutorApiMock{}
	s.nomadRunnerManager = NewNomadRunnerManager(s.apiMock, context.Background())
	s.exerciseRunner = NewRunner(tests.DefaultRunnerId)
	s.mockRunnerQueries([]string{})
	s.registerDefaultEnvironment()
}

func (s *ManagerTestSuite) mockRunnerQueries(returnedRunnerIds []string) {
	// reset expected calls to allow new mocked return values
	s.apiMock.ExpectedCalls = []*mock.Call{}
	s.apiMock.On("WatchAllocations", mock.Anything, mock.Anything, mock.Anything).Return(nil)
	s.apiMock.On("LoadRunners", tests.DefaultJobId).Return(returnedRunnerIds, nil)
	s.apiMock.On("JobScale", tests.DefaultJobId).Return(len(returnedRunnerIds), nil)
	s.apiMock.On("SetJobScale", tests.DefaultJobId, mock.AnythingOfType("uint"), "Runner Requested").Return(nil)
}

func (s *ManagerTestSuite) registerDefaultEnvironment() {
	s.nomadRunnerManager.RegisterEnvironment(defaultEnvironmentId, tests.DefaultJobId, defaultDesiredRunnersCount)
}

func (s *ManagerTestSuite) AddIdleRunnerForDefaultEnvironment(r Runner) {
	job, _ := s.nomadRunnerManager.jobs.Get(defaultEnvironmentId)
	job.idleRunners.Add(r)
}

func (s *ManagerTestSuite) waitForRunnerRefresh() {
	time.Sleep(100 * time.Millisecond)
}

func (s *ManagerTestSuite) TestRegisterEnvironmentAddsNewJob() {
	s.nomadRunnerManager.RegisterEnvironment(anotherEnvironmentId, tests.DefaultJobId, defaultDesiredRunnersCount)
	job, ok := s.nomadRunnerManager.jobs.Get(defaultEnvironmentId)
	s.True(ok)
	s.NotNil(job)
}

func (s *ManagerTestSuite) TestClaimReturnsNotFoundErrorIfEnvironmentNotFound() {
	runner, err := s.nomadRunnerManager.Claim(EnvironmentId(42))
	s.Nil(runner)
	s.Equal(ErrUnknownExecutionEnvironment, err)
}

func (s *ManagerTestSuite) TestClaimReturnsRunnerIfAvailable() {
	s.AddIdleRunnerForDefaultEnvironment(s.exerciseRunner)
	receivedRunner, err := s.nomadRunnerManager.Claim(defaultEnvironmentId)
	s.NoError(err)
	s.Equal(s.exerciseRunner, receivedRunner)
}

func (s *ManagerTestSuite) TestClaimReturnsErrorIfNoRunnerAvailable() {
	s.waitForRunnerRefresh()
	runner, err := s.nomadRunnerManager.Claim(defaultEnvironmentId)
	s.Nil(runner)
	s.Equal(ErrNoRunnersAvailable, err)
}

func (s *ManagerTestSuite) TestClaimReturnsNoRunnerOfDifferentEnvironment() {
	s.AddIdleRunnerForDefaultEnvironment(s.exerciseRunner)
	receivedRunner, err := s.nomadRunnerManager.Claim(anotherEnvironmentId)
	s.Nil(receivedRunner)
	s.Error(err)
}

func (s *ManagerTestSuite) TestClaimDoesNotReturnTheSameRunnerTwice() {
	s.AddIdleRunnerForDefaultEnvironment(s.exerciseRunner)
	s.AddIdleRunnerForDefaultEnvironment(NewRunner(tests.AnotherRunnerId))

	firstReceivedRunner, err := s.nomadRunnerManager.Claim(defaultEnvironmentId)
	require.NoError(s.T(), err)
	secondReceivedRunner, err := s.nomadRunnerManager.Claim(defaultEnvironmentId)
	require.NoError(s.T(), err)
	s.NotEqual(firstReceivedRunner, secondReceivedRunner)
}

func (s *ManagerTestSuite) TestClaimThrowsAnErrorIfNoRunnersAvailable() {
	receivedRunner, err := s.nomadRunnerManager.Claim(defaultEnvironmentId)
	s.Nil(receivedRunner)
	s.Error(err)
}

func (s *ManagerTestSuite) TestClaimAddsRunnerToUsedRunners() {
	s.mockRunnerQueries([]string{tests.DefaultRunnerId})
	s.waitForRunnerRefresh()
	receivedRunner, err := s.nomadRunnerManager.Claim(defaultEnvironmentId)
	require.NoError(s.T(), err)
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
	savedRunner, err := s.nomadRunnerManager.Get(tests.DefaultRunnerId)
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

func (s *ManagerTestSuite) TestRefreshFetchesRunners() {
	s.mockRunnerQueries([]string{tests.DefaultRunnerId})
	s.waitForRunnerRefresh()
	s.apiMock.AssertCalled(s.T(), "LoadRunners", tests.DefaultJobId)
}

func (s *ManagerTestSuite) TestNewRunnersFoundInRefreshAreAddedToIdleRunners() {
	s.mockRunnerQueries([]string{tests.DefaultRunnerId})
	s.waitForRunnerRefresh()
	job, _ := s.nomadRunnerManager.jobs.Get(defaultEnvironmentId)
	_, ok := job.idleRunners.Get(tests.DefaultRunnerId)
	s.True(ok)
}

func (s *ManagerTestSuite) TestRefreshScalesJob() {
	s.mockRunnerQueries([]string{tests.DefaultRunnerId})
	s.waitForRunnerRefresh()
	// use one runner to necessitate rescaling
	_, _ = s.nomadRunnerManager.Claim(defaultEnvironmentId)
	s.waitForRunnerRefresh()
	s.apiMock.AssertCalled(s.T(), "SetJobScale", tests.DefaultJobId, defaultDesiredRunnersCount, "Runner Requested")
}

func (s *ManagerTestSuite) TestRefreshAddsRunnerToPool() {
	s.mockRunnerQueries([]string{tests.DefaultRunnerId})
	s.waitForRunnerRefresh()
	job, _ := s.nomadRunnerManager.jobs.Get(defaultEnvironmentId)
	poolRunner, ok := job.idleRunners.Get(tests.DefaultRunnerId)
	s.True(ok)
	s.Equal(tests.DefaultRunnerId, poolRunner.Id())
}

func (s *ManagerTestSuite) TestUpdateRunnersLogsErrorFromWatchAllocation() {
	var hook *test.Hook
	logger, hook := test.NewNullLogger()
	log = logger.WithField("pkg", "runner")
	s.modifyMockedCall("WatchAllocations", func(call *mock.Call) {
		call.Return(tests.DefaultError)
	})

	s.nomadRunnerManager.updateRunners(context.Background())

	require.Equal(s.T(), 1, len(hook.Entries))
	s.Equal(logrus.ErrorLevel, hook.LastEntry().Level)
	s.Equal(hook.LastEntry().Data[logrus.ErrorKey], tests.DefaultError)
}

func (s *ManagerTestSuite) TestUpdateRunnersAddsIdleRunner() {
	allocation := &nomadApi.Allocation{ID: tests.AllocationID}
	defaultJob, ok := s.nomadRunnerManager.jobs.Get(defaultEnvironmentId)
	require.True(s.T(), ok)
	allocation.JobID = string(defaultJob.jobId)

	_, ok = defaultJob.idleRunners.Get(allocation.ID)
	require.False(s.T(), ok)

	s.modifyMockedCall("WatchAllocations", func(call *mock.Call) {
		call.Run(func(args mock.Arguments) {
			onCreate, ok := args.Get(1).(nomad.AllocationProcessor)
			require.True(s.T(), ok)
			onCreate(allocation)
		})
	})

	s.nomadRunnerManager.updateRunners(context.Background())

	_, ok = defaultJob.idleRunners.Get(allocation.ID)
	s.True(ok)
}

func (s *ManagerTestSuite) TestUpdateRunnersRemovesIdleAndUsedRunner() {
	allocation := &nomadApi.Allocation{ID: tests.AllocationID}
	defaultJob, ok := s.nomadRunnerManager.jobs.Get(defaultEnvironmentId)
	require.True(s.T(), ok)
	allocation.JobID = string(defaultJob.jobId)

	testRunner := NewRunner(allocation.ID)
	defaultJob.idleRunners.Add(testRunner)
	s.nomadRunnerManager.usedRunners.Add(testRunner)

	s.modifyMockedCall("WatchAllocations", func(call *mock.Call) {
		call.Run(func(args mock.Arguments) {
			onDelete, ok := args.Get(2).(nomad.AllocationProcessor)
			require.True(s.T(), ok)
			onDelete(allocation)
		})
	})

	s.nomadRunnerManager.updateRunners(context.Background())

	_, ok = defaultJob.idleRunners.Get(allocation.ID)
	s.False(ok)
	_, ok = s.nomadRunnerManager.usedRunners.Get(allocation.ID)
	s.False(ok)
}

func (s *ManagerTestSuite) modifyMockedCall(method string, modifier func(call *mock.Call)) {
	for _, c := range s.apiMock.ExpectedCalls {
		if c.Method == method {
			modifier(c)
		}
	}
	s.True(ok)
	s.Equal(tests.DefaultRunnerId, poolRunner.Id())
}

func (s *ManagerTestSuite) TestWhenEnvironmentDoesNotExistEnvironmentExistsReturnsFalse() {
	id := anotherEnvironmentId
	_, ok := s.nomadRunnerManager.jobs.Get(id)
	require.False(s.T(), ok)

	s.False(s.nomadRunnerManager.EnvironmentExists(id))
}

func (s *ManagerTestSuite) TestWhenEnvironmentExistsEnvironmentExistsReturnsTrue() {
	id := anotherEnvironmentId
	s.nomadRunnerManager.jobs.Add(&NomadJob{environmentId: id})

	exists := s.nomadRunnerManager.EnvironmentExists(id)
	s.True(exists)
}
