package runner

import (
	"gitlab.hpi.de/codeocean/codemoon/poseidon/store"
	"sync"
)

// Pool is a type of entity store that should store runner entities.
type Pool interface {
	store.EntityStore

	// Sample returns and removes an arbitrary entity from the pool.
	// ok is true iff a runner was returned.
	Sample() (r Runner, ok bool)
}

// localRunnerPool stores runner objects in the local application memory.
// ToDo: Create implementation that use some persistent storage like a database
type localRunnerPool struct {
	sync.RWMutex
	runners map[string]Runner
}

// NewLocalRunnerPool responds with a Pool implementation.
// This implementation stores the data thread-safe in the local application memory
func NewLocalRunnerPool() *localRunnerPool {
	return &localRunnerPool{
		runners: make(map[string]Runner),
	}
}

func (pool *localRunnerPool) Add(r store.Entity) {
	pool.Lock()
	defer pool.Unlock()
	runnerEntity, ok := r.(Runner)
	if !ok {
		log.
			WithField("pool", pool).
			WithField("entity", r).
			Fatal("Entity of type runner.Runner was expected, but wasn't given.")
	}
	pool.runners[r.Id()] = runnerEntity
}

func (pool *localRunnerPool) Get(id string) (r store.Entity, ok bool) {
	pool.RLock()
	defer pool.RUnlock()
	r, ok = pool.runners[id]
	return
}

func (pool *localRunnerPool) Delete(id string) {
	pool.Lock()
	defer pool.Unlock()
	delete(pool.runners, id)
}

func (pool *localRunnerPool) Sample() (Runner, bool) {
	pool.Lock()
	defer pool.Unlock()
	for _, runner := range pool.runners {
		delete(pool.runners, runner.Id())
		return runner, true
	}
	return nil, false
}

func (pool *localRunnerPool) Len() int {
	return len(pool.runners)
}
