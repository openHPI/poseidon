package api

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/api/dto"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/nomad"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/runner"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/tests/e2e/helpers"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"
)

func TestWebSocketTestSuite(t *testing.T) {
	suite.Run(t, new(WebSocketTestSuite))
}

type WebSocketTestSuite struct {
	suite.Suite
	router      *mux.Router
	executionId runner.ExecutionId
	runner      runner.Runner
	apiMock     *nomad.ExecutorApiMock
	server      *httptest.Server
}

func (suite *WebSocketTestSuite) SetupTest() {
	runnerId := "runner-id"
	suite.runner, suite.apiMock = helpers.NewNomadAllocationWithMockedApiClient(runnerId)

	// default execution
	suite.executionId = "execution-id"
	suite.runner.Add(suite.executionId, &executionRequestHead)
	mockApiExecuteHead(suite.apiMock)

	runnerManager := &runner.ManagerMock{}
	runnerManager.On("Get", suite.runner.Id()).Return(suite.runner, nil)
	suite.router = NewRouter(runnerManager, nil)
	suite.server = httptest.NewServer(suite.router)
}

func (suite *WebSocketTestSuite) TearDownTest() {
	suite.server.Close()
}

func (suite *WebSocketTestSuite) TestWebsocketConnectionCanBeEstablished() {
	wsUrl, err := suite.webSocketUrl("ws", suite.runner.Id(), suite.executionId)
	suite.Require().NoError(err)
	_, _, err = websocket.DefaultDialer.Dial(wsUrl.String(), nil)
	suite.Require().NoError(err)
}

func (suite *WebSocketTestSuite) TestWebsocketReturns404IfExecutionDoesNotExist() {
	wsUrl, err := suite.webSocketUrl("ws", suite.runner.Id(), "invalid-execution-id")
	suite.Require().NoError(err)
	_, response, _ := websocket.DefaultDialer.Dial(wsUrl.String(), nil)
	suite.Equal(http.StatusNotFound, response.StatusCode)
}

func (suite *WebSocketTestSuite) TestWebsocketReturns400IfRequestedViaHttp() {
	wsUrl, err := suite.webSocketUrl("http", suite.runner.Id(), suite.executionId)
	suite.Require().NoError(err)
	response, err := http.Get(wsUrl.String())
	suite.Require().NoError(err)
	// This functionality is implemented by the WebSocket library.
	suite.Equal(http.StatusBadRequest, response.StatusCode)
}

func (suite *WebSocketTestSuite) TestWebsocketConnection() {
	wsUrl, err := suite.webSocketUrl("ws", suite.runner.Id(), suite.executionId)
	suite.Require().NoError(err)
	connection, _, err := websocket.DefaultDialer.Dial(wsUrl.String(), nil)
	suite.Require().NoError(err)
	err = connection.SetReadDeadline(time.Now().Add(5 * time.Second))
	suite.Require().NoError(err)

	suite.Run("Receives start message", func() {
		message, err := helpers.ReceiveNextWebSocketMessage(connection)
		suite.Require().NoError(err)
		suite.Equal(dto.WebSocketMetaStart, message.Type)
	})

	suite.Run("Executes the request in the runner", func() {
		<-time.After(100 * time.Millisecond)
		suite.apiMock.AssertCalled(suite.T(), "ExecuteCommand",
			mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	})

	suite.Run("Can send input", func() {
		err = connection.WriteMessage(websocket.TextMessage, []byte("Hello World\n"))
		suite.Require().NoError(err)
	})

	messages, err := helpers.ReceiveAllWebSocketMessages(connection)
	suite.Require().Error(err)
	suite.Equal(&websocket.CloseError{Code: websocket.CloseNormalClosure}, err)

	suite.Run("Receives output message", func() {
		stdout, _, _ := helpers.WebSocketOutputMessages(messages)
		suite.Equal("Hello World", stdout)
	})

	suite.Run("Receives exit message", func() {
		controlMessages := helpers.WebSocketControlMessages(messages)
		suite.Require().Equal(1, len(controlMessages))
		suite.Equal(dto.WebSocketExit, controlMessages[0].Type)
	})
}

func (suite *WebSocketTestSuite) TestCancelWebSocketConnection() {
	executionId := runner.ExecutionId("sleeping-execution")
	suite.runner.Add(executionId, &executionRequestSleep)
	canceled := mockApiExecuteSleep(suite.apiMock)

	wsUrl, err := webSocketUrl("ws", suite.server, suite.router, suite.runner.Id(), executionId)
	suite.Require().NoError(err)
	connection, _, err := websocket.DefaultDialer.Dial(wsUrl.String(), nil)
	suite.Require().NoError(err)

	message, err := helpers.ReceiveNextWebSocketMessage(connection)
	suite.Require().NoError(err)
	suite.Equal(dto.WebSocketMetaStart, message.Type)

	select {
	case <-canceled:
		suite.Fail("Execute canceled unexpected")
	default:
	}

	err = connection.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""), time.Now().Add(time.Second))
	suite.Require().NoError(err)

	select {
	case <-canceled:
	case <-time.After(time.Second):
		suite.Fail("Execute not canceled")
	}
}

func (suite *WebSocketTestSuite) TestWebSocketConnectionTimeout() {
	executionId := runner.ExecutionId("time-out-execution")
	limitExecution := executionRequestSleep
	limitExecution.TimeLimit = 2
	suite.runner.Add(executionId, &limitExecution)
	canceled := mockApiExecuteSleep(suite.apiMock)

	wsUrl, err := webSocketUrl("ws", suite.server, suite.router, suite.runner.Id(), executionId)
	suite.Require().NoError(err)
	connection, _, err := websocket.DefaultDialer.Dial(wsUrl.String(), nil)
	suite.Require().NoError(err)

	message, err := helpers.ReceiveNextWebSocketMessage(connection)
	suite.Require().NoError(err)
	suite.Equal(dto.WebSocketMetaStart, message.Type)

	select {
	case <-canceled:
		suite.Fail("Execute canceled unexpected")
	case <-time.After(time.Duration(limitExecution.TimeLimit-1) * time.Second):
		<-time.After(time.Second)
	}

	select {
	case <-canceled:
	case <-time.After(time.Second):
		suite.Fail("Execute not canceled")
	}

	message, err = helpers.ReceiveNextWebSocketMessage(connection)
	suite.Require().NoError(err)
	suite.Equal(dto.WebSocketMetaTimeout, message.Type)
}

func (suite *WebSocketTestSuite) TestWebsocketStdoutAndStderr() {
	executionId := runner.ExecutionId("ls-execution")
	suite.runner.Add(executionId, &executionRequestLs)
	mockApiExecuteLs(suite.apiMock)

	wsUrl, err := webSocketUrl("ws", suite.server, suite.router, suite.runner.Id(), executionId)
	suite.Require().NoError(err)
	connection, _, err := websocket.DefaultDialer.Dial(wsUrl.String(), nil)
	suite.Require().NoError(err)

	messages, err := helpers.ReceiveAllWebSocketMessages(connection)
	suite.Require().Error(err)
	suite.Equal(&websocket.CloseError{Code: websocket.CloseNormalClosure}, err)
	stdout, stderr, _ := helpers.WebSocketOutputMessages(messages)

	suite.Contains(stdout, "existing-file")

	suite.Contains(stderr, "non-existing-file")
}

func (suite *WebSocketTestSuite) TestWebsocketError() {
	executionId := runner.ExecutionId("error-execution")
	suite.runner.Add(executionId, &executionRequestError)
	mockApiExecuteError(suite.apiMock)

	wsUrl, err := webSocketUrl("ws", suite.server, suite.router, suite.runner.Id(), executionId)
	suite.Require().NoError(err)
	connection, _, err := websocket.DefaultDialer.Dial(wsUrl.String(), nil)
	suite.Require().NoError(err)

	messages, err := helpers.ReceiveAllWebSocketMessages(connection)
	suite.Require().Error(err)
	suite.Equal(&websocket.CloseError{Code: websocket.CloseNormalClosure}, err)

	_, _, errMessages := helpers.WebSocketOutputMessages(messages)
	suite.Equal(1, len(errMessages))
	suite.Equal("Error executing the request", errMessages[0])
}

func (suite *WebSocketTestSuite) TestWebsocketNonZeroExit() {
	executionId := runner.ExecutionId("exit-execution")
	suite.runner.Add(executionId, &executionRequestExitNonZero)
	mockApiExecuteExitNonZero(suite.apiMock)

	wsUrl, err := webSocketUrl("ws", suite.server, suite.router, suite.runner.Id(), executionId)
	suite.Require().NoError(err)
	connection, _, err := websocket.DefaultDialer.Dial(wsUrl.String(), nil)
	suite.Require().NoError(err)

	messages, err := helpers.ReceiveAllWebSocketMessages(connection)
	suite.Require().Error(err)
	suite.Equal(&websocket.CloseError{Code: websocket.CloseNormalClosure}, err)

	controlMessages := helpers.WebSocketControlMessages(messages)
	suite.Equal(2, len(controlMessages))
	suite.Equal(&dto.WebSocketMessage{Type: dto.WebSocketExit, ExitCode: 42}, controlMessages[1])
}

func TestWebsocketTLS(t *testing.T) {
	runnerId := "runner-id"
	r, apiMock := helpers.NewNomadAllocationWithMockedApiClient(runnerId)

	executionId := runner.ExecutionId("execution-id")
	r.Add(executionId, &executionRequestLs)
	mockApiExecuteLs(apiMock)

	runnerManager := &runner.ManagerMock{}
	runnerManager.On("Get", r.Id()).Return(r, nil)
	router := NewRouter(runnerManager, nil)

	server, err := helpers.StartTLSServer(t, router)
	require.NoError(t, err)
	defer server.Close()

	wsUrl, err := webSocketUrl("wss", server, router, runnerId, executionId)
	require.NoError(t, err)

	config := &tls.Config{RootCAs: nil, InsecureSkipVerify: true}
	d := websocket.Dialer{TLSClientConfig: config}
	connection, _, err := d.Dial(wsUrl.String(), nil)
	require.NoError(t, err)

	message, err := helpers.ReceiveNextWebSocketMessage(connection)
	require.NoError(t, err)
	assert.Equal(t, dto.WebSocketMetaStart, message.Type)
	_, err = helpers.ReceiveAllWebSocketMessages(connection)
	require.Error(t, err)
	assert.Equal(t, &websocket.CloseError{Code: websocket.CloseNormalClosure}, err)
}

func TestRawToCodeOceanWriter(t *testing.T) {
	testMessage := "test"
	var message []byte

	connectionMock := &webSocketConnectionMock{}
	connectionMock.On("WriteMessage", mock.AnythingOfType("int"), mock.AnythingOfType("[]uint8")).
		Run(func(args mock.Arguments) {
			message = args.Get(1).([]byte)
		}).
		Return(nil)
	connectionMock.On("CloseHandler").Return(nil)
	connectionMock.On("SetCloseHandler", mock.Anything).Return()

	proxy, err := newWebSocketProxy(connectionMock)
	require.NoError(t, err)
	writer := &rawToCodeOceanWriter{
		proxy:      proxy,
		outputType: dto.WebSocketOutputStdout,
	}

	_, err = writer.Write([]byte(testMessage))
	require.NoError(t, err)

	expected, _ := json.Marshal(struct {
		Type string `json:"type"`
		Data string `json:"data"`
	}{string(dto.WebSocketOutputStdout), testMessage})
	assert.Equal(t, expected, message)
}

// --- Test suite specific test helpers ---

func webSocketUrl(scheme string, server *httptest.Server, router *mux.Router, runnerId string, executionId runner.ExecutionId) (*url.URL, error) {
	websocketUrl, err := url.Parse(server.URL)
	if err != nil {
		return nil, err
	}
	path, err := router.Get(WebsocketPath).URL(RunnerIdKey, runnerId)
	if err != nil {
		return nil, err
	}
	websocketUrl.Scheme = scheme
	websocketUrl.Path = path.Path
	websocketUrl.RawQuery = fmt.Sprintf("executionId=%s", executionId)
	return websocketUrl, nil
}

func (suite *WebSocketTestSuite) webSocketUrl(scheme, runnerId string, executionId runner.ExecutionId) (*url.URL, error) {
	return webSocketUrl(scheme, suite.server, suite.router, runnerId, executionId)
}

var executionRequestLs = dto.ExecutionRequest{Command: "ls"}

// mockApiExecuteLs mocks the ExecuteCommand of an ExecutorApi to act as if 'ls existing-file non-existing-file' was executed.
func mockApiExecuteLs(api *nomad.ExecutorApiMock) {
	helpers.MockApiExecute(api, &executionRequestLs,
		func(_ string, _ context.Context, _ []string, _ io.Reader, stdout, stderr io.Writer) (int, error) {
			_, _ = stdout.Write([]byte("existing-file\n"))
			_, _ = stderr.Write([]byte("ls: cannot access 'non-existing-file': No such file or directory\n"))
			return 0, nil
		})
}

var executionRequestHead = dto.ExecutionRequest{Command: "head -n 1"}

// mockApiExecuteHead mocks the ExecuteCommand of an ExecutorApi to act as if 'head -n 1' was executed.
func mockApiExecuteHead(api *nomad.ExecutorApiMock) {
	helpers.MockApiExecute(api, &executionRequestHead,
		func(_ string, _ context.Context, _ []string, stdin io.Reader, stdout io.Writer, stderr io.Writer) (int, error) {
			scanner := bufio.NewScanner(stdin)
			for !scanner.Scan() {
				scanner = bufio.NewScanner(stdin)
			}
			_, _ = stdout.Write(scanner.Bytes())
			return 0, nil
		})
}

var executionRequestSleep = dto.ExecutionRequest{Command: "sleep infinity"}

// mockApiExecuteSleep mocks the ExecuteCommand method of an ExecutorAPI to sleep until the execution is canceled.
func mockApiExecuteSleep(api *nomad.ExecutorApiMock) <-chan bool {
	canceled := make(chan bool, 1)
	helpers.MockApiExecute(api, &executionRequestSleep,
		func(_ string, ctx context.Context, _ []string, stdin io.Reader, stdout io.Writer, stderr io.Writer) (int, error) {
			<-ctx.Done()
			close(canceled)
			return 0, ctx.Err()
		})
	return canceled
}

var executionRequestError = dto.ExecutionRequest{Command: "error"}

// mockApiExecuteError mocks the ExecuteCommand method of an ExecutorApi to return an error.
func mockApiExecuteError(api *nomad.ExecutorApiMock) {
	helpers.MockApiExecute(api, &executionRequestError,
		func(_ string, _ context.Context, _ []string, _ io.Reader, _, _ io.Writer) (int, error) {
			return 0, errors.New("intended error")
		})
}

var executionRequestExitNonZero = dto.ExecutionRequest{Command: "exit 42"}

// mockApiExecuteExitNonZero mocks the ExecuteCommand method of an ExecutorApi to exit with exit status 42.
func mockApiExecuteExitNonZero(api *nomad.ExecutorApiMock) {
	helpers.MockApiExecute(api, &executionRequestExitNonZero,
		func(_ string, _ context.Context, _ []string, _ io.Reader, _, _ io.Writer) (int, error) {
			return 42, nil
		})
}
