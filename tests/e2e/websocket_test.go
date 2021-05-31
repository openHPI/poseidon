package e2e

import (
	"encoding/json"
	"fmt"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/suite"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/api"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/api/dto"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/tests/helpers"
	"net/http"
	"strings"
	"time"
)

func (s *E2ETestSuite) TestExecuteCommandRoute() {
	runnerId, err := ProvideRunner(&dto.RunnerRequest{})
	s.Require().NoError(err)

	webSocketURL, err := ProvideWebSocketURL(&s.Suite, runnerId, &dto.ExecutionRequest{Command: "true"})
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

	_, _, _ = connection.ReadMessage()
	s.True(connectionClosed, "connection should be closed")
}

func (s *E2ETestSuite) TestOutputToStdout() {
	connection, err := ProvideWebSocketConnection(&s.Suite, &dto.ExecutionRequest{Command: "echo Hello World"})
	s.Require().NoError(err)

	messages, err := helpers.ReceiveAllWebSocketMessages(connection)
	s.Require().Error(err)
	s.Equal(&websocket.CloseError{Code: websocket.CloseNormalClosure}, err)

	s.Require().Equal(&dto.WebSocketMessage{Type: dto.WebSocketMetaStart}, messages[0])
	s.Require().Equal(&dto.WebSocketMessage{Type: dto.WebSocketOutputStdout, Data: "Hello World\r\n"}, messages[1])
	s.Require().Equal(&dto.WebSocketMessage{Type: dto.WebSocketExit}, messages[2])
}

func (s *E2ETestSuite) TestOutputToStderr() {
	s.T().Skip("known bug causing all output to be written to stdout (even if it should be written to stderr)")
	connection, err := ProvideWebSocketConnection(&s.Suite, &dto.ExecutionRequest{Command: "cat -invalid"})
	s.Require().NoError(err)

	messages, err := helpers.ReceiveAllWebSocketMessages(connection)
	s.Require().Error(err)
	s.Equal(&websocket.CloseError{Code: websocket.CloseNormalClosure}, err)

	controlMessages := helpers.WebSocketControlMessages(messages)
	s.Require().Equal(&dto.WebSocketMessage{Type: dto.WebSocketMetaStart}, controlMessages[0])
	s.Require().Equal(&dto.WebSocketMessage{Type: dto.WebSocketExit}, controlMessages[1])

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

// ProvideWebSocketConnection establishes a client WebSocket connection to run the passed ExecutionRequest.
// It requires a running Poseidon instance.
func ProvideWebSocketConnection(suite *suite.Suite, request *dto.ExecutionRequest) (connection *websocket.Conn, err error) {
	runnerId, err := ProvideRunner(&dto.RunnerRequest{})
	if err != nil {
		return
	}
	webSocketURL, err := ProvideWebSocketURL(suite, runnerId, request)
	if err != nil {
		return
	}
	connection, err = ConnectToWebSocket(webSocketURL)
	return
}

// ProvideWebSocketURL creates a WebSocket endpoint from the ExecutionRequest via an external api request.
// It requires a running Poseidon instance.
func ProvideWebSocketURL(suite *suite.Suite, runnerId string, request *dto.ExecutionRequest) (string, error) {
	url := helpers.BuildURL(api.BasePath, api.RunnersPath, runnerId, api.ExecutePath)
	executionRequestBytes, _ := json.Marshal(request)
	reader := strings.NewReader(string(executionRequestBytes))
	resp, err := http.Post(url, "application/json", reader)
	suite.Require().NoError(err)
	suite.Require().Equal(http.StatusOK, resp.StatusCode)

	executionResponse := new(dto.ExecutionResponse)
	err = json.NewDecoder(resp.Body).Decode(executionResponse)
	suite.Require().NoError(err)
	return executionResponse.WebSocketUrl, nil
}

// ConnectToWebSocket establish an external WebSocket connection to the provided url.
// It requires a running Poseidon instance.
func ConnectToWebSocket(url string) (conn *websocket.Conn, err error) {
	conn, _, err = websocket.DefaultDialer.Dial(url, nil)
	return
}
