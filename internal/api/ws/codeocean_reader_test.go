package ws

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/gorilla/websocket"
	"github.com/openHPI/poseidon/tests"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
)

type MainTestSuite struct {
	tests.MemoryLeakTestSuite
}

func TestMainTestSuite(t *testing.T) {
	suite.Run(t, new(MainTestSuite))
}

func (s *MainTestSuite) TestCodeOceanToRawReaderReturnsOnlyAfterOneByteWasRead() {
	readingCtx, cancel := context.WithCancel(context.Background())
	forwardingCtx := readingCtx
	defer cancel()
	reader := NewCodeOceanToRawReader(nil, readingCtx, forwardingCtx)

	read := make(chan bool)
	go func() {
		//nolint:makezero // we can't make zero initial length here as the reader otherwise doesn't block
		p := make([]byte, 10)
		_, err := reader.Read(p)
		s.Require().NoError(err)
		read <- true
	}()

	s.Run("Does not return immediately when there is no data", func() {
		s.False(tests.ChannelReceivesSomething(read, tests.ShortTimeout))
	})

	s.Run("Returns when there is data available", func() {
		reader.buffer <- byte(42)
		s.True(tests.ChannelReceivesSomething(read, tests.ShortTimeout))
	})
}

func (s *MainTestSuite) TestCodeOceanToRawReaderReturnsOnlyAfterOneByteWasReadFromConnection() {
	messages := make(chan io.Reader)
	defer close(messages)

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
		s.Require().NoError(err)
		read <- true
	}()

	s.Run("Does not return immediately when there is no data", func() {
		s.False(tests.ChannelReceivesSomething(read, tests.ShortTimeout))
	})

	s.Run("Returns when there is data available", func() {
		messages <- strings.NewReader("Hello")
		s.True(tests.ChannelReceivesSomething(read, tests.ShortTimeout))
	})
}
