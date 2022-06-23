package ws

import (
	"context"
	"github.com/gorilla/websocket"
	"github.com/openHPI/poseidon/pkg/logging"
	"io"
)

const CodeOceanToRawReaderBufferSize = 1024

var log = logging.GetLogger("ws")

// WebSocketReader is an interface that is intended for providing abstraction around reading from a WebSocket.
// Besides, io.Reader, it also implements io.Writer. The Write method is used to inject data into the WebSocket stream.
type WebSocketReader interface {
	io.Reader
	io.Writer
	Start()
	Stop()
}

// codeOceanToRawReader is an io.Reader implementation that provides the content of the WebSocket connection
// to CodeOcean. You have to start the Reader by calling readInputLoop. After that you can use the Read function.
type codeOceanToRawReader struct {
	connection Connection

	// readCtx is the context in that messages from CodeOcean are read.
	readCtx       context.Context
	cancelReadCtx context.CancelFunc
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

func NewCodeOceanToRawReader(connection Connection, wsCtx, executorCtx context.Context) *codeOceanToRawReader {
	return &codeOceanToRawReader{
		connection:     connection,
		readCtx:        wsCtx, // This context may be canceled before the executorCtx.
		cancelReadCtx:  func() {},
		executorCtx:    executorCtx,
		buffer:         make(chan byte, CodeOceanToRawReaderBufferSize),
		priorityBuffer: make(chan byte, CodeOceanToRawReaderBufferSize),
	}
}

// readInputLoop reads from the WebSocket connection and buffers the user's input.
// This is necessary because input must be read for the connection to handle special messages like close and call the
// CloseHandler.
func (cr *codeOceanToRawReader) readInputLoop(ctx context.Context) {
	readMessage := make(chan bool)
	loopContext, cancelInputLoop := context.WithCancel(ctx)
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

	log.WithField("message", string(message)).Trace("Received message from client")
	for _, character := range message {
		select {
		case <-ctx.Done():
			return true
		case buffer <- character:
		}
	}
	return false
}

// Start starts the read input loop asynchronously.
func (cr *codeOceanToRawReader) Start() {
	ctx, cancel := context.WithCancel(cr.readCtx)
	cr.cancelReadCtx = cancel
	go cr.readInputLoop(ctx)
}

// Stop stops the asynchronous read input loop.
func (cr *codeOceanToRawReader) Stop() {
	cr.cancelReadCtx()
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
