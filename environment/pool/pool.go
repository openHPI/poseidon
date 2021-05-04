package pool

import (
	"gitlab.hpi.de/codeocean/codemoon/poseidon/runner"
	"sync"
)

// RunnerPool is the interface for storing runners
// ToDo: Create interface implementation that use some persistent storage like a database
type RunnerPool interface {
	// AddRunner adds a provided runner to the pool storage
	AddRunner(runner.Runner)
	// GetRunner returns a runner from the pool storage
	// If the requested runner is not stored 'ok' will be false
	GetRunner(id string) (r runner.Runner, ok bool)
	// DeleteRunner deletes the runner with the passed id from the pool storage
	DeleteRunner(id string)
}

// localRunnerPool stores runner objects in the local application memory
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

func (pool *localRunnerPool) AddRunner(r runner.Runner) {
	pool.Lock()
	defer pool.Unlock()
	pool.runners[r.Id()] = r
}

func (pool *localRunnerPool) GetRunner(id string) (r runner.Runner, ok bool) {
	pool.RLock()
	defer pool.RUnlock()
	r, ok = pool.runners[id]
	return
}

func (pool *localRunnerPool) DeleteRunner(id string) {
	pool.Lock()
	defer pool.Unlock()
	delete(pool.runners, id)
}
