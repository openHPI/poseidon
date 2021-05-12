package runner

import (
	"context"
	"encoding/json"
	"github.com/google/uuid"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/api/dto"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/store"
	"sync"
)

// ContextKey is the type for keys in a request context.
type ContextKey string

// ExecutionId is an id for an execution in a Runner.
type ExecutionId string

const (
	// runnerContextKey is the key used to store runners in context.Context
	runnerContextKey ContextKey = "runner"
)

type Runner interface {
	store.Entity

	// AddExecution saves the supplied ExecutionRequest for the runner and returns an ExecutionId to retrieve it again.
	AddExecution(dto.ExecutionRequest) (ExecutionId, error)

	Execution(ExecutionId) (executionRequest dto.ExecutionRequest, ok bool)

	// DeleteExecution deletes the execution of the runner with the specified id.
	DeleteExecution(ExecutionId)

	// Execute executes the execution with the given ID.
	Execute(ExecutionId)

	// Copy copies the specified files into the runner.
	Copy(dto.FileCreation)
}

// NomadAllocation is an abstraction to communicate with Nomad allocations.
type NomadAllocation struct {
	sync.RWMutex
	id         string
	ch         chan bool
	executions map[ExecutionId]dto.ExecutionRequest
}

// NewRunner creates a new runner with the provided id.
func NewRunner(id string) Runner {
	return &NomadAllocation{
		id:         id,
		ch:         make(chan bool),
		executions: make(map[ExecutionId]dto.ExecutionRequest),
	}
}

// MarshalJSON implements json.Marshaler interface.
// This exports private attributes like the id too.
func (r *NomadAllocation) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Id string `json:"runnerId"`
	}{
		Id: r.Id(),
	})
}

func (r *NomadAllocation) Id() string {
	return r.id
}

func (r *NomadAllocation) Execution(id ExecutionId) (executionRequest dto.ExecutionRequest, ok bool) {
	r.RLock()
	defer r.RUnlock()
	executionRequest, ok = r.executions[id]
	return
}

func (r *NomadAllocation) AddExecution(request dto.ExecutionRequest) (ExecutionId, error) {
	r.Lock()
	defer r.Unlock()
	idUuid, err := uuid.NewRandom()
	if err != nil {
		return ExecutionId(""), err
	}
	id := ExecutionId(idUuid.String())
	r.executions[id] = request
	return id, err
}

func (r *NomadAllocation) Execute(id ExecutionId) {

}

func (r *NomadAllocation) Copy(files dto.FileCreation) {

}

func (r *NomadAllocation) DeleteExecution(id ExecutionId) {
	r.Lock()
	defer r.Unlock()
	delete(r.executions, id)
}

// NewContext creates a context containing a runner.
func NewContext(ctx context.Context, runner Runner) context.Context {
	return context.WithValue(ctx, runnerContextKey, runner)
}

// FromContext returns a runner from a context.
func FromContext(ctx context.Context) (Runner, bool) {
	runner, ok := ctx.Value(runnerContextKey).(Runner)
	return runner, ok
}
