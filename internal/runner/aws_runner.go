package runner

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/openHPI/poseidon/internal/config"
	"github.com/openHPI/poseidon/pkg/dto"
	"github.com/openHPI/poseidon/pkg/execution"
	"io"
)

var ErrWrongMessageType = errors.New("received message that is not a text messages")

type awsFunctionRequest struct {
	Action string                  `json:"action"`
	Cmd    []string                `json:"cmd"`
	Files  map[dto.FilePath][]byte `json:"files"`
}

// AWSFunctionWorkload is an abstraction to build a request to an AWS Lambda Function.
type AWSFunctionWorkload struct {
	InactivityTimer
	id                string
	fs                map[dto.FilePath][]byte
	executions        execution.Storer
	runningExecutions map[execution.ID]context.CancelFunc
	onDestroy         DestroyRunnerHandler
	environment       ExecutionEnvironment
}

// NewAWSFunctionWorkload creates a new AWSFunctionWorkload with the provided id.
func NewAWSFunctionWorkload(
	environment ExecutionEnvironment, onDestroy DestroyRunnerHandler) (*AWSFunctionWorkload, error) {
	newUUID, err := uuid.NewUUID()
	if err != nil {
		return nil, fmt.Errorf("failed generating runner id: %w", err)
	}

	workload := &AWSFunctionWorkload{
		id:                newUUID.String(),
		fs:                make(map[dto.FilePath][]byte),
		executions:        execution.NewLocalStorage(),
		runningExecutions: make(map[execution.ID]context.CancelFunc),
		onDestroy:         onDestroy,
		environment:       environment,
	}
	workload.InactivityTimer = NewInactivityTimer(workload, func(_ Runner) error {
		return workload.Destroy()
	})
	return workload, nil
}

func (w *AWSFunctionWorkload) ID() string {
	return w.id
}

func (w *AWSFunctionWorkload) MappedPorts() []*dto.MappedPort {
	return []*dto.MappedPort{}
}

func (w *AWSFunctionWorkload) StoreExecution(id string, request *dto.ExecutionRequest) {
	w.executions.Add(execution.ID(id), request)
}

func (w *AWSFunctionWorkload) ExecutionExists(id string) bool {
	return w.executions.Exists(execution.ID(id))
}

func (w *AWSFunctionWorkload) ExecuteInteractively(id string, _ io.ReadWriter, stdout, stderr io.Writer) (
	<-chan ExitInfo, context.CancelFunc, error) {
	w.ResetTimeout()
	request, ok := w.executions.Pop(execution.ID(id))
	if !ok {
		return nil, nil, ErrorUnknownExecution
	}
	command, ctx, cancel := prepareExecution(request)
	exitInternal := make(chan ExitInfo)
	exit := make(chan ExitInfo, 1)

	go w.executeCommand(ctx, command, stdout, stderr, exitInternal)
	go w.handleRunnerTimeout(ctx, exitInternal, exit, execution.ID(id))

	return exit, cancel, nil
}

// UpdateFileSystem copies Files into the executor.
func (w *AWSFunctionWorkload) UpdateFileSystem(request *dto.UpdateFileSystemRequest) error {
	for _, path := range request.Delete {
		delete(w.fs, path)
	}
	for _, file := range request.Copy {
		w.fs[file.Path] = file.Content
	}
	return nil
}

func (w *AWSFunctionWorkload) Destroy() error {
	for _, cancel := range w.runningExecutions {
		cancel()
	}
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

	exitCode, err := w.receiveOutput(wsConn, stdout, stderr, ctx)
	if w.TimeoutPassed() {
		err = ErrorRunnerInactivityTimeout
	}
	exit <- ExitInfo{exitCode, err}
}

func (w *AWSFunctionWorkload) receiveOutput(
	conn *websocket.Conn, stdout, stderr io.Writer, ctx context.Context) (uint8, error) {
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
			log.WithField("data", wsMessage).Warn("unexpected message from aws function")
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
	exitInternal <-chan ExitInfo, exit chan<- ExitInfo, id execution.ID) {
	executionCtx, cancelExecution := context.WithCancel(ctx)
	w.runningExecutions[id] = cancelExecution
	defer delete(w.runningExecutions, id)
	defer close(exit)

	select {
	case exitInfo := <-exitInternal:
		exit <- exitInfo
	case <-executionCtx.Done():
		exit <- ExitInfo{255, ErrorRunnerInactivityTimeout}
	}
}
