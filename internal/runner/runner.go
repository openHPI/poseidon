package runner

import (
	"context"
	"github.com/influxdata/influxdb-client-go/v2/api/write"
	"github.com/openHPI/poseidon/pkg/dto"
	"github.com/openHPI/poseidon/pkg/monitoring"
	"github.com/openHPI/poseidon/pkg/storage"
	"io"
	"net/http"
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

	// Environment returns the id of the Environment to which the Runner belongs.
	Environment() dto.EnvironmentID

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

	// ListFileSystem streams the listing of the file system of the requested directory into the Writer provided.
	// The result is streamed via the io.Writer in order to not overload the memory with user input.
	ListFileSystem(path string, recursive bool, result io.Writer, privilegedExecution bool, ctx context.Context) error

	// UpdateFileSystem processes a dto.UpdateFileSystemRequest by first deleting each given dto.FilePath recursively
	// and then copying each given dto.File to the runner.
	UpdateFileSystem(request *dto.UpdateFileSystemRequest) error

	// GetFileContent streams the file content at the requested path into the Writer provided at content.
	// The result is streamed via the io.Writer in order to not overload the memory with user input.
	GetFileContent(path string, content http.ResponseWriter, privilegedExecution bool, ctx context.Context) error

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

// monitorExecutionsRunnerID passes the id of the runner executing the execution into the monitoring Point p.
func monitorExecutionsRunnerID(env dto.EnvironmentID, runnerID string) storage.WriteCallback[*dto.ExecutionRequest] {
	return func(p *write.Point, _ *dto.ExecutionRequest, _ storage.EventType) {
		p.AddTag(monitoring.InfluxKeyEnvironmentID, env.ToString())
		p.AddTag(monitoring.InfluxKeyRunnerID, runnerID)
	}
}
