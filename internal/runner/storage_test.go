package runner

import (
	"github.com/stretchr/testify/suite"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/pkg/dto"
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

func (s *RunnerPoolTestSuite) SetupTest() {
	s.runnerStorage = NewLocalRunnerStorage()
	s.runner = NewRunner(tests.DefaultRunnerID, nil)
	s.runner.StoreExecution(tests.DefaultExecutionID, &dto.ExecutionRequest{Command: "true"})
}

func (s *RunnerPoolTestSuite) TestAddedRunnerCanBeRetrieved() {
	s.runnerStorage.Add(s.runner)
	retrievedRunner, ok := s.runnerStorage.Get(s.runner.ID())
	s.True(ok, "A saved runner should be retrievable")
	s.Equal(s.runner, retrievedRunner)
}

func (s *RunnerPoolTestSuite) TestRunnerWithSameIdOverwritesOldOne() {
	otherRunnerWithSameID := NewRunner(s.runner.ID(), nil)
	// assure runner is actually different
	s.NotEqual(s.runner, otherRunnerWithSameID)

	s.runnerStorage.Add(s.runner)
	s.runnerStorage.Add(otherRunnerWithSameID)
	retrievedRunner, _ := s.runnerStorage.Get(s.runner.ID())
	s.NotEqual(s.runner, retrievedRunner)
	s.Equal(otherRunnerWithSameID, retrievedRunner)
}

func (s *RunnerPoolTestSuite) TestDeletedRunnersAreNotAccessible() {
	s.runnerStorage.Add(s.runner)
	s.runnerStorage.Delete(s.runner.ID())
	retrievedRunner, ok := s.runnerStorage.Get(s.runner.ID())
	s.Nil(retrievedRunner)
	s.False(ok, "A deleted runner should not be accessible")
}

func (s *RunnerPoolTestSuite) TestSampleReturnsRunnerWhenOneIsAvailable() {
	s.runnerStorage.Add(s.runner)
	sampledRunner, ok := s.runnerStorage.Sample()
	s.NotNil(sampledRunner)
	s.True(ok)
}

func (s *RunnerPoolTestSuite) TestSampleReturnsFalseWhenNoneIsAvailable() {
	sampledRunner, ok := s.runnerStorage.Sample()
	s.Nil(sampledRunner)
	s.False(ok)
}

func (s *RunnerPoolTestSuite) TestSampleRemovesRunnerFromPool() {
	s.runnerStorage.Add(s.runner)
	sampledRunner, _ := s.runnerStorage.Sample()
	_, ok := s.runnerStorage.Get(sampledRunner.ID())
	s.False(ok)
}

func (s *RunnerPoolTestSuite) TestLenOfEmptyPoolIsZero() {
	s.Equal(0, s.runnerStorage.Length())
}

func (s *RunnerPoolTestSuite) TestLenChangesOnStoreContentChange() {
	s.Run("len increases when runner is added", func() {
		s.runnerStorage.Add(s.runner)
		s.Equal(1, s.runnerStorage.Length())
	})

	s.Run("len does not increase when runner with same id is added", func() {
		s.runnerStorage.Add(s.runner)
		s.Equal(1, s.runnerStorage.Length())
	})

	s.Run("len increases again when different runner is added", func() {
		anotherRunner := NewRunner(tests.AnotherRunnerID, nil)
		s.runnerStorage.Add(anotherRunner)
		s.Equal(2, s.runnerStorage.Length())
	})

	s.Run("len decreases when runner is deleted", func() {
		s.runnerStorage.Delete(s.runner.ID())
		s.Equal(1, s.runnerStorage.Length())
	})

	s.Run("len decreases when runner is sampled", func() {
		_, _ = s.runnerStorage.Sample()
		s.Equal(0, s.runnerStorage.Length())
	})
}
