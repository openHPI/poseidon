package runner

import (
	"context"
	"encoding/json"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/api/dto"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/nomad"
	"io"
	"time"
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
	// Id returns the id of the runner.
	Id() string

	ExecutionStorage

	// Execute runs the given execution request and forwards from and to the given reader and writers.
	// An ExitInfo is sent to the exit channel on command completion.
	Execute(request *dto.ExecutionRequest, stdin io.Reader, stdout, stderr io.Writer) (exit <-chan ExitInfo, cancel context.CancelFunc)

	// Copy copies the specified files into the runner.
	Copy(dto.FileCreation)
}

// NomadAllocation is an abstraction to communicate with Nomad allocations.
type NomadAllocation struct {
	ExecutionStorage
	id  string
	api nomad.ExecutorApi
}

// NewRunner creates a new runner with the provided id.
func NewRunner(id string) Runner {
	return NewNomadAllocation(id, nil)
}

// NewNomadAllocation creates a new Nomad allocation with the provided id.
func NewNomadAllocation(id string, apiClient nomad.ExecutorApi) *NomadAllocation {
	return &NomadAllocation{
		id:               id,
		api:              apiClient,
		ExecutionStorage: NewLocalExecutionStorage(),
	}
}

func (r *NomadAllocation) Id() string {
	return r.id
}

type ExitInfo struct {
	Code uint8
	Err  error
}

func (r *NomadAllocation) Execute(request *dto.ExecutionRequest, stdin io.Reader, stdout, stderr io.Writer) (<-chan ExitInfo, context.CancelFunc) {
	command := request.FullCommand()
	var ctx context.Context
	var cancel context.CancelFunc
	if request.TimeLimit == 0 {
		ctx, cancel = context.WithCancel(context.Background())
	} else {
		ctx, cancel = context.WithTimeout(context.Background(), time.Duration(request.TimeLimit)*time.Second)
	}
	exit := make(chan ExitInfo)
	go func() {
		exitCode, err := r.api.ExecuteCommand(r.Id(), ctx, command, stdin, stdout, stderr)
		exit <- ExitInfo{uint8(exitCode), err}
		close(exit)
	}()
	return exit, cancel
}

func (r *NomadAllocation) Copy(files dto.FileCreation) {

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

// NewContext creates a context containing a runner.
func NewContext(ctx context.Context, runner Runner) context.Context {
	return context.WithValue(ctx, runnerContextKey, runner)
}

// FromContext returns a runner from a context.
func FromContext(ctx context.Context) (Runner, bool) {
	runner, ok := ctx.Value(runnerContextKey).(Runner)
	return runner, ok
}
