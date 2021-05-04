package runner

import (
	"encoding/json"
	"sync"
)

type Status string

const (
	StatusReady    Status = "ready"
	StatusRunning  Status = "running"
	StatusTimeout  Status = "timeout"
	StatusFinished Status = "finished"
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
