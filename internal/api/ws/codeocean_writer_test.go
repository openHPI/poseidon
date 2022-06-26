package ws

import (
	"context"
	"encoding/json"
	"github.com/gorilla/websocket"
	"github.com/openHPI/poseidon/internal/runner"
	"github.com/openHPI/poseidon/pkg/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestRawToCodeOceanWriter(t *testing.T) {
	connectionMock, message := buildConnectionMock(t)
	proxyCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	output := NewCodeOceanOutputWriter(connectionMock, proxyCtx)
	<-message // start message

	t.Run("StdOut", func(t *testing.T) {
		testMessage := "testStdOut"
		_, err := output.StdOut().Write([]byte(testMessage))
		require.NoError(t, err)

		expected, err := json.Marshal(struct {
			Type string `json:"type"`
			Data string `json:"data"`
		}{string(dto.WebSocketOutputStdout), testMessage})
		require.NoError(t, err)

		assert.Equal(t, expected, <-message)
	})

	t.Run("StdErr", func(t *testing.T) {
		testMessage := "testStdErr"
		_, err := output.StdErr().Write([]byte(testMessage))
		require.NoError(t, err)

		expected, err := json.Marshal(struct {
			Type string `json:"type"`
			Data string `json:"data"`
		}{string(dto.WebSocketOutputStderr), testMessage})
		require.NoError(t, err)

		assert.Equal(t, expected, <-message)
	})
}

type sendExitInfoTestCase struct {
	name    string
	info    *runner.ExitInfo
	message dto.WebSocketMessage
}

func TestCodeOceanOutputWriter_SendExitInfo(t *testing.T) {
	testCases := []sendExitInfoTestCase{
		{"Timeout", &runner.ExitInfo{Err: runner.ErrorRunnerInactivityTimeout},
			dto.WebSocketMessage{Type: dto.WebSocketMetaTimeout}},
		{"Error", &runner.ExitInfo{Err: websocket.ErrCloseSent},
			dto.WebSocketMessage{Type: dto.WebSocketOutputError, Data: "Error executing the request"}},
		{"Exit", &runner.ExitInfo{Code: 21},
			dto.WebSocketMessage{Type: dto.WebSocketExit, ExitCode: 21}},
	}

	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) {
			connectionMock, message := buildConnectionMock(t)
			proxyCtx, cancel := context.WithCancel(context.Background())
			defer cancel()
			output := NewCodeOceanOutputWriter(connectionMock, proxyCtx)
			<-message // start message

			output.SendExitInfo(test.info)
			expected, err := json.Marshal(test.message)
			require.NoError(t, err)

			msg := <-message
			assert.Equal(t, expected, msg)
		})
	}
}

func buildConnectionMock(t *testing.T) (conn *ConnectionMock, messages chan []byte) {
	t.Helper()
	message := make(chan []byte)
	connectionMock := &ConnectionMock{}
	connectionMock.On("WriteMessage", mock.AnythingOfType("int"), mock.AnythingOfType("[]uint8")).
		Run(func(args mock.Arguments) {
			m, ok := args.Get(1).([]byte)
			require.True(t, ok)
			message <- m
		}).
		Return(nil)
	connectionMock.On("CloseHandler").Return(nil)
	connectionMock.On("SetCloseHandler", mock.Anything).Return()
	connectionMock.On("Close").Return()
	return connectionMock, message
}
