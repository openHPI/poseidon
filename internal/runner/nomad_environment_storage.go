package runner

import (
	"github.com/openHPI/poseidon/pkg/dto"
	"sync"
)

// EnvironmentStorage is an interface for storing environments.
type EnvironmentStorage interface {
	// List returns all environments stored in this storage.
	List() []ExecutionEnvironment

	// Add adds an environment to the storage.
	// It overwrites the old environment if one with the same id was already stored.
	Add(environment ExecutionEnvironment)

	// Get returns an environment from the storage.
	// Iff the environment does not exist in the store, ok will be false.
	Get(id dto.EnvironmentID) (environment ExecutionEnvironment, ok bool)

	// Delete deletes the environment with the passed id from the storage. It does nothing if no environment with the id
	// is present in the storage.
	Delete(id dto.EnvironmentID)

	// Length returns the number of currently stored environments in the storage.
	Length() int
}

// localEnvironmentStorage stores ExecutionEnvironment objects in the local application memory.
type localEnvironmentStorage struct {
	sync.RWMutex
	environments map[dto.EnvironmentID]ExecutionEnvironment
}

// NewLocalEnvironmentStorage responds with an empty localEnvironmentStorage.
// This implementation stores the data thread-safe in the local application memory.
func NewLocalEnvironmentStorage() *localEnvironmentStorage {
	return &localEnvironmentStorage{
		environments: make(map[dto.EnvironmentID]ExecutionEnvironment),
	}
}

func (s *localEnvironmentStorage) List() []ExecutionEnvironment {
	s.RLock()
	defer s.RUnlock()
	values := make([]ExecutionEnvironment, 0, len(s.environments))
	for _, v := range s.environments {
		values = append(values, v)
	}
	return values
}

func (s *localEnvironmentStorage) Add(environment ExecutionEnvironment) {
	s.Lock()
	defer s.Unlock()
	s.environments[environment.ID()] = environment
}

func (s *localEnvironmentStorage) Get(id dto.EnvironmentID) (environment ExecutionEnvironment, ok bool) {
	s.RLock()
	defer s.RUnlock()
	environment, ok = s.environments[id]
	return
}

func (s *localEnvironmentStorage) Delete(id dto.EnvironmentID) {
	s.Lock()
	defer s.Unlock()
	delete(s.environments, id)
}

func (s *localEnvironmentStorage) Length() int {
	s.RLock()
	defer s.RUnlock()
	return len(s.environments)
}
