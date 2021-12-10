package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/gorilla/websocket"
	"github.com/openHPI/poseidon/internal/api"
	"github.com/openHPI/poseidon/internal/nomad"
	"github.com/openHPI/poseidon/pkg/dto"
	"github.com/openHPI/poseidon/tests"
	"github.com/openHPI/poseidon/tests/helpers"
	"github.com/stretchr/testify/suite"
	"net/http"
	"strings"
	"time"
)

func (s *E2ETestSuite) TestExecuteCommandRoute() {
	runnerID, err := ProvideRunner(&dto.RunnerRequest{
		ExecutionEnvironmentID: tests.DefaultEnvironmentIDAsInteger,
	})
	s.Require().NoError(err)

	webSocketURL, err := ProvideWebSocketURL(&s.Suite, runnerID, &dto.ExecutionRequest{Command: "true"})
	s.Require().NoError(err)
	s.NotEqual("", webSocketURL)

	var connection *websocket.Conn
	var connectionClosed bool

	connection, err = ConnectToWebSocket(webSocketURL)
	s.Require().NoError(err, "websocket connects")
	closeHandler := connection.CloseHandler()
	connection.SetCloseHandler(func(code int, text string) error {
		connectionClosed = true
		return closeHandler(code, text)
	})

	startMessage, err := helpers.ReceiveNextWebSocketMessage(connection)
	s.Require().NoError(err)
	s.Equal(&dto.WebSocketMessage{Type: dto.WebSocketMetaStart}, startMessage)

	exitMessage, err := helpers.ReceiveNextWebSocketMessage(connection)
	s.Require().NoError(err)
	s.Equal(&dto.WebSocketMessage{Type: dto.WebSocketExit}, exitMessage)

	_, err = helpers.ReceiveAllWebSocketMessages(connection)
	s.Require().Error(err)
	s.Equal(&websocket.CloseError{Code: websocket.CloseNormalClosure}, err)

	_, _, err = connection.ReadMessage()
	s.True(websocket.IsCloseError(err, websocket.CloseNormalClosure))
	s.True(connectionClosed, "connection should be closed")
}

func (s *E2ETestSuite) TestOutputToStdout() {
	connection, err := ProvideWebSocketConnection(&s.Suite, &dto.ExecutionRequest{Command: "echo Hello World"})
	s.Require().NoError(err)

	messages, err := helpers.ReceiveAllWebSocketMessages(connection)
	s.Require().Error(err)
	s.Equal(&websocket.CloseError{Code: websocket.CloseNormalClosure}, err)

	controlMessages := helpers.WebSocketControlMessages(messages)
	s.Require().Equal(&dto.WebSocketMessage{Type: dto.WebSocketMetaStart}, controlMessages[0])
	s.Require().Equal(&dto.WebSocketMessage{Type: dto.WebSocketExit}, controlMessages[1])

	stdout, _, _ := helpers.WebSocketOutputMessages(messages)
	s.Require().Equal("Hello World\r\n", stdout)
}

func (s *E2ETestSuite) TestOutputToStderr() {
	connection, err := ProvideWebSocketConnection(&s.Suite, &dto.ExecutionRequest{Command: "cat -invalid"})
	s.Require().NoError(err)

	messages, err := helpers.ReceiveAllWebSocketMessages(connection)
	s.Require().Error(err)
	s.Equal(&websocket.CloseError{Code: websocket.CloseNormalClosure}, err)

	controlMessages := helpers.WebSocketControlMessages(messages)
	s.Require().Equal(2, len(controlMessages))
	s.Require().Equal(&dto.WebSocketMessage{Type: dto.WebSocketMetaStart}, controlMessages[0])
	s.Require().Equal(&dto.WebSocketMessage{Type: dto.WebSocketExit, ExitCode: 1}, controlMessages[1])

	stdout, stderr, errors := helpers.WebSocketOutputMessages(messages)
	s.NotContains(stdout, "cat: invalid option", "Stdout should not contain the error")
	s.Contains(stderr, "cat: invalid option", "Stderr should contain the error")
	s.Empty(errors)
}

func (s *E2ETestSuite) TestCommandHead() {
	hello := "Hello World!"
	connection, err := ProvideWebSocketConnection(&s.Suite, &dto.ExecutionRequest{Command: "head -n 1"})
	s.Require().NoError(err)

	startMessage, err := helpers.ReceiveNextWebSocketMessage(connection)
	s.Require().NoError(err)
	s.Equal(dto.WebSocketMetaStart, startMessage.Type)

	err = connection.WriteMessage(websocket.TextMessage, []byte(fmt.Sprintf("%s\n", hello)))
	s.Require().NoError(err)

	messages, err := helpers.ReceiveAllWebSocketMessages(connection)
	s.Require().Error(err)
	s.Equal(err, &websocket.CloseError{Code: websocket.CloseNormalClosure})
	stdout, _, _ := helpers.WebSocketOutputMessages(messages)
	s.Contains(stdout, fmt.Sprintf("%s\r\n%s\r\n", hello, hello))
}

func (s *E2ETestSuite) TestCommandReturnsAfterTimeout() {
	connection, err := ProvideWebSocketConnection(&s.Suite, &dto.ExecutionRequest{Command: "sleep 4", TimeLimit: 1})
	s.Require().NoError(err)

	c := make(chan bool)
	var messages []*dto.WebSocketMessage
	go func() {
		messages, err = helpers.ReceiveAllWebSocketMessages(connection)
		if !s.Equal(&websocket.CloseError{Code: websocket.CloseNormalClosure}, err) {
			s.T().Fail()
		}
		close(c)
	}()

	select {
	case <-time.After(2 * time.Second):
		s.T().Fatal("The execution should have returned by now")
	case <-c:
		if s.Equal(&dto.WebSocketMessage{Type: dto.WebSocketMetaTimeout}, messages[len(messages)-1]) {
			return
		}
	}
	s.T().Fail()
}

func (s *E2ETestSuite) TestEchoEnvironment() {
	connection, err := ProvideWebSocketConnection(&s.Suite, &dto.ExecutionRequest{
		Command:     "echo $hello",
		Environment: map[string]string{"hello": "world"},
	})
	s.Require().NoError(err)

	startMessage, err := helpers.ReceiveNextWebSocketMessage(connection)
	s.Require().NoError(err)
	s.Equal(dto.WebSocketMetaStart, startMessage.Type)

	messages, err := helpers.ReceiveAllWebSocketMessages(connection)
	s.Require().Error(err)
	s.Equal(err, &websocket.CloseError{Code: websocket.CloseNormalClosure})
	stdout, _, _ := helpers.WebSocketOutputMessages(messages)
	s.Equal("world\r\n", stdout)
}

func (s *E2ETestSuite) TestStderrFifoIsRemoved() {
	runnerID, err := ProvideRunner(&dto.RunnerRequest{
		ExecutionEnvironmentID: tests.DefaultEnvironmentIDAsInteger,
	})
	s.Require().NoError(err)

	webSocketURL, err := ProvideWebSocketURL(&s.Suite, runnerID, &dto.ExecutionRequest{Command: "ls -a /tmp/"})
	s.Require().NoError(err)
	connection, err := ConnectToWebSocket(webSocketURL)
	s.Require().NoError(err)

	messages, err := helpers.ReceiveAllWebSocketMessages(connection)
	s.Require().Error(err)
	s.Equal(&websocket.CloseError{Code: websocket.CloseNormalClosure}, err)

	stdout, _, _ := helpers.WebSocketOutputMessages(messages)
	s.Contains(stdout, ".fifo", "there should be a .fifo file during the execution")

	s.NotContains(s.ListTempDirectory(runnerID), ".fifo", "/tmp/ should not contain any .fifo files after the execution")
}

func (s *E2ETestSuite) ListTempDirectory(runnerID string) string {
	allocListStub, _, err := nomadClient.Jobs().Allocations(runnerID, true, nil)
	s.Require().NoError(err)
	s.Require().Equal(1, len(allocListStub))
	alloc, _, err := nomadClient.Allocations().Info(allocListStub[0].ID, nil)
	s.Require().NoError(err)

	var stdout, stderr bytes.Buffer
	exit, err := nomadClient.Allocations().Exec(context.Background(), alloc, nomad.TaskName,
		false, []string{"ls", "-a", "/tmp/"}, strings.NewReader(""), &stdout, &stderr, nil, nil)

	s.Require().NoError(err)
	s.Require().Equal(0, exit)
	s.Require().Empty(stderr)
	return stdout.String()
}

// ProvideWebSocketConnection establishes a client WebSocket connection to run the passed ExecutionRequest.
// It requires a running Poseidon instance.
func ProvideWebSocketConnection(s *suite.Suite, request *dto.ExecutionRequest) (*websocket.Conn, error) {
	runnerID, err := ProvideRunner(&dto.RunnerRequest{
		ExecutionEnvironmentID: tests.DefaultEnvironmentIDAsInteger,
	})
	if err != nil {
		return nil, fmt.Errorf("error providing runner: %w", err)
	}
	webSocketURL, err := ProvideWebSocketURL(s, runnerID, request)
	if err != nil {
		return nil, fmt.Errorf("error providing WebSocket URL: %w", err)
	}
	connection, err := ConnectToWebSocket(webSocketURL)
	if err != nil {
		return nil, fmt.Errorf("error connecting to WebSocket: %w", err)
	}
	return connection, nil
}

// ProvideWebSocketURL creates a WebSocket endpoint from the ExecutionRequest via an external api request.
// It requires a running Poseidon instance.
func ProvideWebSocketURL(s *suite.Suite, runnerID string, request *dto.ExecutionRequest) (string, error) {
	url := helpers.BuildURL(api.BasePath, api.RunnersPath, runnerID, api.ExecutePath)
	executionRequestByteString, err := json.Marshal(request)
	s.Require().NoError(err)
	reader := strings.NewReader(string(executionRequestByteString))
	resp, err := http.Post(url, "application/json", reader) //nolint:gosec // url is not influenced by a user
	s.Require().NoError(err)
	s.Require().Equal(http.StatusOK, resp.StatusCode)

	executionResponse := new(dto.ExecutionResponse)
	err = json.NewDecoder(resp.Body).Decode(executionResponse)
	s.Require().NoError(err)
	return executionResponse.WebSocketURL, nil
}

// ConnectToWebSocket establish an external WebSocket connection to the provided url.
// It requires a running Poseidon instance.
func ConnectToWebSocket(url string) (conn *websocket.Conn, err error) {
	conn, _, err = websocket.DefaultDialer.Dial(url, nil)
	return
}
