package storage

import (
	"github.com/influxdata/influxdb-client-go/v2/api/write"
	"github.com/openHPI/poseidon/tests"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"testing"
	"time"
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

func (s *ObjectPoolTestSuite) TestLocalStorage_List() {
	s.objectStorage.Add("my_id", s.object)
	s.objectStorage.Add("my_id 2", 21)
	retrievedRunners := s.objectStorage.List()
	s.Len(retrievedRunners, 2)
	s.Contains(retrievedRunners, s.object)
	s.Contains(retrievedRunners, 21)
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

func TestNewMonitoredLocalStorage_Callback(t *testing.T) {
	callbackCalls := 0
	callbackAdditions := 0
	callbackDeletions := 0
	os := NewMonitoredLocalStorage[string]("testMeasurement", func(p *write.Point, o string, eventType EventType) {
		callbackCalls++
		if eventType == Deletion {
			callbackDeletions++
		} else if eventType == Creation {
			callbackAdditions++
		}
	}, 0)

	assertCallbackCounts := func(test func(), totalCalls, additions, deletions int) {
		beforeTotal := callbackCalls
		beforeAdditions := callbackAdditions
		beforeDeletions := callbackDeletions
		test()
		assert.Equal(t, beforeTotal+totalCalls, callbackCalls)
		assert.Equal(t, beforeAdditions+additions, callbackAdditions)
		assert.Equal(t, beforeDeletions+deletions, callbackDeletions)
	}

	t.Run("Add", func(t *testing.T) {
		assertCallbackCounts(func() {
			os.Add("id 1", "object 1")
		}, 1, 1, 0)
	})

	t.Run("Delete", func(t *testing.T) {
		assertCallbackCounts(func() {
			os.Delete("id 1")
		}, 1, 0, 1)
	})

	t.Run("List", func(t *testing.T) {
		assertCallbackCounts(func() {
			os.List()
		}, 0, 0, 0)
	})

	t.Run("Pop", func(t *testing.T) {
		os.Add("id 1", "object 1")

		assertCallbackCounts(func() {
			o, ok := os.Pop("id 1")
			assert.True(t, ok)
			assert.Equal(t, "object 1", o)
		}, 1, 0, 1)
	})

	t.Run("Purge", func(t *testing.T) {
		os.Add("id 1", "object 1")
		os.Add("id 2", "object 2")

		assertCallbackCounts(func() {
			os.Purge()
		}, 2, 0, 2)
	})
}

func TestNewMonitoredLocalStorage_Periodically(t *testing.T) {
	callbackCalls := 0
	NewMonitoredLocalStorage[string]("testMeasurement", func(p *write.Point, o string, eventType EventType) {
		callbackCalls++
		assert.Equal(t, Periodically, eventType)
	}, 200*time.Millisecond)

	time.Sleep(tests.ShortTimeout)
	assert.Equal(t, 1, callbackCalls)
	time.Sleep(200 * time.Millisecond)
	assert.Equal(t, 2, callbackCalls)
}
