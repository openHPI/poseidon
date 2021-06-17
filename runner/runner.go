package runner

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/api/dto"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/nomad"
	"io"
	"strings"
	"sync"
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

var (
	ErrorFileCopyFailed          = errors.New("file copy failed")
	ErrorRunnerInactivityTimeout = errors.New("runner inactivity timeout exceeded")
)

// InactivityTimer is a wrapper around a timer that is used to delete a a Runner after some time of inactivity.
type InactivityTimer interface {
	// SetupTimeout starts the timeout after a runner gets deleted.
	SetupTimeout(duration time.Duration)

	// ResetTimeout resets the current timeout so that the runner gets deleted after the time set in Setup from now.
	// It does not make an already expired timer run again.
	ResetTimeout()

	// StopTimeout stops the timeout but does not remove the runner.
	StopTimeout()

	// TimeoutPassed returns true if the timeout expired and false otherwise.
	TimeoutPassed() bool
}

type TimerState uint8

const (
	TimerInactive TimerState = 0
	TimerRunning  TimerState = 1
	TimerExpired  TimerState = 2
)

type InactivityTimerImplementation struct {
	timer    *time.Timer
	duration time.Duration
	state    TimerState
	runner   Runner
	manager  Manager
	sync.Mutex
}

func NewInactivityTimer(runner Runner, manager Manager) InactivityTimer {
	return &InactivityTimerImplementation{
		state:   TimerInactive,
		runner:  runner,
		manager: manager,
	}
}

func (t *InactivityTimerImplementation) SetupTimeout(duration time.Duration) {
	t.Lock()
	defer t.Unlock()
	// Stop old timer if present.
	if t.timer != nil {
		t.timer.Stop()
	}
	if duration == 0 {
		t.state = TimerInactive
		return
	}
	t.state = TimerRunning
	t.duration = duration

	t.timer = time.AfterFunc(duration, func() {
		t.Lock()
		t.state = TimerExpired
		// The timer must be unlocked here already in order to avoid a deadlock with the call to StopTimout in Manager.Return.
		t.Unlock()
		err := t.manager.Return(t.runner)
		if err != nil {
			log.WithError(err).WithField("id", t.runner.Id()).Warn("Returning runner after inactivity caused an error")
		} else {
			log.WithField("id", t.runner.Id()).Info("Returning runner due to inactivity timeout")
		}
	})
}

func (t *InactivityTimerImplementation) ResetTimeout() {
	t.Lock()
	defer t.Unlock()
	if t.state != TimerRunning {
		// The timer has already expired or been stopped. We don't want to restart it.
		return
	}
	if t.timer.Stop() {
		t.timer.Reset(t.duration)
	} else {
		log.Error("Timer is in state running but stopped. This should never happen")
	}
}

func (t *InactivityTimerImplementation) StopTimeout() {
	t.Lock()
	defer t.Unlock()
	if t.state != TimerRunning {
		return
	}
	t.timer.Stop()
	t.state = TimerInactive
}

func (t *InactivityTimerImplementation) TimeoutPassed() bool {
	return t.state == TimerExpired
}

type Runner interface {
	// Id returns the id of the runner.
	Id() string

	ExecutionStorage
	InactivityTimer

	// ExecuteInteractively runs the given execution request and forwards from and to the given reader and writers.
	// An ExitInfo is sent to the exit channel on command completion.
	// Output from the runner is forwarded immediately.
	ExecuteInteractively(
		request *dto.ExecutionRequest,
		stdin io.Reader,
		stdout,
		stderr io.Writer,
	) (exit <-chan ExitInfo, cancel context.CancelFunc)

	// UpdateFileSystem processes a dto.UpdateFileSystemRequest by first deleting each given dto.FilePath recursively
	// and then copying each given dto.File to the runner.
	UpdateFileSystem(request *dto.UpdateFileSystemRequest) error
}

// NomadJob is an abstraction to communicate with Nomad environments.
type NomadJob struct {
	ExecutionStorage
	InactivityTimer
	id  string
	api nomad.ExecutorAPI
}

// NewNomadJob creates a new NomadJob with the provided id.
func NewNomadJob(id string, apiClient nomad.ExecutorAPI, manager Manager) *NomadJob {
	job := &NomadJob{
		id:               id,
		api:              apiClient,
		ExecutionStorage: NewLocalExecutionStorage(),
	}
	job.InactivityTimer = NewInactivityTimer(job, manager)
	return job
}

func (r *NomadJob) Id() string {
	return r.id
}

type ExitInfo struct {
	Code uint8
	Err  error
}

func (r *NomadJob) ExecuteInteractively(
	request *dto.ExecutionRequest,
	stdin io.Reader,
	stdout, stderr io.Writer,
) (<-chan ExitInfo, context.CancelFunc) {
	r.ResetTimeout()

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
		exitCode, err := r.api.ExecuteCommand(r.id, ctx, command, true, stdin, stdout, stderr)
		if err == nil && r.TimeoutPassed() {
			err = ErrorRunnerInactivityTimeout
		}
		exit <- ExitInfo{uint8(exitCode), err}
		close(exit)
	}()
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

func createTarArchiveForFiles(filesToCopy []dto.File, w io.Writer) error {
	tarWriter := tar.NewWriter(w)
	for _, file := range filesToCopy {
		if err := tarWriter.WriteHeader(tarHeader(file)); err != nil {
			log.
				WithError(err).
				WithField("file", file).
				Error("Error writing tar file header")
			return err
		}
		if _, err := tarWriter.Write(file.ByteContent()); err != nil {
			log.
				WithError(err).
				WithField("file", file).
				Error("Error writing tar file content")
			return err
		}
	}
	return tarWriter.Close()
}

func fileDeletionCommand(pathsToDelete []dto.FilePath) string {
	if len(pathsToDelete) == 0 {
		return ""
	}
	command := "rm --recursive --force "
	for _, filePath := range pathsToDelete {
		// To avoid command injection, filenames need to be quoted.
		// See https://unix.stackexchange.com/questions/347332/what-characters-need-to-be-escaped-in-files-without-quotes for details.
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
	return json.Marshal(struct {
		ID string `json:"runnerId"`
	}{
		ID: r.Id(),
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
