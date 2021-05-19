package runner

import (
	"github.com/sirupsen/logrus"
	"github.com/sirupsen/logrus/hooks/test"
	"github.com/stretchr/testify/suite"
	"testing"
)

func TestRunnerPoolTestSuite(t *testing.T) {
	suite.Run(t, new(RunnerPoolTestSuite))
}

type RunnerPoolTestSuite struct {
	suite.Suite
	runnerPool *localRunnerPool
	runner     Runner
}

func (suite *RunnerPoolTestSuite) SetupTest() {
	suite.runnerPool = NewLocalRunnerPool()
	suite.runner = NewRunner(defaultRunnerId)
}

func (suite *RunnerPoolTestSuite) TestAddInvalidEntityTypeThrowsFatal() {
	var hook *test.Hook
	logger, hook := test.NewNullLogger()
	// don't terminate program on fatal log entry
	logger.ExitFunc = func(int) {}
	log = logger.WithField("pkg", "environment")

	dummyEntity := DummyEntity{}
	suite.runnerPool.Add(dummyEntity)
	suite.Equal(logrus.FatalLevel, hook.LastEntry().Level)
	suite.Equal(dummyEntity, hook.LastEntry().Data["entity"])
}

func (suite *RunnerPoolTestSuite) TestAddValidEntityDoesNotThrowFatal() {
	var hook *test.Hook
	logger, hook := test.NewNullLogger()
	log = logger.WithField("pkg", "environment")

	suite.runnerPool.Add(suite.runner)
	// currently, the Add method does not log anything else. adjust if necessary
	suite.Nil(hook.LastEntry())
}

func (suite *RunnerPoolTestSuite) TestAddedRunnerCanBeRetrieved() {
	suite.runnerPool.Add(suite.runner)
	retrievedRunner, ok := suite.runnerPool.Get(suite.runner.Id())
	suite.True(ok, "A saved runner should be retrievable")
	suite.Equal(suite.runner, retrievedRunner)
}

func (suite *RunnerPoolTestSuite) TestRunnerWithSameIdOverwritesOldOne() {
	otherRunnerWithSameId := NewRunner(suite.runner.Id())
	// assure runner is actually different
	suite.NotEqual(suite.runner, otherRunnerWithSameId)

	suite.runnerPool.Add(suite.runner)
	suite.runnerPool.Add(otherRunnerWithSameId)
	retrievedRunner, _ := suite.runnerPool.Get(suite.runner.Id())
	suite.NotEqual(suite.runner, retrievedRunner)
	suite.Equal(otherRunnerWithSameId, retrievedRunner)
}

func (suite *RunnerPoolTestSuite) TestDeletedRunnersAreNotAccessible() {
	suite.runnerPool.Add(suite.runner)
	suite.runnerPool.Delete(suite.runner.Id())
	retrievedRunner, ok := suite.runnerPool.Get(suite.runner.Id())
	suite.Nil(retrievedRunner)
	suite.False(ok, "A deleted runner should not be accessible")
}

func (suite *RunnerPoolTestSuite) TestSampleReturnsRunnerWhenOneIsAvailable() {
	suite.runnerPool.Add(suite.runner)
	sampledRunner, ok := suite.runnerPool.Sample()
	suite.NotNil(sampledRunner)
	suite.True(ok)
}

func (suite *RunnerPoolTestSuite) TestSampleReturnsFalseWhenNoneIsAvailable() {
	sampledRunner, ok := suite.runnerPool.Sample()
	suite.Nil(sampledRunner)
	suite.False(ok)
}

func (suite *RunnerPoolTestSuite) TestSampleRemovesRunnerFromPool() {
	suite.runnerPool.Add(suite.runner)
	sampledRunner, _ := suite.runnerPool.Sample()
	_, ok := suite.runnerPool.Get(sampledRunner.Id())
	suite.False(ok)
}

func (suite *RunnerPoolTestSuite) TestLenOfEmptyPoolIsZero() {
	suite.Equal(0, suite.runnerPool.Len())
}

func (suite *RunnerPoolTestSuite) TestLenChangesOnStoreContentChange() {
	suite.Run("len increases when runner is added", func() {
		suite.runnerPool.Add(suite.runner)
		suite.Equal(1, suite.runnerPool.Len())
	})

	suite.Run("len does not increase when runner with same id is added", func() {
		suite.runnerPool.Add(suite.runner)
		suite.Equal(1, suite.runnerPool.Len())
	})

	suite.Run("len increases again when different runner is added", func() {
		anotherRunner := NewRunner(anotherRunnerId)
		suite.runnerPool.Add(anotherRunner)
		suite.Equal(2, suite.runnerPool.Len())
	})

	suite.Run("len decreases when runner is deleted", func() {
		suite.runnerPool.Delete(suite.runner.Id())
		suite.Equal(1, suite.runnerPool.Len())
	})

	suite.Run("len decreases when runner is sampled", func() {
		_, _ = suite.runnerPool.Sample()
		suite.Equal(0, suite.runnerPool.Len())
	})
}
