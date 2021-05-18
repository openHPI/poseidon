package runner

import (
	"gitlab.hpi.de/codeocean/codemoon/poseidon/store"
	"sync"
)

// JobStore is a type of entity store that should store job entities.
type JobStore interface {
	store.EntityStore
}

// nomadJobStore stores NomadJob objects in the local application memory.
type nomadJobStore struct {
	sync.RWMutex
	jobs map[string]*NomadJob
}

// NewNomadJobStore responds with a Pool implementation.
// This implementation stores the data thread-safe in the local application memory.
func NewNomadJobStore() *nomadJobStore {
	return &nomadJobStore{
		jobs: make(map[string]*NomadJob),
	}
}

func (pool *nomadJobStore) Add(nomadJob store.Entity) {
	pool.Lock()
	defer pool.Unlock()
	jobEntity, ok := nomadJob.(*NomadJob)
	if !ok {
		log.
			WithField("pool", pool).
			WithField("entity", nomadJob).
			Fatal("Entity of type NomadJob was expected, but wasn't given.")
	}
	pool.jobs[nomadJob.Id()] = jobEntity
}

func (pool *nomadJobStore) Get(id string) (nomadJob store.Entity, ok bool) {
	pool.RLock()
	defer pool.RUnlock()
	nomadJob, ok = pool.jobs[id]
	return
}

func (pool *nomadJobStore) Delete(id string) {
	pool.Lock()
	defer pool.Unlock()
	delete(pool.jobs, id)
}

func (pool *nomadJobStore) Len() int {
	pool.RLock()
	defer pool.RUnlock()
	return len(pool.jobs)
}
