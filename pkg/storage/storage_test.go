package storage

import (
	"github.com/stretchr/testify/suite"
	"testing"
)

func TestRunnerPoolTestSuite(t *testing.T) {
	suite.Run(t, new(ObjectPoolTestSuite))
}

type ObjectPoolTestSuite struct {
	suite.Suite
	objectStorage *localStorage[any]
	object        int
}

func (s *ObjectPoolTestSuite) SetupTest() {
	s.objectStorage = NewLocalStorage[any]()
	s.object = 42
}

func (s *ObjectPoolTestSuite) TestAddedObjectCanBeRetrieved() {
	s.objectStorage.Add("my_id", s.object)
	retrievedRunner, ok := s.objectStorage.Get("my_id")
	s.True(ok, "A saved object should be retrievable")
	s.Equal(s.object, retrievedRunner)
}

func (s *ObjectPoolTestSuite) TestObjectWithSameIdOverwritesOldOne() {
	otherObject := 21
	// assure object is actually different
	s.NotEqual(s.object, otherObject)

	s.objectStorage.Add("my_id", s.object)
	s.objectStorage.Add("my_id", otherObject)
	retrievedObject, _ := s.objectStorage.Get("my_id")
	s.NotEqual(s.object, retrievedObject)
	s.Equal(otherObject, retrievedObject)
}

func (s *ObjectPoolTestSuite) TestDeletedObjectsAreNotAccessible() {
	s.objectStorage.Add("my_id", s.object)
	s.objectStorage.Delete("my_id")
	retrievedObject, ok := s.objectStorage.Get("my_id")
	s.Nil(retrievedObject)
	s.False(ok, "A deleted object should not be accessible")
}

func (s *ObjectPoolTestSuite) TestSampleReturnsObjectWhenOneIsAvailable() {
	s.objectStorage.Add("my_id", s.object)
	sampledObject, ok := s.objectStorage.Sample()
	s.NotNil(sampledObject)
	s.True(ok)
}

func (s *ObjectPoolTestSuite) TestSampleReturnsFalseWhenNoneIsAvailable() {
	sampledObject, ok := s.objectStorage.Sample()
	s.Nil(sampledObject)
	s.False(ok)
}

func (s *ObjectPoolTestSuite) TestSampleRemovesObjectFromPool() {
	s.objectStorage.Add("my_id", s.object)
	_, _ = s.objectStorage.Sample()
	_, ok := s.objectStorage.Get("my_id")
	s.False(ok)
}

func (s *ObjectPoolTestSuite) TestLenOfEmptyPoolIsZero() {
	s.Equal(uint(0), s.objectStorage.Length())
}

func (s *ObjectPoolTestSuite) TestLenChangesOnStoreContentChange() {
	s.Run("len increases when object is added", func() {
		s.objectStorage.Add("my_id_1", s.object)
		s.Equal(uint(1), s.objectStorage.Length())
	})

	s.Run("len does not increase when object with same id is added", func() {
		s.objectStorage.Add("my_id_1", s.object)
		s.Equal(uint(1), s.objectStorage.Length())
	})

	s.Run("len increases again when different object is added", func() {
		anotherObject := 21
		s.objectStorage.Add("my_id_2", anotherObject)
		s.Equal(uint(2), s.objectStorage.Length())
	})

	s.Run("len decreases when object is deleted", func() {
		s.objectStorage.Delete("my_id_1")
		s.Equal(uint(1), s.objectStorage.Length())
	})

	s.Run("len decreases when object is sampled", func() {
		_, _ = s.objectStorage.Sample()
		s.Equal(uint(0), s.objectStorage.Length())
	})
}
