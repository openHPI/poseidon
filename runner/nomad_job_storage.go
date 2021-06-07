package runner

import (
	"sync"
)

// NomadJobStorage is an interface for storing NomadJobs.
type NomadJobStorage interface {
	// Add adds a job to the storage.
	// It overwrites the old job if one with the same id was already stored.
	Add(job *NomadJob)

	// Get returns a job from the storage.
	// Iff the job does not exist in the store, ok will be false.
	Get(id EnvironmentID) (job *NomadJob, ok bool)

	// Delete deletes the job with the passed id from the storage. It does nothing if no job with the id is present in
	// the storage.
	Delete(id EnvironmentID)

	// Length returns the number of currently stored jobs in the storage.
	Length() int
}

// localNomadJobStorage stores NomadJob objects in the local application memory.
type localNomadJobStorage struct {
	sync.RWMutex
	jobs map[EnvironmentID]*NomadJob
}

// NewLocalNomadJobStorage responds with an empty localNomadJobStorage.
// This implementation stores the data thread-safe in the local application memory.
func NewLocalNomadJobStorage() *localNomadJobStorage {
	return &localNomadJobStorage{
		jobs: make(map[EnvironmentID]*NomadJob),
	}
}

func (s *localNomadJobStorage) Add(job *NomadJob) {
	s.Lock()
	defer s.Unlock()
	s.jobs[job.ID()] = job
}

func (s *localNomadJobStorage) Get(id EnvironmentID) (job *NomadJob, ok bool) {
	s.RLock()
	defer s.RUnlock()
	job, ok = s.jobs[id]
	return
}

func (s *localNomadJobStorage) Delete(id EnvironmentID) {
	s.Lock()
	defer s.Unlock()
	delete(s.jobs, id)
}

func (s *localNomadJobStorage) Length() int {
	s.RLock()
	defer s.RUnlock()
	return len(s.jobs)
}
