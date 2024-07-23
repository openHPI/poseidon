package runner

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/openHPI/poseidon/internal/config"
	"github.com/openHPI/poseidon/pkg/dto"
	"github.com/openHPI/poseidon/pkg/monitoring"
	"github.com/openHPI/poseidon/pkg/storage"
)

var ErrWrongMessageType = errors.New("received message that is not a text message")

type awsFunctionRequest struct {
	Action string                  `json:"action"`
	Cmd    []string                `json:"cmd"`
	Files  map[dto.FilePath][]byte `json:"files"`
}

// AWSFunctionWorkload is an abstraction to build a request to an AWS Lambda Function.
// It is not persisted on a Poseidon restart.
// The InactivityTimer is used actively. It stops listening to the Lambda function.
// AWS terminates the Lambda Function after the [Globals.Function.Timeout](deploy/aws/template.yaml).
type AWSFunctionWorkload struct {
	InactivityTimer
	id                string
	fs                map[dto.FilePath][]byte
	executions        storage.Storage[*dto.ExecutionRequest]
	runningExecutions map[string]context.CancelFunc
	onDestroy         DestroyRunnerHandler
	environment       ExecutionEnvironment
	//nolint:containedctx // See #630.
	ctx    context.Context
	cancel context.CancelFunc
}

// NewAWSFunctionWorkload creates a new AWSFunctionWorkload with the provided id.
func NewAWSFunctionWorkload(
	environment ExecutionEnvironment, onDestroy DestroyRunnerHandler,
) (*AWSFunctionWorkload, error) {
	newUUID, err := uuid.NewUUID()
	if err != nil {
		return nil, fmt.Errorf("failed generating runner id: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	workload := &AWSFunctionWorkload{
		id:                newUUID.String(),
		fs:                make(map[dto.FilePath][]byte),
		runningExecutions: make(map[string]context.CancelFunc),
		onDestroy:         onDestroy,
		environment:       environment,
		ctx:               ctx,
		cancel:            cancel,
	}
	workload.executions = storage.NewMonitoredLocalStorage[*dto.ExecutionRequest](
		monitoring.MeasurementExecutionsAWS, monitorExecutionsRunnerID(environment.ID(), workload.id), time.Minute, ctx)
	workload.InactivityTimer = NewInactivityTimer(workload, func(_ Runner) error {
		return workload.Destroy(nil)
	})
	return workload, nil
}

func (w *AWSFunctionWorkload) ID() string {
	return w.id
}

func (w *AWSFunctionWorkload) Environment() dto.EnvironmentID {
	return w.environment.ID()
}

func (w *AWSFunctionWorkload) MappedPorts() []*dto.MappedPort {
	return []*dto.MappedPort{}
}

func (w *AWSFunctionWorkload) StoreExecution(id string, request *dto.ExecutionRequest) {
	w.executions.Add(id, request)
}

func (w *AWSFunctionWorkload) ExecutionExists(id string) bool {
	_, ok := w.executions.Get(id)
	return ok
}

// ExecuteInteractively runs the execution request in an AWS function.
// It should be further improved by using the passed context to handle lost connections.
func (w *AWSFunctionWorkload) ExecuteInteractively(
	_ context.Context, id string, _ io.ReadWriter, stdout, stderr io.Writer) (
	<-chan ExitInfo, context.CancelFunc, error,
) {
	w.ResetTimeout()
	request, ok := w.executions.Pop(id)
	if !ok {
		return nil, nil, ErrUnknownExecution
	}
	hideEnvironmentVariables(request, "AWS")
	request.PrivilegedExecution = true // AWS does not support multiple users at this moment.
	command, ctx, cancel := prepareExecution(w.ctx, request)
	commands := []string{"/bin/bash", "-c", command}
	exitInternal := make(chan ExitInfo)
	exit := make(chan ExitInfo, 1)

	go w.executeCommand(ctx, commands, stdout, stderr, exitInternal)
	go w.handleRunnerTimeout(ctx, exitInternal, exit, id)

	return exit, cancel, nil
}

// ListFileSystem is currently not supported with this aws serverless function.
// This is because the function execution ends with the termination of the workload code.
// So an on-demand file system listing after the termination is not possible. Also, we do not want to copy all files.
func (w *AWSFunctionWorkload) ListFileSystem(_ context.Context, _ string, _ bool, _ io.Writer, _ bool) error {
	return dto.ErrNotSupported
}

// UpdateFileSystem copies Files into the executor.
// Current limitation: No files can be deleted apart from the previously added files.
// Future Work: Deduplication of the file systems, as the largest workload is likely to be used by additional
// CSV files or similar, which are the same for many executions.
func (w *AWSFunctionWorkload) UpdateFileSystem(_ context.Context, request *dto.UpdateFileSystemRequest) error {
	for _, path := range request.Delete {
		delete(w.fs, path)
	}
	for _, file := range request.Copy {
		w.fs[file.Path] = file.Content
	}
	return nil
}

// GetFileContent is currently not supported with this aws serverless function.
// This is because the function execution ends with the termination of the workload code.
// So an on-demand file streaming after the termination is not possible. Also, we do not want to copy all files.
func (w *AWSFunctionWorkload) GetFileContent(_ context.Context, _ string, _ http.ResponseWriter, _ bool) error {
	return dto.ErrNotSupported
}

func (w *AWSFunctionWorkload) Destroy(_ DestroyReason) error {
	w.cancel()
	if err := w.onDestroy(w); err != nil {
		return fmt.Errorf("error while destroying aws runner: %w", err)
	}
	return nil
}

func (w *AWSFunctionWorkload) executeCommand(ctx context.Context, command []string,
	stdout, stderr io.Writer, exit chan<- ExitInfo,
) {
	defer close(exit)
	data := &awsFunctionRequest{
		Action: w.environment.Image(),
		Cmd:    command,
		Files:  w.fs,
	}
	log.WithContext(ctx).WithField("request", data).Trace("Sending request to AWS")
	rawData, err := json.Marshal(data)
	if err != nil {
		exit <- ExitInfo{uint8(1), fmt.Errorf("cannot stingify aws function request: %w", err)}
		return
	}

	wsConn, response, err := websocket.DefaultDialer.Dial(config.Config.AWS.Endpoint, nil)
	if err != nil {
		exit <- ExitInfo{uint8(1), fmt.Errorf("failed to establish aws connection: %w", err)}
		return
	}
	_ = response.Body.Close()
	defer wsConn.Close()
	err = wsConn.WriteMessage(websocket.TextMessage, rawData)
	if err != nil {
		exit <- ExitInfo{uint8(1), fmt.Errorf("cannot send aws request: %w", err)}
		return
	}

	// receiveOutput listens for the execution timeout (or the exit code).
	exitCode, err := w.receiveOutput(ctx, wsConn, stdout, stderr)
	// TimeoutPassed checks the runner timeout
	if w.TimeoutPassed() {
		err = ErrRunnerInactivityTimeout
	}
	exit <- ExitInfo{exitCode, err}
}

func (w *AWSFunctionWorkload) receiveOutput(ctx context.Context,
	conn *websocket.Conn, stdout, stderr io.Writer,
) (uint8, error) {
	for ctx.Err() == nil {
		messageType, reader, err := conn.NextReader()
		if err != nil {
			return 1, fmt.Errorf("cannot read from aws connection: %w", err)
		}
		if messageType != websocket.TextMessage {
			return 1, ErrWrongMessageType
		}
		var wsMessage dto.WebSocketMessage
		err = json.NewDecoder(reader).Decode(&wsMessage)
		if err != nil {
			return 1, fmt.Errorf("failed to decode message from aws: %w", err)
		}

		log.WithField("msg", wsMessage).Info("New Message from AWS function")

		switch wsMessage.Type {
		default:
			log.WithContext(ctx).WithField("data", wsMessage).Warn("unexpected message from aws function")
		case dto.WebSocketExit:
			return wsMessage.ExitCode, nil
		case dto.WebSocketOutputStdout:
			// We do not check the written bytes as the rawToCodeOceanWriter receives everything or nothing.
			_, err = stdout.Write([]byte(wsMessage.Data))
		case dto.WebSocketOutputStderr, dto.WebSocketOutputError:
			_, err = stderr.Write([]byte(wsMessage.Data))
		}
		if err != nil {
			return 1, fmt.Errorf("failed to forward message: %w", err)
		}
	}
	return 1, fmt.Errorf("receiveOutput stpped by context: %w", ctx.Err())
}

// handleRunnerTimeout listens for a runner timeout and aborts the execution in that case.
// It listens via a context in runningExecutions that is canceled on the timeout event.
func (w *AWSFunctionWorkload) handleRunnerTimeout(ctx context.Context,
	exitInternal <-chan ExitInfo, exit chan<- ExitInfo, executionID string,
) {
	executionCtx, cancelExecution := context.WithCancel(ctx)
	w.runningExecutions[executionID] = cancelExecution
	defer delete(w.runningExecutions, executionID)
	defer close(exit)

	select {
	case exitInfo := <-exitInternal:
		exit <- exitInfo
	case <-executionCtx.Done():
		exit <- ExitInfo{255, ErrRunnerInactivityTimeout}
	}
}

// hideEnvironmentVariables sets the CODEOCEAN variable and unsets all variables starting with the passed prefix.
func hideEnvironmentVariables(request *dto.ExecutionRequest, unsetPrefix string) {
	if request.Environment == nil {
		request.Environment = make(map[string]string)
	}
	request.Command = "unset \"${!" + unsetPrefix + "@}\" && " + request.Command
}
