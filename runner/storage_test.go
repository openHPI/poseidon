package runner

import (
	"github.com/stretchr/testify/suite"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/tests"
	"testing"
)

func TestRunnerPoolTestSuite(t *testing.T) {
	suite.Run(t, new(RunnerPoolTestSuite))
}

type RunnerPoolTestSuite struct {
	suite.Suite
	runnerStorage *localRunnerStorage
	runner        Runner
}

func (suite *RunnerPoolTestSuite) SetupTest() {
	suite.runnerStorage = NewLocalRunnerStorage()
	suite.runner = NewRunner(tests.DefaultRunnerId)
}

func (suite *RunnerPoolTestSuite) TestAddedRunnerCanBeRetrieved() {
	suite.runnerStorage.Add(suite.runner)
	retrievedRunner, ok := suite.runnerStorage.Get(suite.runner.Id())
	suite.True(ok, "A saved runner should be retrievable")
	suite.Equal(suite.runner, retrievedRunner)
}

func (suite *RunnerPoolTestSuite) TestRunnerWithSameIdOverwritesOldOne() {
	otherRunnerWithSameId := NewRunner(suite.runner.Id())
	// assure runner is actually different
	suite.NotEqual(suite.runner, otherRunnerWithSameId)

	suite.runnerStorage.Add(suite.runner)
	suite.runnerStorage.Add(otherRunnerWithSameId)
	retrievedRunner, _ := suite.runnerStorage.Get(suite.runner.Id())
	suite.NotEqual(suite.runner, retrievedRunner)
	suite.Equal(otherRunnerWithSameId, retrievedRunner)
}

func (suite *RunnerPoolTestSuite) TestDeletedRunnersAreNotAccessible() {
	suite.runnerStorage.Add(suite.runner)
	suite.runnerStorage.Delete(suite.runner.Id())
	retrievedRunner, ok := suite.runnerStorage.Get(suite.runner.Id())
	suite.Nil(retrievedRunner)
	suite.False(ok, "A deleted runner should not be accessible")
}

func (suite *RunnerPoolTestSuite) TestSampleReturnsRunnerWhenOneIsAvailable() {
	suite.runnerStorage.Add(suite.runner)
	sampledRunner, ok := suite.runnerStorage.Sample()
	suite.NotNil(sampledRunner)
	suite.True(ok)
}

func (suite *RunnerPoolTestSuite) TestSampleReturnsFalseWhenNoneIsAvailable() {
	sampledRunner, ok := suite.runnerStorage.Sample()
	suite.Nil(sampledRunner)
	suite.False(ok)
}

func (suite *RunnerPoolTestSuite) TestSampleRemovesRunnerFromPool() {
	suite.runnerStorage.Add(suite.runner)
	sampledRunner, _ := suite.runnerStorage.Sample()
	_, ok := suite.runnerStorage.Get(sampledRunner.Id())
	suite.False(ok)
}

func (suite *RunnerPoolTestSuite) TestLenOfEmptyPoolIsZero() {
	suite.Equal(0, suite.runnerStorage.Length())
}

func (suite *RunnerPoolTestSuite) TestLenChangesOnStoreContentChange() {
	suite.Run("len increases when runner is added", func() {
		suite.runnerStorage.Add(suite.runner)
		suite.Equal(1, suite.runnerStorage.Length())
	})

	suite.Run("len does not increase when runner with same id is added", func() {
		suite.runnerStorage.Add(suite.runner)
		suite.Equal(1, suite.runnerStorage.Length())
	})

	suite.Run("len increases again when different runner is added", func() {
		anotherRunner := NewRunner(tests.AnotherRunnerId)
		suite.runnerStorage.Add(anotherRunner)
		suite.Equal(2, suite.runnerStorage.Length())
	})

	suite.Run("len decreases when runner is deleted", func() {
		suite.runnerStorage.Delete(suite.runner.Id())
		suite.Equal(1, suite.runnerStorage.Length())
	})

	suite.Run("len decreases when runner is sampled", func() {
		_, _ = suite.runnerStorage.Sample()
		suite.Equal(0, suite.runnerStorage.Length())
	})
}
