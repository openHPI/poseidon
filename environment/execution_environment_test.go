package environment

import (
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/mocks"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/runner"
	"testing"
	"time"
)

const anotherRunnerId = "4n0th3r-1d"
const jobId = "4n0th3r-1d"

func TestGetNextRunnerTestSuite(t *testing.T) {
	suite.Run(t, new(GetNextRunnerTestSuite))
}

type GetNextRunnerTestSuite struct {
	suite.Suite
	nomadExecutionEnvironment *NomadExecutionEnvironment
	exerciseRunner            runner.Runner
}

func (suite *GetNextRunnerTestSuite) SetupTest() {
	suite.nomadExecutionEnvironment = &NomadExecutionEnvironment{
		availableRunners: make(chan runner.Runner, 50),
		allRunners:       NewLocalRunnerPool(),
	}
	suite.exerciseRunner = CreateTestRunner()
}

func (suite *GetNextRunnerTestSuite) TestGetNextRunnerReturnsRunnerIfAvailable() {
	suite.nomadExecutionEnvironment.availableRunners <- suite.exerciseRunner

	receivedRunner, err := suite.nomadExecutionEnvironment.NextRunner()
	suite.NoError(err)
	suite.Equal(suite.exerciseRunner, receivedRunner)
}

func (suite *GetNextRunnerTestSuite) TestGetNextRunnerChangesStatusOfRunner() {
	suite.nomadExecutionEnvironment.availableRunners <- suite.exerciseRunner

	receivedRunner, _ := suite.nomadExecutionEnvironment.NextRunner()
	suite.Equal(runner.StatusRunning, receivedRunner.Status())
}

func (suite *GetNextRunnerTestSuite) TestGetNextRunnerDoesNotReturnTheSameRunnerTwice() {
	suite.nomadExecutionEnvironment.availableRunners <- suite.exerciseRunner
	suite.nomadExecutionEnvironment.availableRunners <- runner.NewExerciseRunner(anotherRunnerId)

	firstReceivedRunner, _ := suite.nomadExecutionEnvironment.NextRunner()
	secondReceivedRunner, _ := suite.nomadExecutionEnvironment.NextRunner()
	suite.NotEqual(firstReceivedRunner, secondReceivedRunner)
}

func (suite *GetNextRunnerTestSuite) TestGetNextRunnerThrowsAnErrorIfNoRunnersAvailable() {
	receivedRunner, err := suite.nomadExecutionEnvironment.NextRunner()
	suite.Nil(receivedRunner)
	suite.Error(err)
}

func TestRefreshFetchRunners(t *testing.T) {
	apiMock, environment := newRefreshMock([]string{RunnerId}, NewLocalRunnerPool())
	// ToDo: Terminate Refresh when test finished (also in other tests)
	go environment.Refresh()
	_, _ = environment.NextRunner()
	apiMock.AssertCalled(t, "LoadAvailableRunners", jobId)
}

func TestRefreshFetchesRunnersIntoChannel(t *testing.T) {
	_, environment := newRefreshMock([]string{RunnerId}, NewLocalRunnerPool())
	go environment.Refresh()
	availableRunner, _ := environment.NextRunner()
	assert.Equal(t, availableRunner.Id(), RunnerId)
}

func TestRefreshScalesJob(t *testing.T) {
	apiMock, environment := newRefreshMock([]string{RunnerId}, NewLocalRunnerPool())
	go environment.Refresh()
	_, _ = environment.NextRunner()
	time.Sleep(100 * time.Millisecond) // ToDo: Be safe this test is not flaky
	apiMock.AssertCalled(t, "SetJobScaling", jobId, 52, "Runner Requested")
}

func TestRefreshAddsRunnerToPool(t *testing.T) {
	runnersInUse := NewLocalRunnerPool()
	_, environment := newRefreshMock([]string{RunnerId}, runnersInUse)
	go environment.Refresh()
	availableRunner, _ := environment.NextRunner()
	poolRunner, ok := runnersInUse.Get(availableRunner.Id())
	assert.True(t, ok)
	assert.Equal(t, availableRunner, poolRunner)
}

func newRefreshMock(returnedRunnerIds []string, allRunners RunnerPool) (apiClient *mocks.ExecutorApi, environment *NomadExecutionEnvironment) {
	apiClient = &mocks.ExecutorApi{}
	apiClient.On("LoadAvailableRunners", jobId).Return(returnedRunnerIds, nil)
	apiClient.On("GetJobScale", jobId).Return(len(returnedRunnerIds), nil)
	apiClient.On("SetJobScaling", jobId, mock.AnythingOfType("int"), "Runner Requested").Return(nil)
	environment = &NomadExecutionEnvironment{
		jobId:            jobId,
		availableRunners: make(chan runner.Runner, 50),
		allRunners:       allRunners,
		nomadApiClient:   apiClient,
	}
	return
}
