package runner

import (
	"context"
	"github.com/openHPI/poseidon/pkg/dto"
	"io"
)

type ExitInfo struct {
	Code uint8
	Err  error
}

type DestroyRunnerHandler = func(r Runner) error

type Runner interface {
	InactivityTimer

	// ID returns the id of the runner.
	ID() string

	// MappedPorts returns the mapped ports of the runner.
	MappedPorts() []*dto.MappedPort

	// StoreExecution adds a new execution to the runner that can then be executed using ExecuteInteractively.
	StoreExecution(id string, executionRequest *dto.ExecutionRequest)

	// ExecutionExists returns whether the execution with the given id is already stored.
	ExecutionExists(id string) bool

	// ExecuteInteractively runs the given execution request and forwards from and to the given reader and writers.
	// An ExitInfo is sent to the exit channel on command completion.
	// Output from the runner is forwarded immediately.
	ExecuteInteractively(
		id string,
		stdin io.ReadWriter,
		stdout,
		stderr io.Writer,
	) (exit <-chan ExitInfo, cancel context.CancelFunc, err error)

	// UpdateFileSystem processes a dto.UpdateFileSystemRequest by first deleting each given dto.FilePath recursively
	// and then copying each given dto.File to the runner.
	UpdateFileSystem(request *dto.UpdateFileSystemRequest) error

	// Destroy destroys the Runner in Nomad.
	Destroy() error
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
