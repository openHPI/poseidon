package execution

import (
	"github.com/openHPI/poseidon/pkg/dto"
	"sync"
)

// localStorage stores execution objects in the local application memory.
// ToDo: Create implementation that use some persistent storage like a database.
type localStorage struct {
	sync.RWMutex
	executions map[ID]*dto.ExecutionRequest
}

// NewLocalStorage responds with an Storer implementation.
// This implementation stores the data thread-safe in the local application memory.
func NewLocalStorage() *localStorage {
	return &localStorage{
		executions: make(map[ID]*dto.ExecutionRequest),
	}
}

func (s *localStorage) Add(id ID, executionRequest *dto.ExecutionRequest) {
	s.Lock()
	defer s.Unlock()
	s.executions[id] = executionRequest
}

func (s *localStorage) Exists(id ID) bool {
	s.Lock()
	defer s.Unlock()
	_, ok := s.executions[id]
	return ok
}

func (s *localStorage) Pop(id ID) (*dto.ExecutionRequest, bool) {
	s.Lock()
	defer s.Unlock()
	request, ok := s.executions[id]
	delete(s.executions, id)
	return request, ok
}