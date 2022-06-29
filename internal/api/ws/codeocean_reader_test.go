package ws

import (
	"context"
	"github.com/gorilla/websocket"
	"github.com/openHPI/poseidon/tests"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"io"
	"strings"
	"testing"
)

func TestCodeOceanToRawReaderReturnsOnlyAfterOneByteWasRead(t *testing.T) {
	readingCtx, cancel := context.WithCancel(context.Background())
	forwardingCtx := readingCtx
	defer cancel()
	reader := NewCodeOceanToRawReader(nil, readingCtx, forwardingCtx)

	read := make(chan bool)
	go func() {
		//nolint:makezero // we can't make zero initial length here as the reader otherwise doesn't block
		p := make([]byte, 10)
		_, err := reader.Read(p)
		require.NoError(t, err)
		read <- true
	}()

	t.Run("Does not return immediately when there is no data", func(t *testing.T) {
		assert.False(t, tests.ChannelReceivesSomething(read, tests.ShortTimeout))
	})

	t.Run("Returns when there is data available", func(t *testing.T) {
		reader.buffer <- byte(42)
		assert.True(t, tests.ChannelReceivesSomething(read, tests.ShortTimeout))
	})
}

func TestCodeOceanToRawReaderReturnsOnlyAfterOneByteWasReadFromConnection(t *testing.T) {
	messages := make(chan io.Reader)

	connection := &ConnectionMock{}
	connection.On("WriteMessage", mock.AnythingOfType("int"), mock.AnythingOfType("[]uint8")).Return(nil)
	connection.On("CloseHandler").Return(nil)
	connection.On("SetCloseHandler", mock.Anything).Return()
	call := connection.On("NextReader")
	call.Run(func(_ mock.Arguments) {
		call.Return(websocket.TextMessage, <-messages, nil)
	})

	readingCtx, cancel := context.WithCancel(context.Background())
	forwardingCtx := readingCtx
	defer cancel()
	reader := NewCodeOceanToRawReader(connection, readingCtx, forwardingCtx)
	reader.Start()

	read := make(chan bool)
	//nolint:makezero // this is required here to make the Read call blocking
	message := make([]byte, 10)
	go func() {
		_, err := reader.Read(message)
		require.NoError(t, err)
		read <- true
	}()

	t.Run("Does not return immediately when there is no data", func(t *testing.T) {
		assert.False(t, tests.ChannelReceivesSomething(read, tests.ShortTimeout))
	})

	t.Run("Returns when there is data available", func(t *testing.T) {
		messages <- strings.NewReader("Hello")
		assert.True(t, tests.ChannelReceivesSomething(read, tests.ShortTimeout))
	})
}
