package runner

import (
	"github.com/stretchr/testify/suite"
	"testing"
)

func TestEnvironmentStoreTestSuite(t *testing.T) {
	suite.Run(t, new(EnvironmentStoreTestSuite))
}

type EnvironmentStoreTestSuite struct {
	suite.Suite
	environmentStorage *localEnvironmentStorage
	environment        *ExecutionEnvironmentMock
}

func (s *EnvironmentStoreTestSuite) SetupTest() {
	s.environmentStorage = NewLocalEnvironmentStorage()
	environmentMock := &ExecutionEnvironmentMock{}
	environmentMock.On("ID").Return(defaultEnvironmentID)
	s.environment = environmentMock
}

func (s *EnvironmentStoreTestSuite) TestAddedEnvironmentCanBeRetrieved() {
	s.environmentStorage.Add(s.environment)
	retrievedEnvironment, ok := s.environmentStorage.Get(s.environment.ID())
	s.True(ok, "A saved runner should be retrievable")
	s.Equal(s.environment, retrievedEnvironment)
}

func (s *EnvironmentStoreTestSuite) TestEnvironmentWithSameIdOverwritesOldOne() {
	otherEnvironmentWithSameID := &ExecutionEnvironmentMock{}
	otherEnvironmentWithSameID.On("ID").Return(defaultEnvironmentID)
	s.NotEqual(s.environment, otherEnvironmentWithSameID)

	s.environmentStorage.Add(s.environment)
	s.environmentStorage.Add(otherEnvironmentWithSameID)
	retrievedEnvironment, _ := s.environmentStorage.Get(s.environment.ID())
	s.NotEqual(s.environment, retrievedEnvironment)
	s.Equal(otherEnvironmentWithSameID, retrievedEnvironment)
}

func (s *EnvironmentStoreTestSuite) TestDeletedEnvironmentIsNotAccessible() {
	s.environmentStorage.Add(s.environment)
	s.environmentStorage.Delete(s.environment.ID())
	retrievedRunner, ok := s.environmentStorage.Get(s.environment.ID())
	s.Nil(retrievedRunner)
	s.False(ok, "A deleted runner should not be accessible")
}

func (s *EnvironmentStoreTestSuite) TestLenOfEmptyPoolIsZero() {
	s.Equal(0, s.environmentStorage.Length())
}

func (s *EnvironmentStoreTestSuite) TestLenChangesOnStoreContentChange() {
	s.Run("len increases when environment is added", func() {
		s.environmentStorage.Add(s.environment)
		s.Equal(1, s.environmentStorage.Length())
	})

	s.Run("len does not increase when environment with same id is added", func() {
		s.environmentStorage.Add(s.environment)
		s.Equal(1, s.environmentStorage.Length())
	})

	s.Run("len increases again when different environment is added", func() {
		anotherEnvironment := &ExecutionEnvironmentMock{}
		anotherEnvironment.On("ID").Return(anotherEnvironmentID)
		s.environmentStorage.Add(anotherEnvironment)
		s.Equal(2, s.environmentStorage.Length())
	})

	s.Run("len decreases when environment is deleted", func() {
		s.environmentStorage.Delete(s.environment.ID())
		s.Equal(1, s.environmentStorage.Length())
	})
}
