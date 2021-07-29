package runner

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	nomadApi "github.com/hashicorp/nomad/api"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/internal/nomad"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/pkg/dto"
	"io"
	"strings"
	"time"
)

// ContextKey is the type for keys in a request context.
type ContextKey string

// ExecutionID is an id for an execution in a Runner.
type ExecutionID string

const (
	// runnerContextKey is the key used to store runners in context.Context.
	runnerContextKey ContextKey = "runner"
	// SIGQUIT is the character that causes a tty to send the SIGQUIT signal to the controlled process.
	SIGQUIT = 0x1c
	// executionTimeoutGracePeriod is the time to wait after sending a SIGQUIT signal to a timed out execution.
	// If the execution does not return after this grace period, the runner is destroyed.
	executionTimeoutGracePeriod = 3 * time.Second
)

var ErrorFileCopyFailed = errors.New("file copy failed")

type Runner interface {
	// ID returns the id of the runner.
	ID() string
	// MappedPorts returns the mapped ports of the runner.
	MappedPorts() []*dto.MappedPort

	ExecutionStorage
	InactivityTimer

	// ExecuteInteractively runs the given execution request and forwards from and to the given reader and writers.
	// An ExitInfo is sent to the exit channel on command completion.
	// Output from the runner is forwarded immediately.
	ExecuteInteractively(
		request *dto.ExecutionRequest,
		stdin io.ReadWriter,
		stdout,
		stderr io.Writer,
	) (exit <-chan ExitInfo, cancel context.CancelFunc)

	// UpdateFileSystem processes a dto.UpdateFileSystemRequest by first deleting each given dto.FilePath recursively
	// and then copying each given dto.File to the runner.
	UpdateFileSystem(request *dto.UpdateFileSystemRequest) error

	// Destroy destroys the Runner in Nomad.
	Destroy() error
}

// NomadJob is an abstraction to communicate with Nomad environments.
type NomadJob struct {
	ExecutionStorage
	InactivityTimer
	id           string
	portMappings []nomadApi.PortMapping
	api          nomad.ExecutorAPI
	manager      Manager
}

// NewNomadJob creates a new NomadJob with the provided id.
func NewNomadJob(id string, portMappings []nomadApi.PortMapping,
	apiClient nomad.ExecutorAPI, manager Manager,
) *NomadJob {
	job := &NomadJob{
		id:               id,
		portMappings:     portMappings,
		api:              apiClient,
		ExecutionStorage: NewLocalExecutionStorage(),
		manager:          manager,
	}
	job.InactivityTimer = NewInactivityTimer(job, manager)
	return job
}

func (r *NomadJob) ID() string {
	return r.id
}

func (r *NomadJob) MappedPorts() []*dto.MappedPort {
	ports := make([]*dto.MappedPort, 0, len(r.portMappings))
	for _, portMapping := range r.portMappings {
		ports = append(ports, &dto.MappedPort{
			ExposedPort: uint(portMapping.To),
			HostAddress: fmt.Sprintf("%s:%d", portMapping.HostIP, portMapping.Value),
		})
	}
	return ports
}

type ExitInfo struct {
	Code uint8
	Err  error
}

func prepareExecution(request *dto.ExecutionRequest) (
	command []string, ctx context.Context, cancel context.CancelFunc,
) {
	command = request.FullCommand()
	if request.TimeLimit == 0 {
		ctx, cancel = context.WithCancel(context.Background())
	} else {
		ctx, cancel = context.WithTimeout(context.Background(), time.Duration(request.TimeLimit)*time.Second)
	}
	return command, ctx, cancel
}

func (r *NomadJob) executeCommand(ctx context.Context, command []string,
	stdin io.ReadWriter, stdout, stderr io.Writer, exit chan<- ExitInfo,
) {
	exitCode, err := r.api.ExecuteCommand(r.id, ctx, command, true, stdin, stdout, stderr)
	if err == nil && r.TimeoutPassed() {
		err = ErrorRunnerInactivityTimeout
	}
	exit <- ExitInfo{uint8(exitCode), err}
}

func (r *NomadJob) handleExitOrContextDone(ctx context.Context, cancelExecute context.CancelFunc,
	exitInternal <-chan ExitInfo, exit chan<- ExitInfo, stdin io.ReadWriter,
) {
	defer cancelExecute()
	select {
	case exitInfo := <-exitInternal:
		exit <- exitInfo
		close(exit)
		return
	case <-ctx.Done():
		// From this time on until the WebSocket connection to the client is closed in /internal/api/websocket.go
		// waitForExit, output can still be forwarded to the client. We accept this race condition because adding
		// a locking mechanism would complicate the interfaces used (currently io.Writer).
		exit <- ExitInfo{255, ctx.Err()}
		close(exit)
	}
	// This injects the SIGQUIT character into the stdin. This character is parsed by the tty line discipline
	// (tty has to be true) and converted to a SIGQUIT signal sent to the foreground process attached to the tty.
	// By default, SIGQUIT causes the process to terminate and produces a core dump. Processes can catch this signal
	// and ignore it, which is why we destroy the runner if the process does not terminate after a grace period.
	n, err := stdin.Write([]byte{SIGQUIT})
	if n != 1 {
		log.WithField("runner", r.id).Warn("Could not send SIGQUIT because nothing was written")
	}
	if err != nil {
		log.WithField("runner", r.id).WithError(err).Warn("Could not send SIGQUIT due to error")
	}

	select {
	case <-exitInternal:
		log.WithField("runner", r.id).Debug("Execution terminated after SIGQUIT")
	case <-time.After(executionTimeoutGracePeriod):
		log.WithField("runner", r.id).Info("Execution did not quit after SIGQUIT")
		if err := r.Destroy(); err != nil {
			log.WithField("runner", r.id).Error("Error when destroying runner")
		}
	}
}

func (r *NomadJob) ExecuteInteractively(
	request *dto.ExecutionRequest,
	stdin io.ReadWriter,
	stdout, stderr io.Writer,
) (<-chan ExitInfo, context.CancelFunc) {
	r.ResetTimeout()

	command, ctx, cancel := prepareExecution(request)
	exitInternal := make(chan ExitInfo)
	exit := make(chan ExitInfo, 1)
	ctxExecute, cancelExecute := context.WithCancel(context.Background())
	go r.executeCommand(ctxExecute, command, stdin, stdout, stderr, exitInternal)
	go r.handleExitOrContextDone(ctx, cancelExecute, exitInternal, exit, stdin)
	return exit, cancel
}

func (r *NomadJob) UpdateFileSystem(copyRequest *dto.UpdateFileSystemRequest) error {
	r.ResetTimeout()

	var tarBuffer bytes.Buffer
	if err := createTarArchiveForFiles(copyRequest.Copy, &tarBuffer); err != nil {
		return err
	}

	fileDeletionCommand := fileDeletionCommand(copyRequest.Delete)
	copyCommand := "tar --extract --absolute-names --verbose --file=/dev/stdin;"
	updateFileCommand := (&dto.ExecutionRequest{Command: fileDeletionCommand + copyCommand}).FullCommand()
	stdOut := bytes.Buffer{}
	stdErr := bytes.Buffer{}
	exitCode, err := r.api.ExecuteCommand(r.id, context.Background(), updateFileCommand, false,
		&tarBuffer, &stdOut, &stdErr)

	if err != nil {
		return fmt.Errorf(
			"%w: nomad error during file copy: %v",
			nomad.ErrorExecutorCommunicationFailed,
			err)
	}
	if exitCode != 0 {
		return fmt.Errorf(
			"%w: stderr output '%s' and stdout output '%s'",
			ErrorFileCopyFailed,
			stdErr.String(),
			stdOut.String())
	}
	return nil
}

func (r *NomadJob) Destroy() error {
	if err := r.manager.Return(r); err != nil {
		return fmt.Errorf("error while destroying runner: %w", err)
	}
	return nil
}

func createTarArchiveForFiles(filesToCopy []dto.File, w io.Writer) error {
	tarWriter := tar.NewWriter(w)
	for _, file := range filesToCopy {
		if err := tarWriter.WriteHeader(tarHeader(file)); err != nil {
			err := fmt.Errorf("error writing tar file header: %w", err)
			log.
				WithField("file", file).
				Error(err)
			return err
		}
		if _, err := tarWriter.Write(file.ByteContent()); err != nil {
			err := fmt.Errorf("error writing tar file content: %w", err)
			log.
				WithField("file", file).
				Error(err)
			return err
		}
	}
	if err := tarWriter.Close(); err != nil {
		return fmt.Errorf("error closing tar writer: %w", err)
	}
	return nil
}

func fileDeletionCommand(pathsToDelete []dto.FilePath) string {
	if len(pathsToDelete) == 0 {
		return ""
	}
	command := "rm --recursive --force "
	for _, filePath := range pathsToDelete {
		// To avoid command injection, filenames need to be quoted.
		// See https://unix.stackexchange.com/questions/347332/what-characters-need-to-be-escaped-in-files-without-quotes
		// for details.
		singleQuoteEscapedFileName := strings.ReplaceAll(filePath.Cleaned(), "'", "'\\''")
		command += fmt.Sprintf("'%s' ", singleQuoteEscapedFileName)
	}
	command += ";"
	return command
}

func tarHeader(file dto.File) *tar.Header {
	if file.IsDirectory() {
		return &tar.Header{
			Typeflag: tar.TypeDir,
			Name:     file.CleanedPath(),
			Mode:     0755,
		}
	} else {
		return &tar.Header{
			Typeflag: tar.TypeReg,
			Name:     file.CleanedPath(),
			Mode:     0744,
			Size:     int64(len(file.Content)),
		}
	}
}

// MarshalJSON implements json.Marshaler interface.
// This exports private attributes like the id too.
func (r *NomadJob) MarshalJSON() ([]byte, error) {
	res, err := json.Marshal(struct {
		ID string `json:"runnerId"`
	}{
		ID: r.ID(),
	})
	if err != nil {
		return nil, fmt.Errorf("error marshaling Nomad job: %w", err)
	}
	return res, nil
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
