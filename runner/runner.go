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
	ErrorFileCopyFailed = errors.New("file copy failed")
)

type Runner interface {
	// Id returns the id of the runner.
	Id() string

	ExecutionStorage

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

func (r *NomadAllocation) ExecuteInteractively(
	request *dto.ExecutionRequest,
	stdin io.Reader,
	stdout, stderr io.Writer,
) (<-chan ExitInfo, context.CancelFunc) {
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
		exitCode, err := r.api.ExecuteCommand(r.Id(), ctx, command, true, stdin, stdout, stderr)
		exit <- ExitInfo{uint8(exitCode), err}
		close(exit)
	}()
	return exit, cancel
}

func (r *NomadAllocation) UpdateFileSystem(copyRequest *dto.UpdateFileSystemRequest) error {
	var tarBuffer bytes.Buffer
	if err := createTarArchiveForFiles(copyRequest.Copy, &tarBuffer); err != nil {
		return err
	}

	fileDeletionCommand := fileDeletionCommand(copyRequest.Delete)
	copyCommand := "tar --extract --absolute-names --verbose --file=/dev/stdin;"
	updateFileCommand := (&dto.ExecutionRequest{Command: fileDeletionCommand + copyCommand}).FullCommand()
	stdOut := bytes.Buffer{}
	stdErr := bytes.Buffer{}
	exitCode, err := r.api.ExecuteCommand(r.Id(), context.Background(), updateFileCommand, false,
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
