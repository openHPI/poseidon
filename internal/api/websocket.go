package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/gorilla/websocket"
	"github.com/openHPI/poseidon/internal/runner"
	"github.com/openHPI/poseidon/pkg/dto"
	"io"
	"net/http"
	"sync"
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

// WebSocketReader is an interface that is intended for providing abstraction around reading from a WebSocket.
// Besides io.Reader, it also implements io.Writer. The Write method is used to inject data into the WebSocket stream.
type WebSocketReader interface {
	io.Reader
	io.Writer
	startReadInputLoop()
}

// codeOceanToRawReader is an io.Reader implementation that provides the content of the WebSocket connection
// to CodeOcean. You have to start the Reader by calling readInputLoop. After that you can use the Read function.
type codeOceanToRawReader struct {
	connection webSocketConnection

	// wsCtx is the context in that messages from CodeOcean are read.
	wsCtx context.Context
	// executorCtx is the context in that messages are forwarded to the executor.
	executorCtx context.Context

	// A buffered channel of bytes is used to store data coming from CodeOcean via WebSocket
	// and retrieve it when Read(..) is called. Since channels are thread-safe, we use one here
	// instead of bytes.Buffer.
	buffer chan byte
	// The priorityBuffer is a buffer for injecting data into stdin of the execution from Poseidon,
	// for example the character that causes the tty to generate a SIGQUIT signal.
	// It is always read before the regular buffer.
	priorityBuffer chan byte
}

func newCodeOceanToRawReader(connection webSocketConnection, wsCtx, executorCtx context.Context) *codeOceanToRawReader {
	return &codeOceanToRawReader{
		connection:     connection,
		wsCtx:          wsCtx,
		executorCtx:    executorCtx,
		buffer:         make(chan byte, CodeOceanToRawReaderBufferSize),
		priorityBuffer: make(chan byte, CodeOceanToRawReaderBufferSize),
	}
}

// readInputLoop reads from the WebSocket connection and buffers the user's input.
// This is necessary because input must be read for the connection to handle special messages like close and call the
// CloseHandler.
func (cr *codeOceanToRawReader) readInputLoop() {
	readMessage := make(chan bool)
	loopContext, cancelInputLoop := context.WithCancel(cr.wsCtx)
	defer cancelInputLoop()
	readingContext, cancelNextMessage := context.WithCancel(loopContext)
	defer cancelNextMessage()

	for loopContext.Err() == nil {
		var messageType int
		var reader io.Reader
		var err error

		go func() {
			messageType, reader, err = cr.connection.NextReader()
			select {
			case <-readingContext.Done():
			case readMessage <- true:
			}
		}()
		select {
		case <-loopContext.Done():
			return
		case <-readMessage:
		}

		if handleInput(messageType, reader, err, cr.buffer, loopContext) {
			return
		}
	}
}

// handleInput receives a new message from the client and may forward it to the executor.
func handleInput(messageType int, reader io.Reader, err error, buffer chan byte, ctx context.Context) (done bool) {
	if err != nil && websocket.IsCloseError(err, websocket.CloseNormalClosure) {
		log.Debug("ReadInputLoop: The client closed the connection!")
		// The close handler will do something soon.
		return true
	} else if err != nil {
		log.WithError(err).Warn("Error reading client message")
		return true
	}
	if messageType != websocket.TextMessage {
		log.WithField("messageType", messageType).Warn("Received message of wrong type")
		return true
	}

	message, err := io.ReadAll(reader)
	if err != nil {
		log.WithError(err).Warn("error while reading WebSocket message")
		return true
	}
	for _, character := range message {
		select {
		case <-ctx.Done():
			return true
		case buffer <- character:
		}
	}
	return false
}

// startReadInputLoop start the read input loop asynchronously.
func (cr *codeOceanToRawReader) startReadInputLoop() {
	go cr.readInputLoop()
}

// Read implements the io.Reader interface.
// It returns bytes from the buffer or priorityBuffer.
func (cr *codeOceanToRawReader) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}

	// Ensure to not return until at least one byte has been read to avoid busy waiting.
	select {
	case <-cr.executorCtx.Done():
		return 0, io.EOF
	case p[0] = <-cr.priorityBuffer:
	case p[0] = <-cr.buffer:
	}
	var n int
	for n = 1; n < len(p); n++ {
		select {
		case p[n] = <-cr.priorityBuffer:
		case p[n] = <-cr.buffer:
		default:
			return n, nil
		}
	}
	return n, nil
}

// Write implements the io.Writer interface.
// Data written to a codeOceanToRawReader using this method is returned by Read before other data from the WebSocket.
func (cr *codeOceanToRawReader) Write(p []byte) (n int, err error) {
	var c byte
	for n, c = range p {
		select {
		case cr.priorityBuffer <- c:
		default:
			break
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
	if rc.proxy.webSocketCtx.Err() != nil {
		return 0, nil
	}
	err := rc.proxy.sendToClient(dto.WebSocketMessage{Type: rc.outputType, Data: string(p)})
	return len(p), err
}

// webSocketProxy is an encapsulation of logic for forwarding between Runners and CodeOcean.
type webSocketProxy struct {
	webSocketCtx    context.Context
	cancelWebSocket context.CancelFunc
	connection      webSocketConnection
	connectionMu    sync.Mutex
	Stdin           WebSocketReader
	Stdout          io.Writer
	Stderr          io.Writer
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

// newWebSocketProxy returns an initiated and started webSocketProxy.
// As this proxy is already started, a start message is send to the client.
func newWebSocketProxy(connection webSocketConnection, proxyCtx context.Context) (*webSocketProxy, error) {
	wsCtx, cancelWsCommunication := context.WithCancel(proxyCtx)
	stdin := newCodeOceanToRawReader(connection, wsCtx, proxyCtx)
	proxy := &webSocketProxy{
		connection:      connection,
		Stdin:           stdin,
		webSocketCtx:    wsCtx,
		cancelWebSocket: cancelWsCommunication,
	}
	proxy.Stdout = &rawToCodeOceanWriter{proxy: proxy, outputType: dto.WebSocketOutputStdout}
	proxy.Stderr = &rawToCodeOceanWriter{proxy: proxy, outputType: dto.WebSocketOutputStderr}

	err := proxy.sendToClient(dto.WebSocketMessage{Type: dto.WebSocketMetaStart})
	if err != nil {
		cancelWsCommunication()
		return nil, err
	}

	closeHandler := connection.CloseHandler()
	connection.SetCloseHandler(func(code int, text string) error {
		//nolint:errcheck // The default close handler always returns nil, so the error can be safely ignored.
		_ = closeHandler(code, text)
		cancelWsCommunication()
		return nil
	})
	return proxy, nil
}

// waitForExit waits for an exit of either the runner (when the command terminates) or the client closing the WebSocket
// and handles WebSocket exit messages.
func (wp *webSocketProxy) waitForExit(exit <-chan runner.ExitInfo, cancelExecution context.CancelFunc) {
	wp.Stdin.startReadInputLoop()

	var exitInfo runner.ExitInfo
	select {
	case <-wp.webSocketCtx.Done():
		log.Info("Client closed the connection")
		cancelExecution()
		<-exit // /internal/runner/runner.go handleExitOrContextDone does not require client connection anymore.
		<-exit // The goroutine closes this channel indicating that it does not use the connection to the executor anymore.
		return
	case exitInfo = <-exit:
		log.Info("Execution returned")
		wp.cancelWebSocket()
	}

	if errors.Is(exitInfo.Err, context.DeadlineExceeded) || errors.Is(exitInfo.Err, runner.ErrorRunnerInactivityTimeout) {
		err := wp.sendToClient(dto.WebSocketMessage{Type: dto.WebSocketMetaTimeout})
		if err == nil {
			wp.closeNormal()
		} else {
			log.WithError(err).Warn("Failed to send timeout message to client")
		}
		return
	} else if exitInfo.Err != nil {
		errorMessage := "Error executing the request"
		log.WithError(exitInfo.Err).Warn(errorMessage)
		err := wp.sendToClient(dto.WebSocketMessage{Type: dto.WebSocketOutputError, Data: errorMessage})
		if err == nil {
			wp.closeNormal()
		} else {
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
		log.WithError(err).Warn("Error sending exit message")
		return
	}
	wp.closeNormal()
}

func (wp *webSocketProxy) sendToClient(message dto.WebSocketMessage) error {
	encodedMessage, err := json.Marshal(message)
	if err != nil {
		log.WithField("message", message).WithError(err).Warn("Marshal error")
		wp.closeWithError("Error creating message")
		return fmt.Errorf("error marshaling WebSocket message: %w", err)
	}
	err = wp.writeMessage(websocket.TextMessage, encodedMessage)
	if err != nil {
		errorMessage := "Error writing the message"
		log.WithField("message", message).WithError(err).Warn(errorMessage)
		wp.closeWithError(errorMessage)
		return fmt.Errorf("error writing WebSocket message: %w", err)
	}
	return nil
}

func (wp *webSocketProxy) closeWithError(message string) {
	wp.close(websocket.FormatCloseMessage(websocket.CloseInternalServerErr, message))
}

func (wp *webSocketProxy) closeNormal() {
	wp.close(websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
}

func (wp *webSocketProxy) close(message []byte) {
	err := wp.writeMessage(websocket.CloseMessage, message)
	_ = wp.connection.Close()
	if err != nil {
		log.WithError(err).Warn("Error during websocket close")
	}
}

func (wp *webSocketProxy) writeMessage(messageType int, data []byte) error {
	wp.connectionMu.Lock()
	defer wp.connectionMu.Unlock()
	return wp.connection.WriteMessage(messageType, data) //nolint:wrapcheck // Wrap the original WriteMessage in a mutex.
}

// connectToRunner is the endpoint for websocket connections.
func (r *RunnerController) connectToRunner(writer http.ResponseWriter, request *http.Request) {
	targetRunner, _ := runner.FromContext(request.Context())
	executionID := request.URL.Query().Get(ExecutionIDKey)
	if !targetRunner.ExecutionExists(executionID) {
		writeNotFound(writer, ErrUnknownExecutionID)
		return
	}

	connection, err := upgradeConnection(writer, request)
	if err != nil {
		writeInternalServerError(writer, err, dto.ErrorUnknown)
		return
	}
	proxyCtx, cancelProxy := context.WithCancel(context.Background())
	defer cancelProxy()
	proxy, err := newWebSocketProxy(connection, proxyCtx)
	if err != nil {
		return
	}

	log.WithField("runnerId", targetRunner.ID()).WithField("executionID", executionID).Info("Running execution")
	exit, cancel, err := targetRunner.ExecuteInteractively(executionID, proxy.Stdin, proxy.Stdout, proxy.Stderr)
	if err != nil {
		proxy.closeWithError(fmt.Sprintf("execution failed with: %v", err))
		return
	}

	proxy.waitForExit(exit, cancel)
}
