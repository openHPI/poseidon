package runner

import (
	"context"
	"encoding/json"
	"sync"
)

type Status string
type ContextKey string

const (
	StatusReady    Status = "ready"
	StatusRunning  Status = "running"
	StatusTimeout  Status = "timeout"
	StatusFinished Status = "finished"

	// runnerContextKey is the key used to store runners in context.Context
	runnerContextKey ContextKey = "runner"
)

type Runner interface {
	SetStatus(Status)
	Status() Status
	Id() string
}

// ExerciseRunner is an abstraction to communicate with Nomad allocations
type ExerciseRunner struct {
	sync.Mutex
	id     string
	status Status
	ch     chan bool
}

// NewExerciseRunner creates a new exercise runner with the provided id
// As default value the status is ready
func NewExerciseRunner(id string) *ExerciseRunner {
	return &ExerciseRunner{
		id:     id,
		status: StatusReady,
		Mutex:  sync.Mutex{},
		ch:     make(chan bool),
	}
}

// MarshalJSON implements json.Marshaler interface
// This exports also private attributes like the id
func (r *ExerciseRunner) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Id     string `json:"runnerId"`
		Status Status `json:"status"`
	}{
		Id:     r.Id(),
		Status: r.Status(),
	})
}

// SetStatus sets the status thread-safe
func (r *ExerciseRunner) SetStatus(status Status) {
	r.Lock()
	defer r.Unlock()
	r.status = status
}

// Status returns the status thread-safe
func (r *ExerciseRunner) Status() Status {
	r.Lock()
	defer r.Unlock()
	return r.status
}

// Id returns the id of the runner
func (r *ExerciseRunner) Id() string {
	return r.id
}

func NewContext(ctx context.Context, runner Runner) context.Context {
	return context.WithValue(ctx, runnerContextKey, runner)
}

func FromContext(ctx context.Context) (Runner, bool) {
	runner, ok := ctx.Value(runnerContextKey).(Runner)
	return runner, ok
}
