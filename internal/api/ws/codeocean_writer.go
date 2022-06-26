package ws

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/gorilla/websocket"
	"github.com/openHPI/poseidon/internal/runner"
	"github.com/openHPI/poseidon/pkg/dto"
	"io"
)

// CodeOceanOutputWriterBufferSize defines the number of messages.
const CodeOceanOutputWriterBufferSize = 64

// rawToCodeOceanWriter is a simple io.Writer implementation that just forwards the call to sendMessage.
type rawToCodeOceanWriter struct {
	sendMessage func(string)
}

// Write implements the io.Writer interface.
func (rc *rawToCodeOceanWriter) Write(p []byte) (int, error) {
	rc.sendMessage(string(p))
	return len(p), nil
}

// WebSocketWriter is an interface that defines which data is required and which information can be passed.
type WebSocketWriter interface {
	StdOut() io.Writer
	StdErr() io.Writer
	SendExitInfo(info *runner.ExitInfo)
}

// codeOceanOutputWriter is a concrete WebSocketWriter implementation.
// It forwards the data written to stdOut or stdErr (Nomad, AWS) to the WebSocket connection (CodeOcean).
type codeOceanOutputWriter struct {
	connection Connection
	stdOut     io.Writer
	stdErr     io.Writer
	queue      chan *writingLoopMessage
	stopped    bool
}

// writingLoopMessage is an internal data structure to notify the writing loop when it should stop.
type writingLoopMessage struct {
	done bool
	data *dto.WebSocketMessage
}

// NewCodeOceanOutputWriter provies an codeOceanOutputWriter for the time the context ctx is active.
// The codeOceanOutputWriter handles all the messages defined in the websocket.schema.json (start, timeout, stdout, ..).
func NewCodeOceanOutputWriter(connection Connection, ctx context.Context) *codeOceanOutputWriter {
	cw := &codeOceanOutputWriter{
		connection: connection,
		queue:      make(chan *writingLoopMessage, CodeOceanOutputWriterBufferSize),
		stopped:    false,
	}
	cw.stdOut = &rawToCodeOceanWriter{sendMessage: func(s string) {
		cw.send(&dto.WebSocketMessage{Type: dto.WebSocketOutputStdout, Data: s})
	}}
	cw.stdErr = &rawToCodeOceanWriter{sendMessage: func(s string) {
		cw.send(&dto.WebSocketMessage{Type: dto.WebSocketOutputStderr, Data: s})
	}}

	go cw.startWritingLoop()
	go cw.stopWhenContextDone(ctx)
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

// SendExitInfo forwards the kind of exit (timeout, error, normal) to CodeOcean.
func (cw *codeOceanOutputWriter) SendExitInfo(info *runner.ExitInfo) {
	switch {
	case errors.Is(info.Err, context.DeadlineExceeded) || errors.Is(info.Err, runner.ErrorRunnerInactivityTimeout):
		cw.send(&dto.WebSocketMessage{Type: dto.WebSocketMetaTimeout})
	case info.Err != nil:
		errorMessage := "Error executing the request"
		log.WithError(info.Err).Warn(errorMessage)
		cw.send(&dto.WebSocketMessage{Type: dto.WebSocketOutputError, Data: errorMessage})
	default:
		cw.send(&dto.WebSocketMessage{Type: dto.WebSocketExit, ExitCode: info.Code})
	}
}

// stopWhenContextDone notifies the writing loop to stop after the context has been passed.
func (cw *codeOceanOutputWriter) stopWhenContextDone(ctx context.Context) {
	<-ctx.Done()
	if !cw.stopped {
		cw.queue <- &writingLoopMessage{done: true}
	}
}

// send forwards the passed dto.WebSocketMessage to the writing loop.
func (cw *codeOceanOutputWriter) send(message *dto.WebSocketMessage) {
	if cw.stopped {
		return
	}
	done := message.Type == dto.WebSocketExit ||
		message.Type == dto.WebSocketMetaTimeout ||
		message.Type == dto.WebSocketOutputError
	cw.queue <- &writingLoopMessage{done: done, data: message}
}

// startWritingLoop enables the writing loop.
// This is the central and only place where written changes to the WebSocket connection should be done.
// It synchronizes the messages to provide state checks of the WebSocket connection.
func (cw *codeOceanOutputWriter) startWritingLoop() {
	for {
		message := <-cw.queue
		done := sendMessage(cw.connection, message.data)
		if done || message.done {
			break
		}
	}
	cw.stopped = true
	message := websocket.FormatCloseMessage(websocket.CloseNormalClosure, "")
	err := cw.connection.WriteMessage(websocket.CloseMessage, message)
	err2 := cw.connection.Close()
	if err != nil || err2 != nil {
		log.WithError(err).WithField("err2", err2).Warn("Error during websocket close")
	}
}

// sendMessage is a helper function for the writing loop. It must not be called from somewhere else!
func sendMessage(connection Connection, message *dto.WebSocketMessage) (done bool) {
	if message == nil {
		return false
	}

	encodedMessage, err := json.Marshal(message)
	if err != nil {
		log.WithField("message", message).WithError(err).Warn("Marshal error")
		return false
	}

	log.WithField("message", message).Trace("Sending message to client")
	err = connection.WriteMessage(websocket.TextMessage, encodedMessage)
	if err != nil {
		errorMessage := "Error writing the message"
		log.WithField("message", message).WithError(err).Warn(errorMessage)
		return true
	}

	return false
}
