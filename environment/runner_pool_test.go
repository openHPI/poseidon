package environment

import (
	"github.com/stretchr/testify/suite"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/runner"
	"testing"
)

type DummyEntity struct{}

func (DummyEntity) Id() string {
	return ""
}

func TestRunnerPoolTestSuite(t *testing.T) {
	suite.Run(t, new(RunnerPoolTestSuite))
}

type RunnerPoolTestSuite struct {
	suite.Suite
	runnerPool *localRunnerPool
	runner     runner.Runner
}

func (suite *RunnerPoolTestSuite) SetupTest() {
	suite.runnerPool = NewLocalRunnerPool()
	suite.runner = CreateTestRunner()
}

func (suite *RunnerPoolTestSuite) TestAddInvalidEntityTypeReturnsError() {
	dummyEntity := DummyEntity{}
	err := suite.runnerPool.Add(dummyEntity)
	suite.Error(err)
}

func (suite *RunnerPoolTestSuite) TestAddValidEntityReturnsNoError() {
	err := suite.runnerPool.Add(suite.runner)
	suite.NoError(err)
}

func (suite *RunnerPoolTestSuite) TestAddedRunnerCanBeRetrieved() {
	_ = suite.runnerPool.Add(suite.runner)
	retrievedRunner, ok := suite.runnerPool.Get(suite.runner.Id())
	suite.True(ok, "A saved runner should be retrievable")
	suite.Equal(suite.runner, retrievedRunner)
}

func (suite *RunnerPoolTestSuite) TestRunnerWithSameIdOverwritesOldOne() {
	otherRunnerWithSameId := runner.NewExerciseRunner(suite.runner.Id())
	// assure runner is actually different
	suite.NotEqual(suite.runner, otherRunnerWithSameId)

	_ = suite.runnerPool.Add(suite.runner)
	_ = suite.runnerPool.Add(otherRunnerWithSameId)
	retrievedRunner, _ := suite.runnerPool.Get(suite.runner.Id())
	suite.NotEqual(suite.runner, retrievedRunner)
	suite.Equal(otherRunnerWithSameId, retrievedRunner)
}

func (suite *RunnerPoolTestSuite) TestDeletedRunnersAreNotAccessible() {
	_ = suite.runnerPool.Add(suite.runner)
	suite.runnerPool.Delete(suite.runner.Id())
	retrievedRunner, ok := suite.runnerPool.Get(suite.runner.Id())
	suite.Nil(retrievedRunner)
	suite.False(ok, "A deleted runner should not be accessible")
}
