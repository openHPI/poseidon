package environment

import (
	"gitlab.hpi.de/codeocean/codemoon/poseidon/runner"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/store"
	"sync"
)

// RunnerPool is a type of entity store that should store runner entities.
type RunnerPool interface {
	store.EntityStore
}

// localRunnerPool stores runner objects in the local application memory.
// ToDo: Create implementation that use some persistent storage like a database
type localRunnerPool struct {
	sync.RWMutex
	runners map[string]runner.Runner
}

// NewLocalRunnerPool responds with a RunnerPool implementation
// This implementation stores the data thread-safe in the local application memory
func NewLocalRunnerPool() *localRunnerPool {
	return &localRunnerPool{
		runners: make(map[string]runner.Runner),
	}
}

func (pool *localRunnerPool) Add(r store.Entity) {
	pool.Lock()
	defer pool.Unlock()
	runnerEntity, ok := r.(runner.Runner)
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
