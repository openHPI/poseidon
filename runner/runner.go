package runner

import (
	"context"
	"encoding/json"
	"github.com/google/uuid"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/api/dto"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/logging"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/store"
	"sync"
	"time"
)

var log = logging.GetLogger("runner")

// Status is the type for the status of a Runner.
type Status string

// ContextKey is the type for keys in a request context.
type ContextKey string

// ExecutionId is an id for an execution in a Runner.
type ExecutionId string

const (
	StatusReady    Status = "ready"
	StatusRunning  Status = "running"
	StatusTimeout  Status = "timeout"
	StatusFinished Status = "finished"

	// runnerContextKey is the key used to store runners in context.Context
	runnerContextKey ContextKey = "runner"
)

type Runner interface {
	store.Entity

	// SetStatus sets the status of the runner.
	SetStatus(Status)

	// Status gets the status of the runner.
	Status() Status

	// Execution looks up an ExecutionId for the runner and returns the associated RunnerRequest.
	// If this request does not exit, ok is false, else true.
	Execution(ExecutionId) (request dto.ExecutionRequest, ok bool)

	// AddExecution saves the supplied ExecutionRequest for the runner and returns an ExecutionId to retrieve it again.
	AddExecution(dto.ExecutionRequest) (ExecutionId, error)

	// DeleteExecution deletes the execution of the runner with the specified id.
	DeleteExecution(ExecutionId)

	// Execute runs one of the runners Executions by it's id
	// ok will be false if the runner has no execution with the provided id
	Execute(ExecutionId) (err error, ok bool)
}

// ExerciseRunner is an abstraction to communicate with Nomad allocations.
type ExerciseRunner struct {
	sync.RWMutex
	id         string
	status     Status
	ch         chan bool
	executions map[ExecutionId]dto.ExecutionRequest
}

// NewExerciseRunner creates a new exercise runner with the provided id.
func NewExerciseRunner(id string) *ExerciseRunner {
	return &ExerciseRunner{
		id:         id,
		status:     StatusReady,
		ch:         make(chan bool),
		executions: make(map[ExecutionId]dto.ExecutionRequest),
	}
}

// MarshalJSON implements json.Marshaler interface.
// This exports private attributes like the id too.
func (r *ExerciseRunner) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Id     string `json:"runnerId"`
		Status Status `json:"status"`
	}{
		Id:     r.Id(),
		Status: r.Status(),
	})
}

func (r *ExerciseRunner) SetStatus(status Status) {
	r.Lock()
	defer r.Unlock()
	r.status = status
}

func (r *ExerciseRunner) Status() Status {
	r.RLock()
	defer r.RUnlock()
	return r.status
}

func (r *ExerciseRunner) Id() string {
	return r.id
}

func (r *ExerciseRunner) Execution(id ExecutionId) (executionRequest dto.ExecutionRequest, ok bool) {
	r.RLock()
	defer r.RUnlock()
	executionRequest, ok = r.executions[id]
	return
}

func (r *ExerciseRunner) AddExecution(request dto.ExecutionRequest) (ExecutionId, error) {
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

func (r *ExerciseRunner) DeleteExecution(id ExecutionId) {
	r.Lock()
	defer r.Unlock()
	delete(r.executions, id)
}

func (r *ExerciseRunner) Execute(id ExecutionId) (err error, ok bool) {
	execution, ok := r.Execution(id)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(execution.TimeLimit)*time.Second)
	defer cancel()
	log.WithField("Context", ctx).Printf("ToDo: Running execution")
	// ToDo: Implement command execution
	// r.nomadApiClient.ExecuteCommand(r.id, ctx, )
	return
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
