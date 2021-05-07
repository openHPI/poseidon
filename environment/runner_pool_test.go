package environment

import (
	"github.com/sirupsen/logrus"
	"github.com/sirupsen/logrus/hooks/test"
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

func (suite *RunnerPoolTestSuite) TestAddValidEntityThrowsFatal() {
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
	otherRunnerWithSameId := runner.NewExerciseRunner(suite.runner.Id())
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
