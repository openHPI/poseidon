package e2e

import (
	"encoding/json"
	"fmt"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/suite"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/api"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/api/dto"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/tests/e2e/helpers"
	"net/http"
	"strings"
	"time"
)

func (suite *E2ETestSuite) TestExecuteCommandRoute() {
	runnerId, err := ProvideRunner(&suite.Suite, &dto.RunnerRequest{})
	suite.Require().NoError(err)

	webSocketURL, err := ProvideWebSocketURL(&suite.Suite, runnerId, &dto.ExecutionRequest{Command: "true"})
	suite.Require().NoError(err)
	suite.NotEqual("", webSocketURL)

	var connection *websocket.Conn
	var connectionClosed bool

	connection, err = ConnectToWebSocket(webSocketURL)
	suite.Require().NoError(err, "websocket connects")
	closeHandler := connection.CloseHandler()
	connection.SetCloseHandler(func(code int, text string) error {
		connectionClosed = true
		return closeHandler(code, text)
	})

	startMessage, err := helpers.ReceiveNextWebSocketMessage(connection)
	suite.Require().NoError(err)
	suite.Equal(&dto.WebSocketMessage{Type: dto.WebSocketMetaStart}, startMessage)

	exitMessage, err := helpers.ReceiveNextWebSocketMessage(connection)
	suite.Require().NoError(err)
	suite.Equal(&dto.WebSocketMessage{Type: dto.WebSocketExit}, exitMessage)

	_, err = helpers.ReceiveAllWebSocketMessages(connection)
	suite.Require().Error(err)
	suite.Equal(&websocket.CloseError{Code: websocket.CloseNormalClosure}, err)

	_, _, _ = connection.ReadMessage()
	suite.True(connectionClosed, "connection should be closed")
}

func (suite *E2ETestSuite) TestOutputToStdout() {
	connection, err := ProvideWebSocketConnection(&suite.Suite, &dto.ExecutionRequest{Command: "echo Hello World"})
	suite.Require().NoError(err)

	messages, err := helpers.ReceiveAllWebSocketMessages(connection)
	suite.Require().Error(err)
	suite.Equal(&websocket.CloseError{Code: websocket.CloseNormalClosure}, err)

	suite.Require().Equal(&dto.WebSocketMessage{Type: dto.WebSocketMetaStart}, messages[0])
	suite.Require().Equal(&dto.WebSocketMessage{Type: dto.WebSocketOutputStdout, Data: "Hello World\r\n"}, messages[1])
	suite.Require().Equal(&dto.WebSocketMessage{Type: dto.WebSocketExit}, messages[2])
}

func (suite *E2ETestSuite) TestOutputToStderr() {
	suite.T().Skip("known bug causing all output to be written to stdout (even if it should be written to stderr)")
	connection, err := ProvideWebSocketConnection(&suite.Suite, &dto.ExecutionRequest{Command: "cat -invalid"})
	suite.Require().NoError(err)

	messages, err := helpers.ReceiveAllWebSocketMessages(connection)
	suite.Require().Error(err)
	suite.Equal(&websocket.CloseError{Code: websocket.CloseNormalClosure}, err)

	controlMessages := helpers.WebSocketControlMessages(messages)
	suite.Require().Equal(&dto.WebSocketMessage{Type: dto.WebSocketMetaStart}, controlMessages[0])
	suite.Require().Equal(&dto.WebSocketMessage{Type: dto.WebSocketExit}, controlMessages[1])

	stdout, stderr, errors := helpers.WebSocketOutputMessages(messages)
	suite.NotContains(stdout, "cat: invalid option", "Stdout should not contain the error")
	suite.Contains(stderr, "cat: invalid option", "Stderr should contain the error")
	suite.Empty(errors)
}

func (suite *E2ETestSuite) TestCommandHead() {
	hello := "Hello World!"
	connection, err := ProvideWebSocketConnection(&suite.Suite, &dto.ExecutionRequest{Command: "head -n 1"})
	suite.Require().NoError(err)

	startMessage, err := helpers.ReceiveNextWebSocketMessage(connection)
	suite.Require().NoError(err)
	suite.Equal(dto.WebSocketMetaStart, startMessage.Type)

	err = connection.WriteMessage(websocket.TextMessage, []byte(fmt.Sprintf("%s\n", hello)))
	suite.Require().NoError(err)

	messages, err := helpers.ReceiveAllWebSocketMessages(connection)
	suite.Require().Error(err)
	suite.Equal(err, &websocket.CloseError{Code: websocket.CloseNormalClosure})
	stdout, _, _ := helpers.WebSocketOutputMessages(messages)
	suite.Contains(stdout, fmt.Sprintf("%s\r\n%s\r\n", hello, hello))
}

func (suite *E2ETestSuite) TestCommandReturnsAfterTimeout() {
	connection, err := ProvideWebSocketConnection(&suite.Suite, &dto.ExecutionRequest{Command: "sleep 4", TimeLimit: 1})
	suite.Require().NoError(err)

	c := make(chan bool)
	var messages []*dto.WebSocketMessage
	go func() {
		messages, err = helpers.ReceiveAllWebSocketMessages(connection)
		if !suite.Equal(&websocket.CloseError{Code: websocket.CloseNormalClosure}, err) {
			suite.T().Fail()
		}
		close(c)
	}()

	select {
	case <-time.After(2 * time.Second):
		suite.T().Fatal("The execution should have returned by now")
	case <-c:
		if suite.Equal(&dto.WebSocketMessage{Type: dto.WebSocketMetaTimeout}, messages[len(messages)-1]) {
			return
		}
	}
	suite.T().Fail()
}

func (suite *E2ETestSuite) TestEchoEnvironment() {
	connection, err := ProvideWebSocketConnection(&suite.Suite, &dto.ExecutionRequest{
		Command:     "echo $hello",
		Environment: map[string]string{"hello": "world"},
	})
	suite.Require().NoError(err)

	startMessage, err := helpers.ReceiveNextWebSocketMessage(connection)
	suite.Require().NoError(err)
	suite.Equal(dto.WebSocketMetaStart, startMessage.Type)

	messages, err := helpers.ReceiveAllWebSocketMessages(connection)
	suite.Require().Error(err)
	suite.Equal(err, &websocket.CloseError{Code: websocket.CloseNormalClosure})
	stdout, _, _ := helpers.WebSocketOutputMessages(messages)
	suite.Equal("world\r\n", stdout)
}

// ProvideWebSocketConnection establishes a client WebSocket connection to run the passed ExecutionRequest.
// It requires a running Poseidon instance.
func ProvideWebSocketConnection(suite *suite.Suite, request *dto.ExecutionRequest) (connection *websocket.Conn, err error) {
	runnerId, err := ProvideRunner(suite, &dto.RunnerRequest{})
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

// ProvideRunner creates a runner with the given RunnerRequest via an external request.
// It needs a running Poseidon instance to work.
func ProvideRunner(suite *suite.Suite, request *dto.RunnerRequest) (string, error) {
	url := helpers.BuildURL(api.RouteBase, api.RouteRunners)
	runnerRequestBytes, _ := json.Marshal(request)
	reader := strings.NewReader(string(runnerRequestBytes))
	resp, err := http.Post(url, "application/json", reader)
	suite.Require().NoError(err)
	suite.Require().Equal(http.StatusOK, resp.StatusCode)

	runnerResponse := new(dto.RunnerResponse)
	err = json.NewDecoder(resp.Body).Decode(runnerResponse)
	suite.Require().NoError(err)

	return runnerResponse.Id, nil
}

// ProvideWebSocketURL creates a WebSocket endpoint from the ExecutionRequest via an external api request.
// It requires a running Poseidon instance.
func ProvideWebSocketURL(suite *suite.Suite, runnerId string, request *dto.ExecutionRequest) (string, error) {
	url := helpers.BuildURL(api.RouteBase, api.RouteRunners, "/", runnerId, api.ExecutePath)
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
