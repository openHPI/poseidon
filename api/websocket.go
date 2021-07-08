package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/gorilla/websocket"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/api/dto"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/runner"
	"io"
	"net/http"
)

const CodeOceanToRawReaderBufferSize = 1024

var ErrUnknownExecutionID = errors.New("execution id unknown")

type webSocketConnection interface {
	WriteMessage(messageType int, data []byte) error
	Close() error
	NextReader() (messageType int, r io.Reader, err error)
	CloseHandler() func(code int, text string) error
	SetCloseHandler(handler func(code int, text string) error)
}

type WebSocketReader interface {
	io.Reader
	startReadInputLoop() context.CancelFunc
}

// codeOceanToRawReader is an io.Reader implementation that provides the content of the WebSocket connection
// to CodeOcean. You have to start the Reader by calling readInputLoop. After that you can use the Read function.
type codeOceanToRawReader struct {
	connection webSocketConnection

	// A buffered channel of bytes is used to store data coming from CodeOcean via WebSocket
	// and retrieve it when Read(..) is called. Since channels are thread-safe, we use one here
	// instead of bytes.Buffer.
	buffer chan byte
}

func newCodeOceanToRawReader(connection webSocketConnection) *codeOceanToRawReader {
	return &codeOceanToRawReader{
		connection: connection,
		buffer:     make(chan byte, CodeOceanToRawReaderBufferSize),
	}
}

// readInputLoop reads from the WebSocket connection and buffers the user's input.
// This is necessary because input must be read for the connection to handle special messages like close and call the
// CloseHandler.
func (cr *codeOceanToRawReader) readInputLoop(ctx context.Context) {
	readMessage := make(chan bool)
	for {
		var messageType int
		var reader io.Reader
		var err error

		go func() {
			messageType, reader, err = cr.connection.NextReader()
			readMessage <- true
		}()
		select {
		case <-readMessage:
		case <-ctx.Done():
			return
		}

		if err != nil {
			log.WithError(err).Warn("Error reading client message")
			return
		}
		if messageType != websocket.TextMessage {
			log.WithField("messageType", messageType).Warn("Received message of wrong type")
			return
		}

		message, err := io.ReadAll(reader)
		if err != nil {
			log.WithError(err).Warn("error while reading WebSocket message")
			return
		}
		for _, character := range message {
			select {
			case cr.buffer <- character:
			case <-ctx.Done():
				return
			}
		}
	}
}

// startReadInputLoop start the read input loop asynchronously and returns a context.CancelFunc which can be used
// to cancel the read input loop.
func (cr *codeOceanToRawReader) startReadInputLoop() context.CancelFunc {
	ctx, cancel := context.WithCancel(context.Background())
	go cr.readInputLoop(ctx)
	return cancel
}

// Read implements the io.Reader interface.
// It returns bytes from the buffer.
func (cr *codeOceanToRawReader) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	// Ensure to not return until at least one byte has been read to avoid busy waiting.
	p[0] = <-cr.buffer
	var n int
	for n = 1; n < len(p); n++ {
		select {
		case p[n] = <-cr.buffer:
		default:
			return n, nil
		}
	}
	return n, nil
}

// rawToCodeOceanWriter is an io.Writer implementation that, when written to, wraps the written data in the appropriate
// json structure and sends it to the CodeOcean via WebSocket.
type rawToCodeOceanWriter struct {
	proxy      *webSocketProxy
	outputType dto.WebSocketMessageType
}

// Write implements the io.Writer interface.
// The passed data is forwarded to the WebSocket to CodeOcean.
func (rc *rawToCodeOceanWriter) Write(p []byte) (int, error) {
	err := rc.proxy.sendToClient(dto.WebSocketMessage{Type: rc.outputType, Data: string(p)})
	return len(p), err
}

// webSocketProxy is an encapsulation of logic for forwarding between Runners and CodeOcean.
type webSocketProxy struct {
	userExit   chan bool
	connection webSocketConnection
	Stdin      WebSocketReader
	Stdout     io.Writer
	Stderr     io.Writer
}

// upgradeConnection upgrades a connection to a websocket and returns a webSocketProxy for this connection.
func upgradeConnection(writer http.ResponseWriter, request *http.Request) (webSocketConnection, error) {
	connUpgrader := websocket.Upgrader{}
	connection, err := connUpgrader.Upgrade(writer, request, nil)
	if err != nil {
		log.WithError(err).Warn("Connection upgrade failed")
		return nil, fmt.Errorf("error upgrading the connection: %w", err)
	}
	return connection, nil
}

// newWebSocketProxy returns a initiated and started webSocketProxy.
// As this proxy is already started, a start message is send to the client.
func newWebSocketProxy(connection webSocketConnection) (*webSocketProxy, error) {
	stdin := newCodeOceanToRawReader(connection)
	proxy := &webSocketProxy{
		connection: connection,
		Stdin:      stdin,
		userExit:   make(chan bool),
	}
	proxy.Stdout = &rawToCodeOceanWriter{proxy: proxy, outputType: dto.WebSocketOutputStdout}
	proxy.Stderr = &rawToCodeOceanWriter{proxy: proxy, outputType: dto.WebSocketOutputStderr}

	err := proxy.sendToClient(dto.WebSocketMessage{Type: dto.WebSocketMetaStart})
	if err != nil {
		return nil, err
	}

	closeHandler := connection.CloseHandler()
	connection.SetCloseHandler(func(code int, text string) error {
		//nolint:errcheck // The default close handler always returns nil, so the error can be safely ignored.
		_ = closeHandler(code, text)
		close(proxy.userExit)
		return nil
	})
	return proxy, nil
}

// waitForExit waits for an exit of either the runner (when the command terminates) or the client closing the WebSocket
// and handles WebSocket exit messages.
func (wp *webSocketProxy) waitForExit(exit <-chan runner.ExitInfo, cancelExecution context.CancelFunc) {
	defer wp.close()
	cancelInputLoop := wp.Stdin.startReadInputLoop()
	var exitInfo runner.ExitInfo
	select {
	case exitInfo = <-exit:
		cancelInputLoop()
		log.Info("Execution returned")
	case <-wp.userExit:
		cancelInputLoop()
		cancelExecution()
		log.Info("Client closed the connection")
		return
	}

	if errors.Is(exitInfo.Err, context.DeadlineExceeded) || errors.Is(exitInfo.Err, runner.ErrorRunnerInactivityTimeout) {
		err := wp.sendToClient(dto.WebSocketMessage{Type: dto.WebSocketMetaTimeout})
		if err != nil {
			log.WithError(err).Warn("Failed to send timeout message to client")
		}
		return
	} else if exitInfo.Err != nil {
		errorMessage := "Error executing the request"
		log.WithError(exitInfo.Err).Warn(errorMessage)
		err := wp.sendToClient(dto.WebSocketMessage{Type: dto.WebSocketOutputError, Data: errorMessage})
		if err != nil {
			log.WithError(err).Warn("Failed to send output error message to client")
		}
		return
	}
	log.WithField("exit_code", exitInfo.Code).Debug()

	err := wp.sendToClient(dto.WebSocketMessage{
		Type:     dto.WebSocketExit,
		ExitCode: exitInfo.Code,
	})
	if err != nil {
		return
	}
}

func (wp *webSocketProxy) sendToClient(message dto.WebSocketMessage) error {
	encodedMessage, err := json.Marshal(message)
	if err != nil {
		log.WithField("message", message).WithError(err).Warn("Marshal error")
		wp.closeWithError("Error creating message")
		return fmt.Errorf("error marshaling WebSocket message: %w", err)
	}
	err = wp.connection.WriteMessage(websocket.TextMessage, encodedMessage)
	if err != nil {
		errorMessage := "Error writing the exit message"
		log.WithError(err).Warn(errorMessage)
		wp.closeWithError(errorMessage)
		return fmt.Errorf("error writing WebSocket message: %w", err)
	}
	return nil
}

func (wp *webSocketProxy) closeWithError(message string) {
	err := wp.connection.WriteMessage(websocket.CloseMessage,
		websocket.FormatCloseMessage(websocket.CloseInternalServerErr, message))
	if err != nil {
		log.WithError(err).Warn("Error during websocket close")
	}
}

func (wp *webSocketProxy) close() {
	err := wp.connection.WriteMessage(websocket.CloseMessage,
		websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
	_ = wp.connection.Close()
	if err != nil {
		log.WithError(err).Warn("Error during websocket close")
	}
}

// connectToRunner is the endpoint for websocket connections.
func (r *RunnerController) connectToRunner(writer http.ResponseWriter, request *http.Request) {
	targetRunner, _ := runner.FromContext(request.Context())
	executionID := runner.ExecutionID(request.URL.Query().Get(ExecutionIDKey))
	executionRequest, ok := targetRunner.Pop(executionID)
	if !ok {
		writeNotFound(writer, ErrUnknownExecutionID)
		return
	}

	connection, err := upgradeConnection(writer, request)
	if err != nil {
		writeInternalServerError(writer, err, dto.ErrorUnknown)
		return
	}
	proxy, err := newWebSocketProxy(connection)
	if err != nil {
		return
	}

	log.WithField("runnerId", targetRunner.ID()).WithField("executionID", executionID).Info("Running execution")
	exit, cancel := targetRunner.ExecuteInteractively(executionRequest, proxy.Stdin, proxy.Stdout, proxy.Stderr)

	proxy.waitForExit(exit, cancel)
}
