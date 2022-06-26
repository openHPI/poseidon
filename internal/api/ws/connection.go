package ws

import (
	"io"
)

// Connection is an internal interface for websocket.Conn in order to mock it for unit tests.
type Connection interface {
	WriteMessage(messageType int, data []byte) error
	Close() error
	NextReader() (messageType int, r io.Reader, err error)
	CloseHandler() func(code int, text string) error
	SetCloseHandler(handler func(code int, text string) error)
}
