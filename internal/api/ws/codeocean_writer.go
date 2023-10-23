package ws

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/gorilla/websocket"
	"github.com/openHPI/poseidon/internal/nomad"
	"github.com/openHPI/poseidon/internal/runner"
	"github.com/openHPI/poseidon/pkg/dto"
	"io"
)

// CodeOceanOutputWriterBufferSize defines the number of messages.
const CodeOceanOutputWriterBufferSize = 64

// rawToCodeOceanWriter is a simple io.Writer implementation that just forwards the call to sendMessage.
type rawToCodeOceanWriter struct {
	outputType  dto.WebSocketMessageType
	sendMessage func(*dto.WebSocketMessage)
	ctx         context.Context
}

// Write implements the io.Writer interface.
func (rc *rawToCodeOceanWriter) Write(p []byte) (int, error) {
	switch {
	case rc.ctx.Err() != nil:
		return 0, fmt.Errorf("CodeOceanWriter context done: %w", rc.ctx.Err())
	case len(p) == 0:
		return 0, nil
	default:
		rc.sendMessage(&dto.WebSocketMessage{Type: rc.outputType, Data: string(p)})
		return len(p), nil
	}
}

// WebSocketWriter is an interface that defines which data is required and which information can be passed.
type WebSocketWriter interface {
	StdOut() io.Writer
	StdErr() io.Writer
	Close(info *runner.ExitInfo)
}

// codeOceanOutputWriter is a concrete WebSocketWriter implementation.
// It forwards the data written to stdOut or stdErr (Nomad, AWS) to the WebSocket connection (CodeOcean).
type codeOceanOutputWriter struct {
	connection Connection
	stdOut     *rawToCodeOceanWriter
	stdErr     *rawToCodeOceanWriter
	queue      chan *writingLoopMessage
	ctx        context.Context
}

// writingLoopMessage is an internal data structure to notify the writing loop when it should stop.
type writingLoopMessage struct {
	done bool
	data *dto.WebSocketMessage
}

// NewCodeOceanOutputWriter provides an codeOceanOutputWriter for the time the context ctx is active.
// The codeOceanOutputWriter handles all the messages defined in the websocket.schema.json (start, timeout, stdout, ..).
func NewCodeOceanOutputWriter(
	connection Connection, ctx context.Context, done context.CancelFunc) WebSocketWriter {
	cw := &codeOceanOutputWriter{
		connection: connection,
		queue:      make(chan *writingLoopMessage, CodeOceanOutputWriterBufferSize),
		ctx:        ctx,
	}
	cw.stdOut = &rawToCodeOceanWriter{
		outputType:  dto.WebSocketOutputStdout,
		sendMessage: cw.send,
		ctx:         ctx,
	}
	cw.stdErr = &rawToCodeOceanWriter{
		outputType:  dto.WebSocketOutputStderr,
		sendMessage: cw.send,
		ctx:         ctx,
	}

	go cw.startWritingLoop(done)
	cw.send(&dto.WebSocketMessage{Type: dto.WebSocketMetaStart})
	return cw
}

// StdOut provides an io.Writer that forwards the written data to CodeOcean as StdOut stream.
func (cw *codeOceanOutputWriter) StdOut() io.Writer {
	return cw.stdOut
}

// StdErr provides an io.Writer that forwards the written data to CodeOcean as StdErr stream.
func (cw *codeOceanOutputWriter) StdErr() io.Writer {
	return cw.stdErr
}

// Close forwards the kind of exit (timeout, error, normal) to CodeOcean.
// This results in the closing of the WebSocket connection.
// The call of Close is mandantory!
func (cw *codeOceanOutputWriter) Close(info *runner.ExitInfo) {
	defer func() { cw.queue <- &writingLoopMessage{done: true} }()
	// Mask the internal stop reason before disclosing/forwarding it externally/to CodeOcean.
	switch {
	case info == nil:
		return
	case info.Err == nil:
		cw.send(&dto.WebSocketMessage{Type: dto.WebSocketExit, ExitCode: info.Code})
	case errors.Is(info.Err, context.DeadlineExceeded) || errors.Is(info.Err, runner.ErrorRunnerInactivityTimeout):
		cw.send(&dto.WebSocketMessage{Type: dto.WebSocketMetaTimeout})
	case errors.Is(info.Err, runner.ErrOOMKilled):
		cw.send(&dto.WebSocketMessage{Type: dto.WebSocketOutputError, Data: dto.ErrOOMKilled.Error()})
	case errors.Is(info.Err, nomad.ErrorAllocationCompleted), errors.Is(info.Err, runner.ErrDestroyedByAPIRequest):
		message := "the allocation stopped as expected"
		log.WithContext(cw.ctx).WithError(info.Err).Trace(message)
		cw.send(&dto.WebSocketMessage{Type: dto.WebSocketOutputError, Data: message})
	default:
		errorMessage := "Error executing the request"
		log.WithContext(cw.ctx).WithError(info.Err).Warn(errorMessage)
		cw.send(&dto.WebSocketMessage{Type: dto.WebSocketOutputError, Data: errorMessage})
	}
}

// send forwards the passed dto.WebSocketMessage to the writing loop.
func (cw *codeOceanOutputWriter) send(message *dto.WebSocketMessage) {
	cw.queue <- &writingLoopMessage{done: false, data: message}
}

// startWritingLoop enables the writing loop.
// This is the central and only place where written changes to the WebSocket connection should be done.
// It synchronizes the messages to provide state checks of the WebSocket connection.
func (cw *codeOceanOutputWriter) startWritingLoop(writingLoopDone context.CancelFunc) {
	defer func() {
		message := websocket.FormatCloseMessage(websocket.CloseNormalClosure, "")
		err := cw.connection.WriteMessage(websocket.CloseMessage, message)
		err2 := cw.connection.Close()
		if err != nil || err2 != nil {
			log.WithContext(cw.ctx).WithError(err).WithField("err2", err2).Warn("Error during websocket close")
		}
	}()

	for {
		message := <-cw.queue
		done := true
		if message.data != nil {
			done = sendMessage(cw.connection, message.data, cw.ctx)
		}
		if done || message.done {
			log.WithContext(cw.ctx).Trace("Writing loop done")
			writingLoopDone()
			return
		}
	}
}

// sendMessage is a helper function for the writing loop. It must not be called from somewhere else!
func sendMessage(connection Connection, message *dto.WebSocketMessage, ctx context.Context) (done bool) {
	if message == nil {
		return false
	}

	encodedMessage, err := json.Marshal(message)
	if err != nil {
		log.WithContext(ctx).WithField("message", message).WithError(err).Warn("Marshal error")
		return false
	}

	log.WithContext(ctx).WithField("message", message).Trace("Sending message to client")
	err = connection.WriteMessage(websocket.TextMessage, encodedMessage)
	if err != nil {
		errorMessage := "Error writing the message"
		log.WithContext(ctx).WithField("message", message).WithError(err).Warn(errorMessage)
		return true
	}

	return false
}
