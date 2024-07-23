package ws

import (
	"context"
	"encoding/json"

	"github.com/gorilla/websocket"
	"github.com/openHPI/poseidon/internal/runner"
	"github.com/openHPI/poseidon/pkg/dto"
	"github.com/openHPI/poseidon/tests"
	"github.com/stretchr/testify/mock"
)

func (s *MainTestSuite) TestRawToCodeOceanWriter() {
	connectionMock, messages := buildConnectionMock(&s.MemoryLeakTestSuite)
	proxyCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	output := NewCodeOceanOutputWriter(proxyCtx, connectionMock, cancel)
	defer output.Close(nil)
	<-messages // start messages

	s.Run("StdOut", func() {
		testMessage := "testStdOut"
		_, err := output.StdOut().Write([]byte(testMessage))
		s.Require().NoError(err)

		expected, err := json.Marshal(struct {
			Type string `json:"type"`
			Data string `json:"data"`
		}{string(dto.WebSocketOutputStdout), testMessage})
		s.Require().NoError(err)

		s.Equal(expected, <-messages)
	})

	s.Run("StdErr", func() {
		testMessage := "testStdErr"
		_, err := output.StdErr().Write([]byte(testMessage))
		s.Require().NoError(err)

		expected, err := json.Marshal(struct {
			Type string `json:"type"`
			Data string `json:"data"`
		}{string(dto.WebSocketOutputStderr), testMessage})
		s.Require().NoError(err)

		s.Equal(expected, <-messages)
	})
}

type sendExitInfoTestCase struct {
	name    string
	info    *runner.ExitInfo
	message dto.WebSocketMessage
}

func (s *MainTestSuite) TestCodeOceanOutputWriter_SendExitInfo() {
	testCases := []sendExitInfoTestCase{
		{
			"Timeout", &runner.ExitInfo{Err: runner.ErrRunnerInactivityTimeout},
			dto.WebSocketMessage{Type: dto.WebSocketMetaTimeout},
		},
		{
			"Error", &runner.ExitInfo{Err: websocket.ErrCloseSent},
			dto.WebSocketMessage{Type: dto.WebSocketOutputError, Data: "Error executing the request"},
		},
		// CodeOcean expects this exact string in case of a OOM Killed runner.
		{
			"Specific data for OOM Killed runner", &runner.ExitInfo{Err: runner.ErrOOMKilled},
			dto.WebSocketMessage{Type: dto.WebSocketOutputError, Data: "the allocation was OOM Killed"},
		},
		{
			"Exit", &runner.ExitInfo{Code: 21},
			dto.WebSocketMessage{Type: dto.WebSocketExit, ExitCode: 21},
		},
	}

	for _, test := range testCases {
		s.Run(test.name, func() {
			connectionMock, messages := buildConnectionMock(&s.MemoryLeakTestSuite)
			proxyCtx, cancel := context.WithCancel(context.Background())
			defer cancel()
			output := NewCodeOceanOutputWriter(proxyCtx, connectionMock, cancel)
			<-messages // start messages

			output.Close(test.info)
			expected, err := json.Marshal(test.message)
			s.Require().NoError(err)

			msg := <-messages
			s.Equal(expected, msg)

			<-messages // close message
		})
	}
}

func buildConnectionMock(suite *tests.MemoryLeakTestSuite) (conn *ConnectionMock, messages <-chan []byte) {
	suite.T().Helper()
	message := make(chan []byte)
	connectionMock := &ConnectionMock{}
	connectionMock.On("WriteMessage", mock.AnythingOfType("int"), mock.AnythingOfType("[]uint8")).
		Run(func(args mock.Arguments) {
			m, ok := args.Get(1).([]byte)
			suite.Require().True(ok)
			select {
			case <-suite.TestCtx.Done():
			case message <- m:
			}
		}).
		Return(nil)
	connectionMock.On("CloseHandler").Return(nil)
	connectionMock.On("SetCloseHandler", mock.Anything).Return()
	connectionMock.On("Close").Return(nil)
	return connectionMock, message
}
