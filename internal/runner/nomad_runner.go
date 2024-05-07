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
	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
	"github.com/openHPI/poseidon/internal/nomad"
	"github.com/openHPI/poseidon/pkg/dto"
	"github.com/openHPI/poseidon/pkg/monitoring"
	"github.com/openHPI/poseidon/pkg/nullio"
	"github.com/openHPI/poseidon/pkg/storage"
	"github.com/openHPI/poseidon/pkg/util"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	// runnerContextKey is the key used to store runners in context.Context.
	runnerContextKey dto.ContextKey = "runner"
	// destroyReasonContextKey is the key used to store the reason of the destruction in the context.Context.
	destroyReasonContextKey dto.ContextKey = dto.KeyRunnerDestroyReason
	// SIGQUIT is the character that causes a tty to send the SIGQUIT signal to the controlled process.
	SIGQUIT = 0x1c
	// executionTimeoutGracePeriod is the time to wait after sending a SIGQUIT signal to a timed out execution.
	// If the execution does not return after this grace period, the runner is destroyed.
	executionTimeoutGracePeriod = 3 * time.Second
	// lsCommand is our format for parsing information of a file(system).
	lsCommand          = "ls -l --time-style=+%s -1 --literal"
	lsCommandRecursive = lsCommand + " --recursive"
)

var (
	ErrorUnknownExecution                  = errors.New("unknown execution")
	ErrorFileCopyFailed                    = errors.New("file copy failed")
	ErrFileNotFound                        = errors.New("file not found or insufficient permissions")
	ErrLocalDestruction      DestroyReason = nomad.ErrorLocalDestruction
	ErrOOMKilled             DestroyReason = nomad.ErrorOOMKilled
	ErrDestroyedByAPIRequest DestroyReason = errors.New("the client wants to stop the runner")
	ErrCannotStopExecution   DestroyReason = errors.New("the execution did not stop after SIGQUIT")
	ErrDestroyedAndReplaced  DestroyReason = fmt.Errorf("the runner will be destroyed and replaced: %w", ErrLocalDestruction)
	ErrEnvironmentUpdated    DestroyReason = errors.New("the environment will be destroyed and updated")
)

// NomadJob is an abstraction to communicate with Nomad environments.
type NomadJob struct {
	InactivityTimer
	executions   storage.Storage[*dto.ExecutionRequest]
	id           string
	portMappings []nomadApi.PortMapping
	api          nomad.ExecutorAPI
	onDestroy    DestroyRunnerHandler
	ctx          context.Context
	cancel       context.CancelFunc
}

// NewNomadJob creates a new NomadJob with the provided id.
// The InactivityTimer is used actively. It executes onDestroy when it has expired.
// The InactivityTimer is persisted in Nomad by the runner manager's Claim Function.
func NewNomadJob(id string, portMappings []nomadApi.PortMapping,
	apiClient nomad.ExecutorAPI, onDestroy DestroyRunnerHandler,
) *NomadJob {
	ctx := context.WithValue(context.Background(), dto.ContextKey(dto.KeyRunnerID), id)
	ctx, cancel := context.WithCancel(ctx)
	job := &NomadJob{
		id:           id,
		portMappings: portMappings,
		api:          apiClient,
		onDestroy:    onDestroy,
		ctx:          ctx,
		cancel:       cancel,
	}
	job.executions = storage.NewMonitoredLocalStorage[*dto.ExecutionRequest](
		monitoring.MeasurementExecutionsNomad, monitorExecutionsRunnerID(job.Environment(), id), time.Minute, ctx)
	job.InactivityTimer = NewInactivityTimer(job, func(r Runner) error {
		err := r.Destroy(ErrorRunnerInactivityTimeout)
		if err != nil {
			err = fmt.Errorf("NomadJob: %w", err)
		}
		return err
	})
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
	requestCtx context.Context,
) (<-chan ExitInfo, context.CancelFunc, error) {
	request, ok := r.executions.Pop(id)
	if !ok {
		return nil, nil, ErrorUnknownExecution
	}

	r.ResetTimeout()

	// We have to handle three contexts
	// - requestCtx:   The context of the http request (including Sentry data)
	// - r.ctx:        The context of the runner (runner timeout, or runner destroyed)
	// - executionCtx: The context of the execution (execution timeout)
	// -> The executionCtx cancel that might be triggered (when the client connection breaks)

	command, executionCtx, cancel := prepareExecution(request, r.ctx)
	exitInternal := make(chan ExitInfo)
	exit := make(chan ExitInfo, 1)
	ctxExecute, cancelExecute := context.WithCancel(requestCtx)

	go r.executeCommand(ctxExecute, command, request.PrivilegedExecution, stdin, stdout, stderr, exitInternal)
	go r.handleExitOrContextDone(executionCtx, cancelExecute, exitInternal, exit, stdin)

	return exit, cancel, nil
}

func (r *NomadJob) ListFileSystem(
	path string, recursive bool, content io.Writer, privilegedExecution bool, requestCtx context.Context) error {
	ctx := util.NewMergeContext([]context.Context{r.ctx, requestCtx})
	r.ResetTimeout()
	command := lsCommand
	if recursive {
		command = lsCommandRecursive
	}

	ls2json := &nullio.Ls2JsonWriter{Target: content, Ctx: ctx}
	defer ls2json.Close()
	retrieveCommand := (&dto.ExecutionRequest{Command: fmt.Sprintf("%s %q", command, path)}).FullCommand()
	exitCode, err := r.api.ExecuteCommand(r.id, ctx, retrieveCommand, false, privilegedExecution,
		&nullio.Reader{Ctx: ctx}, ls2json, io.Discard)
	switch {
	case ls2json.HasStartedWriting() && err == nil && exitCode == 0:
		// Successful. Nothing to do.
	case ls2json.HasStartedWriting():
		// if HasStartedWriting the status code of the response is already sent.
		// Therefore, we cannot notify CodeOcean about an error at this point anymore.
		log.WithError(err).WithField("exitCode", exitCode).Warn("Ignoring error of listing the file system")
		err = nil
	case err != nil:
		err = fmt.Errorf("%w: nomad error during retrieve file headers: %v",
			nomad.ErrorExecutorCommunicationFailed, err)
	case exitCode != 0:
		err = ErrFileNotFound
	case !ls2json.HasStartedWriting():
		err = fmt.Errorf("list file system failed silently: %w", nomad.ErrorExecutorCommunicationFailed)
	}
	return err
}

func (r *NomadJob) UpdateFileSystem(copyRequest *dto.UpdateFileSystemRequest, requestCtx context.Context) error {
	ctx := util.NewMergeContext([]context.Context{r.ctx, requestCtx})
	r.ResetTimeout()

	var tarBuffer bytes.Buffer
	if err := createTarArchiveForFiles(copyRequest.Copy, &tarBuffer, ctx); err != nil {
		return err
	}

	fileDeletionCommand := fileDeletionCommand(copyRequest.Delete)
	copyCommand := "tar --extract --absolute-names --verbose --file=/dev/stdin;"
	updateFileCommand := (&dto.ExecutionRequest{Command: fileDeletionCommand + copyCommand}).FullCommand()
	stdOut := bytes.Buffer{}
	stdErr := bytes.Buffer{}
	exitCode, err := r.api.ExecuteCommand(r.id, ctx, updateFileCommand, false,
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

func (r *NomadJob) GetFileContent(
	path string, content http.ResponseWriter, privilegedExecution bool, requestCtx context.Context) error {
	ctx := util.NewMergeContext([]context.Context{r.ctx, requestCtx})
	r.ResetTimeout()

	contentLengthWriter := &nullio.ContentLengthWriter{Target: content}
	p := influxdb2.NewPointWithMeasurement(monitoring.MeasurementFileDownload)
	p.AddTag(monitoring.InfluxKeyRunnerID, r.ID())
	environmentID, err := nomad.EnvironmentIDFromRunnerID(r.ID())
	if err != nil {
		log.WithContext(ctx).WithError(err).Warn("can not parse environment id")
	}
	p.AddTag(monitoring.InfluxKeyEnvironmentID, environmentID.ToString())
	defer contentLengthWriter.SendMonitoringData(p)

	retrieveCommand := (&dto.ExecutionRequest{
		Command: fmt.Sprintf("%s %q && cat %q", lsCommand, path, path),
	}).FullCommand()
	// Improve: Instead of using io.Discard use a **fixed-sized** buffer. With that we could improve the error message.
	exitCode, err := r.api.ExecuteCommand(r.id, ctx, retrieveCommand, false, privilegedExecution,
		&nullio.Reader{Ctx: ctx}, contentLengthWriter, io.Discard)

	if err != nil {
		return fmt.Errorf("%w: nomad error during retrieve file content copy: %v",
			nomad.ErrorExecutorCommunicationFailed, err)
	}
	if exitCode != 0 {
		return ErrFileNotFound
	}
	return nil
}

func (r *NomadJob) Destroy(reason DestroyReason) (err error) {
	r.ctx = context.WithValue(r.ctx, destroyReasonContextKey, reason)
	log.WithContext(r.ctx).Debug("Destroying Runner")
	r.cancel()
	r.StopTimeout()
	if r.onDestroy != nil {
		err = r.onDestroy(r)
		if err != nil {
			log.WithContext(r.ctx).WithError(err).Warn("runner onDestroy callback failed")
		}
	}

	if errors.Is(reason, ErrLocalDestruction) {
		log.WithContext(r.ctx).Debug("Runner destroyed locally")
		return nil
	}

	err = util.RetryExponential(func() (err error) {
		if err = r.api.DeleteJob(r.ID()); err != nil {
			err = fmt.Errorf("error deleting runner in Nomad: %w", err)
		}
		return
	})
	if err != nil {
		return fmt.Errorf("cannot destroy runner: %w", err)
	}
	log.WithContext(r.ctx).Trace("Runner destroyed")
	return nil
}

func prepareExecution(request *dto.ExecutionRequest, environmentCtx context.Context) (
	command string, ctx context.Context, cancel context.CancelFunc,
) {
	command = request.FullCommand()
	if request.TimeLimit == 0 {
		ctx, cancel = context.WithCancel(environmentCtx)
	} else {
		ctx, cancel = context.WithTimeout(environmentCtx, time.Duration(request.TimeLimit)*time.Second)
	}
	return command, ctx, cancel
}

func (r *NomadJob) executeCommand(ctx context.Context, command string, privilegedExecution bool,
	stdin io.ReadWriter, stdout, stderr io.Writer, exit chan<- ExitInfo,
) {
	exitCode, err := r.api.ExecuteCommand(r.id, ctx, command, true, privilegedExecution, stdin, stdout, stderr)
	select {
	case exit <- ExitInfo{uint8(exitCode), err}:
	case <-ctx.Done():
	}
}

func (r *NomadJob) handleExitOrContextDone(ctx context.Context, cancelExecute context.CancelFunc,
	exitInternal <-chan ExitInfo, exit chan<- ExitInfo, stdin io.ReadWriter,
) {
	defer cancelExecute()
	defer close(exit) // When this function has finished the connection to the executor is closed.

	select {
	case exitInfo := <-exitInternal:
		// - The execution ended in time or
		// - the HTTP request of the client/CodeOcean got canceled.
		r.handleExit(exitInfo, exitInternal, exit, stdin, ctx)
	case <-ctx.Done():
		// - The execution timeout was exceeded,
		// - the runner was destroyed (runner timeout, or API delete request), or
		// - the WebSocket connection to the client/CodeOcean closed.
		r.handleContextDone(exitInternal, exit, stdin, ctx)
	}
}

func (r *NomadJob) handleExit(exitInfo ExitInfo, exitInternal <-chan ExitInfo, exit chan<- ExitInfo,
	stdin io.ReadWriter, ctx context.Context) {
	// Special Handling of OOM Killed allocations. See #401.
	const exitCodeOfLikelyOOMKilledAllocation = 128
	const gracePeriodForConfirmingOOMKillReason = time.Second
	if exitInfo.Code == exitCodeOfLikelyOOMKilledAllocation {
		select {
		case <-ctx.Done():
			// We consider this allocation to be OOM Killed.
			r.handleContextDone(exitInternal, exit, stdin, ctx)
			return
		case <-time.After(gracePeriodForConfirmingOOMKillReason):
			// We consider that the payload code returned the exit code.
		}
	}

	exit <- exitInfo
}

func (r *NomadJob) handleContextDone(exitInternal <-chan ExitInfo, exit chan<- ExitInfo,
	stdin io.ReadWriter, ctx context.Context) {
	err := ctx.Err()
	if errors.Is(err, context.DeadlineExceeded) {
		err = ErrorExecutionTimeout
	} // for errors.Is(err, context.Canceled) the user likely disconnected from the execution.
	if reason, ok := r.ctx.Value(destroyReasonContextKey).(error); ok {
		err = reason
		if r.TimeoutPassed() && !errors.Is(err, ErrorRunnerInactivityTimeout) {
			log.WithError(err).Warn("Wrong destroy reason for expired runner")
		}
	}

	// From this time on the WebSocket connection to the client is closed in /internal/api/websocket.go
	// waitForExit. Input can still be sent to the executor.
	exit <- ExitInfo{255, err}

	// This condition prevents further interaction with a stopped / dead allocation.
	if errors.Is(err, nomad.ErrorOOMKilled) {
		return
	}

	// This injects the SIGQUIT character into the stdin. This character is parsed by the tty line discipline
	// (tty has to be true) and converted to a SIGQUIT signal sent to the foreground process attached to the tty.
	// By default, SIGQUIT causes the process to terminate and produces a core dump. Processes can catch this signal
	// and ignore it, which is why we destroy the runner if the process does not terminate after a grace period.
	_, err = stdin.Write([]byte{SIGQUIT})
	// if n != 1 {
	// The SIGQUIT is sent and correctly processed by the allocation.  However, for an unknown
	// reason, the number of bytes written is always zero even though the error is nil.
	// Hence, we disabled this sanity check here. See the MR for more details:
	// https://github.com/openHPI/poseidon/pull/45#discussion_r757029024
	// log.WithField("runner", r.id).Warn("Could not send SIGQUIT because nothing was written")
	// }
	if err != nil {
		log.WithContext(ctx).WithError(err).Warn("Could not send SIGQUIT due to error")
	}

	select {
	case <-exitInternal:
		log.WithContext(ctx).Debug("Execution terminated after SIGQUIT")
	case <-time.After(executionTimeoutGracePeriod):
		log.WithContext(ctx).Info(ErrCannotStopExecution.Error())
		if err := r.Destroy(ErrCannotStopExecution); err != nil {
			log.WithContext(ctx).Error("Error when destroying runner")
		}
	}
}

func createTarArchiveForFiles(filesToCopy []dto.File, w io.Writer, ctx context.Context) error {
	tarWriter := tar.NewWriter(w)
	for _, file := range filesToCopy {
		if err := tarWriter.WriteHeader(tarHeader(file)); err != nil {
			err := fmt.Errorf("error writing tar file header: %w", err)
			log.WithContext(ctx).
				WithField("path", base64.StdEncoding.EncodeToString([]byte(file.Path))).
				WithField("content", base64.StdEncoding.EncodeToString(file.Content)).
				Error(err)
			return err
		}
		if _, err := tarWriter.Write(file.ByteContent()); err != nil {
			err := fmt.Errorf("error writing tar file content: %w", err)
			log.WithContext(ctx).
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
	// See #236. Sticky bit is to allow creating files next to write-protected files.
	const directoryPermission int64 = 0o1777
	const filePermission int64 = 0o744

	if file.IsDirectory() {
		return &tar.Header{
			Typeflag: tar.TypeDir,
			Name:     file.CleanedPath(),
			Mode:     directoryPermission,
		}
	} else {
		return &tar.Header{
			Typeflag: tar.TypeReg,
			Name:     file.CleanedPath(),
			Mode:     filePermission,
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
