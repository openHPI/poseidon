package runner

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	nomadApi "github.com/hashicorp/nomad/api"
	"github.com/openHPI/poseidon/internal/nomad"
	"github.com/openHPI/poseidon/pkg/dto"
	"github.com/openHPI/poseidon/pkg/monitoring"
	"github.com/openHPI/poseidon/pkg/nullio"
	"github.com/openHPI/poseidon/pkg/storage"
	"io"
	"strings"
	"time"
)

const (
	// runnerContextKey is the key used to store runners in context.Context.
	runnerContextKey dto.ContextKey = "runner"
	// SIGQUIT is the character that causes a tty to send the SIGQUIT signal to the controlled process.
	SIGQUIT = 0x1c
	// executionTimeoutGracePeriod is the time to wait after sending a SIGQUIT signal to a timed out execution.
	// If the execution does not return after this grace period, the runner is destroyed.
	executionTimeoutGracePeriod = 3 * time.Second
)

var (
	ErrorUnknownExecution = errors.New("unknown execution")
	ErrorFileCopyFailed   = errors.New("file copy failed")
	ErrFileNotFound       = errors.New("file not found")
)

// NomadJob is an abstraction to communicate with Nomad environments.
type NomadJob struct {
	InactivityTimer
	executions   storage.Storage[*dto.ExecutionRequest]
	id           string
	portMappings []nomadApi.PortMapping
	api          nomad.ExecutorAPI
	onDestroy    DestroyRunnerHandler
}

// NewNomadJob creates a new NomadJob with the provided id.
// The InactivityTimer is used actively. It executes onDestroy when it has expired.
// The InactivityTimer is persisted in Nomad by the runner manager's Claim Function.
func NewNomadJob(id string, portMappings []nomadApi.PortMapping,
	apiClient nomad.ExecutorAPI, onDestroy DestroyRunnerHandler,
) *NomadJob {
	job := &NomadJob{
		id:           id,
		portMappings: portMappings,
		api:          apiClient,
		onDestroy:    onDestroy,
	}
	job.executions = storage.NewMonitoredLocalStorage[*dto.ExecutionRequest](
		monitoring.MeasurementExecutionsNomad, monitorExecutionsRunnerID(job.Environment(), id), time.Minute)
	job.InactivityTimer = NewInactivityTimer(job, onDestroy)
	return job
}

func (r *NomadJob) ID() string {
	return r.id
}

func (r *NomadJob) Environment() dto.EnvironmentID {
	id, err := nomad.EnvironmentIDFromRunnerID(r.ID())
	if err != nil {
		log.WithError(err).Error("Runners must have correct IDs")
	}
	return id
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

func (r *NomadJob) StoreExecution(id string, request *dto.ExecutionRequest) {
	r.executions.Add(id, request)
}

func (r *NomadJob) ExecutionExists(id string) bool {
	_, ok := r.executions.Get(id)
	return ok
}

func (r *NomadJob) ExecuteInteractively(
	id string,
	stdin io.ReadWriter,
	stdout, stderr io.Writer,
) (<-chan ExitInfo, context.CancelFunc, error) {
	request, ok := r.executions.Pop(id)
	if !ok {
		return nil, nil, ErrorUnknownExecution
	}

	r.ResetTimeout()

	command, ctx, cancel := prepareExecution(request)
	exitInternal := make(chan ExitInfo)
	exit := make(chan ExitInfo, 1)
	ctxExecute, cancelExecute := context.WithCancel(context.Background())

	go r.executeCommand(ctxExecute, command, request.PrivilegedExecution, stdin, stdout, stderr, exitInternal)
	go r.handleExitOrContextDone(ctx, cancelExecute, exitInternal, exit, stdin)

	return exit, cancel, nil
}

func (r *NomadJob) ListFileSystem(path string, recursive bool, content io.Writer, ctx context.Context) error {
	r.ResetTimeout()
	command := "ls -l --time-style=+%s -1 --literal"
	if recursive {
		command += " --recursive"
	}

	ls2json := &nullio.Ls2JsonWriter{Target: content}
	defer ls2json.Close()
	retrieveCommand := (&dto.ExecutionRequest{Command: fmt.Sprintf("%s %q", command, path)}).FullCommand()
	exitCode, err := r.api.ExecuteCommand(r.id, ctx, retrieveCommand, false, &nullio.Reader{}, ls2json, io.Discard)
	if err != nil {
		return fmt.Errorf("%w: nomad error during retrieve file headers: %v",
			nomad.ErrorExecutorCommunicationFailed, err)
	}
	if exitCode != 0 {
		return ErrFileNotFound
	}
	return nil
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
		nomad.PrivilegedExecution, // All files should be written and owned by a privileged user #211.
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

func (r *NomadJob) GetFileContent(path string, content io.Writer, ctx context.Context) error {
	r.ResetTimeout()

	retrieveCommand := (&dto.ExecutionRequest{Command: fmt.Sprintf("cat %q", path)}).FullCommand()
	// Improve: Instead of using io.Discard use a **fixed-sized** buffer. With that we could improve the error message.
	exitCode, err := r.api.ExecuteCommand(r.id, ctx, retrieveCommand, false, &nullio.Reader{}, content, io.Discard)

	if err != nil {
		return fmt.Errorf("%w: nomad error during retrieve file content copy: %v",
			nomad.ErrorExecutorCommunicationFailed, err)
	}
	if exitCode != 0 {
		return ErrFileNotFound
	}
	return nil
}

func (r *NomadJob) Destroy() error {
	if err := r.onDestroy(r); err != nil {
		return fmt.Errorf("error while destroying runner: %w", err)
	}
	return nil
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

func (r *NomadJob) executeCommand(ctx context.Context, command []string, privilegedExecution bool,
	stdin io.ReadWriter, stdout, stderr io.Writer, exit chan<- ExitInfo,
) {
	exitCode, err := r.api.ExecuteCommand(r.id, ctx, command, true, privilegedExecution, stdin, stdout, stderr)
	if err == nil && r.TimeoutPassed() {
		err = ErrorRunnerInactivityTimeout
	}
	exit <- ExitInfo{uint8(exitCode), err}
}

func (r *NomadJob) handleExitOrContextDone(ctx context.Context, cancelExecute context.CancelFunc,
	exitInternal <-chan ExitInfo, exit chan<- ExitInfo, stdin io.ReadWriter,
) {
	defer cancelExecute()
	defer close(exit) // When this function has finished the connection to the executor is closed.

	select {
	case exitInfo := <-exitInternal:
		exit <- exitInfo
		return
	case <-ctx.Done():
	}

	// From this time on the WebSocket connection to the client is closed in /internal/api/websocket.go
	// waitForExit. Input can still be sent to the executor.
	exit <- ExitInfo{255, ctx.Err()}

	// This injects the SIGQUIT character into the stdin. This character is parsed by the tty line discipline
	// (tty has to be true) and converted to a SIGQUIT signal sent to the foreground process attached to the tty.
	// By default, SIGQUIT causes the process to terminate and produces a core dump. Processes can catch this signal
	// and ignore it, which is why we destroy the runner if the process does not terminate after a grace period.
	_, err := stdin.Write([]byte{SIGQUIT})
	// if n != 1 {
	// The SIGQUIT is sent and correctly processed by the allocation.  However, for an unknown
	// reason, the number of bytes written is always zero even though the error is nil.
	// Hence, we disabled this sanity check here. See the MR for more details:
	// https://github.com/openHPI/poseidon/pull/45#discussion_r757029024
	// log.WithField("runner", r.id).Warn("Could not send SIGQUIT because nothing was written")
	// }
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

func createTarArchiveForFiles(filesToCopy []dto.File, w io.Writer) error {
	tarWriter := tar.NewWriter(w)
	for _, file := range filesToCopy {
		if err := tarWriter.WriteHeader(tarHeader(file)); err != nil {
			err := fmt.Errorf("error writing tar file header: %w", err)
			log.
				WithField("path", base64.StdEncoding.EncodeToString([]byte(file.Path))).
				WithField("content", base64.StdEncoding.EncodeToString(file.Content)).
				Error(err)
			return err
		}
		if _, err := tarWriter.Write(file.ByteContent()); err != nil {
			err := fmt.Errorf("error writing tar file content: %w", err)
			log.
				WithField("path", base64.StdEncoding.EncodeToString([]byte(file.Path))).
				WithField("content", base64.StdEncoding.EncodeToString(file.Content)).
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
		if filePath == "./*" {
			command += "./* "
		} else {
			// To avoid command injection, filenames need to be quoted.
			// See https://unix.stackexchange.com/questions/347332/what-characters-need-to-be-escaped-in-files-without-quotes
			// for details.
			singleQuoteEscapedFileName := strings.ReplaceAll(filePath.Cleaned(), "'", "'\\''")
			command += fmt.Sprintf("'%s' ", singleQuoteEscapedFileName)
		}
	}
	command += ";"
	return command
}

func tarHeader(file dto.File) *tar.Header {
	if file.IsDirectory() {
		return &tar.Header{
			Typeflag: tar.TypeDir,
			Name:     file.CleanedPath(),
			Mode:     0o755,
		}
	} else {
		return &tar.Header{
			Typeflag: tar.TypeReg,
			Name:     file.CleanedPath(),
			Mode:     0o744,
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
