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
	defaultDesiredRunnersCount = 5
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
	suite.exerciseRunner = NewRunner(defaultRunnerId)
	suite.mockRunnerQueries([]string{})
	suite.registerDefaultEnvironment()
}

func (suite *ManagerTestSuite) mockRunnerQueries(returnedRunnerIds []string) {
	// reset expected calls to allow new mocked return values
	suite.apiMock.ExpectedCalls = []*mock.Call{}
	suite.apiMock.On("LoadRunners", defaultJobId).Return(returnedRunnerIds, nil)
	suite.apiMock.On("JobScale", defaultJobId).Return(len(returnedRunnerIds), nil)
	suite.apiMock.On("SetJobScale", defaultJobId, mock.AnythingOfType("int"), "Runner Requested").Return(nil)
}

func (suite *ManagerTestSuite) registerDefaultEnvironment() {
	suite.nomadRunnerManager.RegisterEnvironment(defaultEnvironmentId, defaultJobId, defaultDesiredRunnersCount)
}

func (suite *ManagerTestSuite) AddIdleRunnerForDefaultEnvironment(r Runner) {
	jobEntity, _ := suite.nomadRunnerManager.jobs.Get(defaultEnvironmentId.toString())
	jobEntity.(*NomadJob).idleRunners.Add(r)
}

func (suite *ManagerTestSuite) waitForRunnerRefresh() {
	time.Sleep(100 * time.Millisecond)
}

func (suite *ManagerTestSuite) TestRegisterEnvironmentAddsNewJob() {
	suite.nomadRunnerManager.RegisterEnvironment(anotherEnvironmentId, defaultJobId, defaultDesiredRunnersCount)
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
	receivedRunner, err := suite.nomadRunnerManager.Claim(anotherEnvironmentId)
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
	suite.mockRunnerQueries([]string{defaultRunnerId})
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
	savedRunner, err := suite.nomadRunnerManager.Get(defaultRunnerId)
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
	suite.mockRunnerQueries([]string{defaultRunnerId})
	suite.waitForRunnerRefresh()
	suite.apiMock.AssertCalled(suite.T(), "LoadRunners", defaultJobId)
}

func (suite *ManagerTestSuite) TestNewRunnersFoundInRefreshAreAddedToIdleRunners() {
	suite.mockRunnerQueries([]string{defaultRunnerId})
	suite.waitForRunnerRefresh()
	jobEntity, _ := suite.nomadRunnerManager.jobs.Get(defaultEnvironmentId.toString())
	_, ok := jobEntity.(*NomadJob).idleRunners.Get(defaultRunnerId)
	suite.True(ok)
}

func (suite *ManagerTestSuite) TestRefreshScalesJob() {
	suite.mockRunnerQueries([]string{defaultRunnerId})
	suite.waitForRunnerRefresh()
	// use one runner to necessitate rescaling
	_, _ = suite.nomadRunnerManager.Claim(defaultEnvironmentId)
	suite.waitForRunnerRefresh()
	suite.apiMock.AssertCalled(suite.T(), "SetJobScale", defaultJobId, defaultDesiredRunnersCount+1, "Runner Requested")
}

func (suite *ManagerTestSuite) TestRefreshAddsRunnerToPool() {
	suite.mockRunnerQueries([]string{defaultRunnerId})
	suite.waitForRunnerRefresh()
	jobEntity, _ := suite.nomadRunnerManager.jobs.Get(defaultEnvironmentId.toString())
	poolRunner, ok := jobEntity.(*NomadJob).idleRunners.Get(defaultRunnerId)
	suite.True(ok)
	suite.Equal(defaultRunnerId, poolRunner.Id())
}
