package runner

import (
	"sync"
)

// NomadEnvironmentStorage is an interface for storing Nomad environments.
type NomadEnvironmentStorage interface {
	// List returns all keys of environments stored in this storage.
	List() []EnvironmentID

	// Add adds an environment to the storage.
	// It overwrites the old environment if one with the same id was already stored.
	Add(environment *NomadEnvironment)

	// Get returns an environment from the storage.
	// Iff the environment does not exist in the store, ok will be false.
	Get(id EnvironmentID) (environment *NomadEnvironment, ok bool)

	// Delete deletes the environment with the passed id from the storage. It does nothing if no environment with the id
	// is present in the storage.
	Delete(id EnvironmentID)

	// Length returns the number of currently stored environments in the storage.
	Length() int
}

// localNomadEnvironmentStorage stores NomadEnvironment objects in the local application memory.
type localNomadEnvironmentStorage struct {
	sync.RWMutex
	environments map[EnvironmentID]*NomadEnvironment
}

// NewLocalNomadEnvironmentStorage responds with an empty localNomadEnvironmentStorage.
// This implementation stores the data thread-safe in the local application memory.
func NewLocalNomadEnvironmentStorage() *localNomadEnvironmentStorage {
	return &localNomadEnvironmentStorage{
		environments: make(map[EnvironmentID]*NomadEnvironment),
	}
}

func (s *localNomadEnvironmentStorage) List() []EnvironmentID {
	keys := make([]EnvironmentID, 0, len(s.environments))
	for k := range s.environments {
		keys = append(keys, k)
	}
	return keys
}

func (s *localNomadEnvironmentStorage) Add(environment *NomadEnvironment) {
	s.Lock()
	defer s.Unlock()
	s.environments[environment.ID()] = environment
}

func (s *localNomadEnvironmentStorage) Get(id EnvironmentID) (environment *NomadEnvironment, ok bool) {
	s.RLock()
	defer s.RUnlock()
	environment, ok = s.environments[id]
	return
}

func (s *localNomadEnvironmentStorage) Delete(id EnvironmentID) {
	s.Lock()
	defer s.Unlock()
	delete(s.environments, id)
}

func (s *localNomadEnvironmentStorage) Length() int {
	s.RLock()
	defer s.RUnlock()
	return len(s.environments)
}
