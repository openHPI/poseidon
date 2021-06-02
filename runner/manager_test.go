package runner

import (
	"errors"
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
	s.nomadRunnerManager = NewNomadRunnerManager(s.apiMock)
	s.exerciseRunner = NewRunner(tests.DefaultRunnerId)
	s.mockRunnerQueries([]string{})
	s.registerDefaultEnvironment()
}

func (s *ManagerTestSuite) mockRunnerQueries(returnedRunnerIds []string) {
	// reset expected calls to allow new mocked return values
	s.apiMock.ExpectedCalls = []*mock.Call{}
	s.apiMock.On("LoadRunners", tests.DefaultJobId).Return(returnedRunnerIds, nil)
	s.apiMock.On("JobScale", tests.DefaultJobId).Return(uint(len(returnedRunnerIds)), nil)
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

	firstReceivedRunner, _ := s.nomadRunnerManager.Claim(defaultEnvironmentId)
	secondReceivedRunner, _ := s.nomadRunnerManager.Claim(defaultEnvironmentId)
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
	receivedRunner, _ := s.nomadRunnerManager.Claim(defaultEnvironmentId)
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
