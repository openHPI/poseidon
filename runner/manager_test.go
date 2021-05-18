package runner

import (
	"errors"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/nomad"
	"testing"
	"time"
)

const (
	anotherRunnerId            = "4n0th3r-runn3r-1d"
	defaultEnvironmentId       = EnvironmentId(0)
	otherEnvironmentId         = EnvironmentId(42)
	defaultDesiredRunnersCount = 5
	jobId                      = "4n0th3r-j0b-1d"
	waitTime                   = 100 * time.Millisecond
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

func (suite *ManagerTestSuite) SetupTest() {
	suite.apiMock = &nomad.ExecutorApiMock{}
	suite.nomadRunnerManager = NewNomadRunnerManager(suite.apiMock)
	suite.exerciseRunner = CreateTestRunner()
	suite.mockRunnerQueries([]string{})
	suite.registerDefaultEnvironment()
}

func (suite *ManagerTestSuite) mockRunnerQueries(returnedRunnerIds []string) {
	// reset expected calls to allow new mocked return values
	suite.apiMock.ExpectedCalls = []*mock.Call{}
	suite.apiMock.On("LoadRunners", jobId).Return(returnedRunnerIds, nil)
	suite.apiMock.On("JobScale", jobId).Return(len(returnedRunnerIds), nil)
	suite.apiMock.On("SetJobScale", jobId, mock.AnythingOfType("int"), "Runner Requested").Return(nil)
}

func (suite *ManagerTestSuite) registerDefaultEnvironment() {
	suite.nomadRunnerManager.RegisterEnvironment(defaultEnvironmentId, jobId, defaultDesiredRunnersCount)
}

func (suite *ManagerTestSuite) AddIdleRunnerForDefaultEnvironment(r Runner) {
	jobEntity, _ := suite.nomadRunnerManager.jobs.Get(defaultEnvironmentId.toString())
	jobEntity.(*NomadJob).idleRunners.Add(r)
}

func (suite *ManagerTestSuite) waitForRunnerRefresh() {
	time.Sleep(waitTime)
}

func (suite *ManagerTestSuite) TestRegisterEnvironmentAddsNewJob() {
	suite.nomadRunnerManager.RegisterEnvironment(otherEnvironmentId, jobId, defaultDesiredRunnersCount)
	jobEntity, ok := suite.nomadRunnerManager.jobs.Get(defaultEnvironmentId.toString())
	suite.True(ok)
	suite.NotNil(jobEntity)
}

func (suite *ManagerTestSuite) TestClaimReturnsNotFoundErrorIfEnvironmentNotFound() {
	runner, err := suite.nomadRunnerManager.Claim(EnvironmentId(42))
	suite.Nil(runner)
	suite.Equal(ErrUnknownExecutionEnvironment, err)
}

func (suite *ManagerTestSuite) TestClaimReturnsRunnerIfAvailable() {
	suite.AddIdleRunnerForDefaultEnvironment(suite.exerciseRunner)
	receivedRunner, err := suite.nomadRunnerManager.Claim(defaultEnvironmentId)
	suite.NoError(err)
	suite.Equal(suite.exerciseRunner, receivedRunner)
}

func (suite *ManagerTestSuite) TestClaimReturnsErrorIfNoRunnerAvailable() {
	suite.waitForRunnerRefresh()
	runner, err := suite.nomadRunnerManager.Claim(defaultEnvironmentId)
	suite.Nil(runner)
	suite.Equal(ErrNoRunnersAvailable, err)
}

func (suite *ManagerTestSuite) TestClaimReturnsNoRunnerOfDifferentEnvironment() {
	suite.AddIdleRunnerForDefaultEnvironment(suite.exerciseRunner)
	receivedRunner, err := suite.nomadRunnerManager.Claim(otherEnvironmentId)
	suite.Nil(receivedRunner)
	suite.Error(err)
}

func (suite *ManagerTestSuite) TestClaimDoesNotReturnTheSameRunnerTwice() {
	suite.AddIdleRunnerForDefaultEnvironment(suite.exerciseRunner)
	suite.AddIdleRunnerForDefaultEnvironment(NewRunner(anotherRunnerId))

	firstReceivedRunner, _ := suite.nomadRunnerManager.Claim(defaultEnvironmentId)
	secondReceivedRunner, _ := suite.nomadRunnerManager.Claim(defaultEnvironmentId)
	suite.NotEqual(firstReceivedRunner, secondReceivedRunner)
}

func (suite *ManagerTestSuite) TestClaimThrowsAnErrorIfNoRunnersAvailable() {
	receivedRunner, err := suite.nomadRunnerManager.Claim(defaultEnvironmentId)
	suite.Nil(receivedRunner)
	suite.Error(err)
}

func (suite *ManagerTestSuite) TestClaimAddsRunnerToUsedRunners() {
	suite.mockRunnerQueries([]string{RunnerId})
	suite.waitForRunnerRefresh()
	receivedRunner, _ := suite.nomadRunnerManager.Claim(defaultEnvironmentId)
	savedRunner, ok := suite.nomadRunnerManager.usedRunners.Get(receivedRunner.Id())
	suite.True(ok)
	suite.Equal(savedRunner, receivedRunner)
}

func (suite *ManagerTestSuite) TestGetReturnsRunnerIfRunnerIsUsed() {
	suite.nomadRunnerManager.usedRunners.Add(suite.exerciseRunner)
	savedRunner, err := suite.nomadRunnerManager.Get(suite.exerciseRunner.Id())
	suite.NoError(err)
	suite.Equal(savedRunner, suite.exerciseRunner)
}

func (suite *ManagerTestSuite) TestGetReturnsErrorIfRunnerNotFound() {
	savedRunner, err := suite.nomadRunnerManager.Get(RunnerId)
	suite.Nil(savedRunner)
	suite.Error(err)
}

func (suite *ManagerTestSuite) TestReturnRemovesRunnerFromUsedRunners() {
	suite.apiMock.On("DeleteRunner", mock.AnythingOfType("string")).Return(nil)
	suite.nomadRunnerManager.usedRunners.Add(suite.exerciseRunner)
	err := suite.nomadRunnerManager.Return(suite.exerciseRunner)
	suite.Nil(err)
	_, ok := suite.nomadRunnerManager.usedRunners.Get(suite.exerciseRunner.Id())
	suite.False(ok)
}

func (suite *ManagerTestSuite) TestReturnCallsDeleteRunnerApiMethod() {
	suite.apiMock.On("DeleteRunner", mock.AnythingOfType("string")).Return(nil)
	err := suite.nomadRunnerManager.Return(suite.exerciseRunner)
	suite.Nil(err)
	suite.apiMock.AssertCalled(suite.T(), "DeleteRunner", suite.exerciseRunner.Id())
}

func (suite *ManagerTestSuite) TestReturnReturnsErrorWhenApiCallFailed() {
	suite.apiMock.On("DeleteRunner", mock.AnythingOfType("string")).Return(errors.New("return failed"))
	err := suite.nomadRunnerManager.Return(suite.exerciseRunner)
	suite.Error(err)
}

func (suite *ManagerTestSuite) TestRefreshFetchesRunners() {
	suite.mockRunnerQueries([]string{RunnerId})
	suite.waitForRunnerRefresh()
	suite.apiMock.AssertCalled(suite.T(), "LoadRunners", jobId)
}

func (suite *ManagerTestSuite) TestNewRunnersFoundInRefreshAreAddedToUnusedRunners() {
	suite.mockRunnerQueries([]string{RunnerId})
	suite.waitForRunnerRefresh()
	availableRunner, _ := suite.nomadRunnerManager.Claim(defaultEnvironmentId)
	suite.Equal(availableRunner.Id(), RunnerId)
}

func (suite *ManagerTestSuite) TestRefreshScalesJob() {
	suite.mockRunnerQueries([]string{RunnerId})
	suite.waitForRunnerRefresh()
	// use one runner to necessitate rescaling
	_, _ = suite.nomadRunnerManager.Claim(defaultEnvironmentId)
	suite.waitForRunnerRefresh()
	suite.apiMock.AssertCalled(suite.T(), "SetJobScale", jobId, defaultDesiredRunnersCount+1, "Runner Requested")
}

func (suite *ManagerTestSuite) TestRefreshAddsRunnerToPool() {
	suite.mockRunnerQueries([]string{RunnerId})
	suite.waitForRunnerRefresh()
	jobEntity, _ := suite.nomadRunnerManager.jobs.Get(defaultEnvironmentId.toString())
	poolRunner, ok := jobEntity.(*NomadJob).idleRunners.Get(RunnerId)
	suite.True(ok)
	suite.Equal(RunnerId, poolRunner.Id())
}
