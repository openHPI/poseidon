package runner

import (
	"gitlab.hpi.de/codeocean/codemoon/poseidon/api/dto"
	"sync"
)

// ExecutionStorage stores executions.
type ExecutionStorage interface {
	// Add adds a runner to the storage.
	// It overwrites the existing execution if an execution with the same id already exists.
	Add(id ExecutionID, executionRequest *dto.ExecutionRequest)

	// Pop deletes the execution with the given id from the storage and returns it.
	// If no such execution exists, ok is false and true otherwise.
	Pop(id ExecutionID) (request *dto.ExecutionRequest, ok bool)
}

// localExecutionStorage stores execution objects in the local application memory.
// ToDo: Create implementation that use some persistent storage like a database.
type localExecutionStorage struct {
	sync.RWMutex
	executions map[ExecutionID]*dto.ExecutionRequest
}

// NewLocalExecutionStorage responds with an ExecutionStorage implementation.
// This implementation stores the data thread-safe in the local application memory.
func NewLocalExecutionStorage() *localExecutionStorage {
	return &localExecutionStorage{
		executions: make(map[ExecutionID]*dto.ExecutionRequest),
	}
}

func (s *localExecutionStorage) Add(id ExecutionID, executionRequest *dto.ExecutionRequest) {
	s.Lock()
	defer s.Unlock()
	s.executions[id] = executionRequest
}

func (s *localExecutionStorage) Pop(id ExecutionID) (*dto.ExecutionRequest, bool) {
	s.Lock()
	defer s.Unlock()
	request, ok := s.executions[id]
	delete(s.executions, id)
	return request, ok
}
