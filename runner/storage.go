package runner

import (
	"sync"
)

// Storage is an interface for storing runners.
type Storage interface {
	// Add adds an runner to the storage.
	// It overwrites the old runner if one with the same id was already stored.
	Add(Runner)

	// Get returns a runner from the storage.
	// Iff the runner does not exist in the storage, ok will be false.
	Get(id string) (r Runner, ok bool)

	// Delete deletes the runner with the passed id from the storage. It does nothing if no runner with the id is present in the store.
	Delete(id string)

	// Length returns the number of currently stored runners in the storage.
	Length() int

	// Sample returns and removes an arbitrary runner from the storage.
	// ok is true iff a runner was returned.
	Sample() (r Runner, ok bool)
}

// localRunnerStorage stores runner objects in the local application memory.
// ToDo: Create implementation that use some persistent storage like a database
type localRunnerStorage struct {
	sync.RWMutex
	runners map[string]Runner
}

// NewLocalRunnerStorage responds with a Storage implementation.
// This implementation stores the data thread-safe in the local application memory
func NewLocalRunnerStorage() *localRunnerStorage {
	return &localRunnerStorage{
		runners: make(map[string]Runner),
	}
}

func (s *localRunnerStorage) Add(r Runner) {
	s.Lock()
	defer s.Unlock()
	s.runners[r.Id()] = r
}

func (s *localRunnerStorage) Get(id string) (r Runner, ok bool) {
	s.RLock()
	defer s.RUnlock()
	r, ok = s.runners[id]
	return
}

func (s *localRunnerStorage) Delete(id string) {
	s.Lock()
	defer s.Unlock()
	delete(s.runners, id)
}

func (s *localRunnerStorage) Sample() (Runner, bool) {
	s.Lock()
	defer s.Unlock()
	for _, runner := range s.runners {
		delete(s.runners, runner.Id())
		return runner, true
	}
	return nil, false
}

func (s *localRunnerStorage) Length() int {
	s.RLock()
	defer s.RUnlock()
	return len(s.runners)
}
