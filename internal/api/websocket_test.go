package api

import (
	"bufio"
	"context"
	"crypto/tls"
	"fmt"
	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	"github.com/openHPI/poseidon/internal/environment"
	"github.com/openHPI/poseidon/internal/nomad"
	"github.com/openHPI/poseidon/internal/runner"
	"github.com/openHPI/poseidon/pkg/dto"
	"github.com/openHPI/poseidon/tests"
	"github.com/openHPI/poseidon/tests/helpers"
	"github.com/sirupsen/logrus"
	"github.com/sirupsen/logrus/hooks/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
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
	executionID string
	runner      runner.Runner
	apiMock     *nomad.ExecutorAPIMock
	server      *httptest.Server
}

func (s *WebSocketTestSuite) SetupTest() {
	runnerID := "runner-id"
	s.runner, s.apiMock = newNomadAllocationWithMockedAPIClient(runnerID)

	// default execution
	s.executionID = tests.DefaultExecutionID
	s.runner.StoreExecution(s.executionID, &executionRequestHead)
	mockAPIExecuteHead(s.apiMock)

	runnerManager := &runner.ManagerMock{}
	runnerManager.On("Get", s.runner.ID()).Return(s.runner, nil)
	s.router = NewRouter(runnerManager, nil)
	s.server = httptest.NewServer(s.router)
}

func (s *WebSocketTestSuite) TearDownTest() {
	s.server.Close()
}

func (s *WebSocketTestSuite) TestWebsocketConnectionCanBeEstablished() {
	wsURL, err := s.webSocketURL("ws", s.runner.ID(), s.executionID)
	s.Require().NoError(err)
	_, _, err = websocket.DefaultDialer.Dial(wsURL.String(), nil)
	s.Require().NoError(err)
}

func (s *WebSocketTestSuite) TestWebsocketReturns404IfExecutionDoesNotExist() {
	wsURL, err := s.webSocketURL("ws", s.runner.ID(), "invalid-execution-id")
	s.Require().NoError(err)
	_, response, err := websocket.DefaultDialer.Dial(wsURL.String(), nil)
	s.Require().ErrorIs(err, websocket.ErrBadHandshake)
	s.Equal(http.StatusNotFound, response.StatusCode)
}

func (s *WebSocketTestSuite) TestWebsocketReturns400IfRequestedViaHttp() {
	wsURL, err := s.webSocketURL("http", s.runner.ID(), s.executionID)
	s.Require().NoError(err)
	response, err := http.Get(wsURL.String())
	s.Require().NoError(err)
	// This functionality is implemented by the WebSocket library.
	s.Equal(http.StatusBadRequest, response.StatusCode)
}

func (s *WebSocketTestSuite) TestWebsocketConnection() {
	wsURL, err := s.webSocketURL("ws", s.runner.ID(), s.executionID)
	s.Require().NoError(err)
	connection, _, err := websocket.DefaultDialer.Dial(wsURL.String(), nil)
	s.Require().NoError(err)
	err = connection.SetReadDeadline(time.Now().Add(5 * time.Second))
	s.Require().NoError(err)

	s.Run("Receives start message", func() {
		message, err := helpers.ReceiveNextWebSocketMessage(connection)
		s.Require().NoError(err)
		s.Equal(dto.WebSocketMetaStart, message.Type)
	})

	s.Run("Executes the request in the runner", func() {
		<-time.After(tests.ShortTimeout)
		s.apiMock.AssertCalled(s.T(), "ExecuteCommand",
			mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.AnythingOfType("bool"),
			mock.Anything, mock.Anything, mock.Anything)
	})

	s.Run("Can send input", func() {
		err = connection.WriteMessage(websocket.TextMessage, []byte("Hello World\n"))
		s.Require().NoError(err)
	})

	messages, err := helpers.ReceiveAllWebSocketMessages(connection)
	s.Require().Error(err)
	s.True(websocket.IsCloseError(err, websocket.CloseNormalClosure))

	s.Run("Receives output message", func() {
		stdout, _, _ := helpers.WebSocketOutputMessages(messages)
		s.Equal("Hello World", stdout)
	})

	s.Run("Receives exit message", func() {
		controlMessages := helpers.WebSocketControlMessages(messages)
		s.Require().Equal(1, len(controlMessages))
		s.Equal(dto.WebSocketExit, controlMessages[0].Type)
	})
}

func (s *WebSocketTestSuite) TestCancelWebSocketConnection() {
	executionID := "sleeping-execution"
	s.runner.StoreExecution(executionID, &executionRequestSleep)
	canceled := mockAPIExecuteSleep(s.apiMock)

	wsURL, err := webSocketURL("ws", s.server, s.router, s.runner.ID(), executionID)
	s.Require().NoError(err)
	connection, _, err := websocket.DefaultDialer.Dial(wsURL.String(), nil)
	s.Require().NoError(err)

	message, err := helpers.ReceiveNextWebSocketMessage(connection)
	s.Require().NoError(err)
	s.Equal(dto.WebSocketMetaStart, message.Type)

	select {
	case <-canceled:
		s.Fail("ExecuteInteractively canceled unexpected")
	default:
	}

	err = connection.WriteControl(websocket.CloseMessage,
		websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""), time.Now().Add(time.Second))
	s.Require().NoError(err)

	select {
	case <-canceled:
	case <-time.After(time.Second):
		s.Fail("ExecuteInteractively not canceled")
	}
}

func (s *WebSocketTestSuite) TestWebSocketConnectionTimeout() {
	executionID := "time-out-execution"
	limitExecution := executionRequestSleep
	limitExecution.TimeLimit = 2
	s.runner.StoreExecution(executionID, &limitExecution)
	canceled := mockAPIExecuteSleep(s.apiMock)

	wsURL, err := webSocketURL("ws", s.server, s.router, s.runner.ID(), executionID)
	s.Require().NoError(err)
	connection, _, err := websocket.DefaultDialer.Dial(wsURL.String(), nil)
	s.Require().NoError(err)

	message, err := helpers.ReceiveNextWebSocketMessage(connection)
	s.Require().NoError(err)
	s.Equal(dto.WebSocketMetaStart, message.Type)

	select {
	case <-canceled:
		s.Fail("ExecuteInteractively canceled unexpected")
	case <-time.After(time.Duration(limitExecution.TimeLimit-1) * time.Second):
		<-time.After(time.Second)
	}

	select {
	case <-canceled:
	case <-time.After(time.Second):
		s.Fail("ExecuteInteractively not canceled")
	}

	message, err = helpers.ReceiveNextWebSocketMessage(connection)
	s.Require().NoError(err)
	s.Equal(dto.WebSocketMetaTimeout, message.Type)
}

func (s *WebSocketTestSuite) TestWebsocketStdoutAndStderr() {
	executionID := "ls-execution"
	s.runner.StoreExecution(executionID, &executionRequestLs)
	mockAPIExecuteLs(s.apiMock)

	wsURL, err := webSocketURL("ws", s.server, s.router, s.runner.ID(), executionID)
	s.Require().NoError(err)
	connection, _, err := websocket.DefaultDialer.Dial(wsURL.String(), nil)
	s.Require().NoError(err)

	messages, err := helpers.ReceiveAllWebSocketMessages(connection)
	s.Require().Error(err)
	s.True(websocket.IsCloseError(err, websocket.CloseNormalClosure))
	stdout, stderr, _ := helpers.WebSocketOutputMessages(messages)

	s.Contains(stdout, "existing-file")

	s.Contains(stderr, "non-existing-file")
}

func (s *WebSocketTestSuite) TestWebsocketError() {
	executionID := "error-execution"
	s.runner.StoreExecution(executionID, &executionRequestError)
	mockAPIExecuteError(s.apiMock)

	wsURL, err := webSocketURL("ws", s.server, s.router, s.runner.ID(), executionID)
	s.Require().NoError(err)
	connection, _, err := websocket.DefaultDialer.Dial(wsURL.String(), nil)
	s.Require().NoError(err)

	messages, err := helpers.ReceiveAllWebSocketMessages(connection)
	s.Require().Error(err)
	s.True(websocket.IsCloseError(err, websocket.CloseNormalClosure))

	_, _, errMessages := helpers.WebSocketOutputMessages(messages)
	s.Require().Equal(1, len(errMessages))
	s.Equal("Error executing the request", errMessages[0])
}

func (s *WebSocketTestSuite) TestWebsocketNonZeroExit() {
	executionID := "exit-execution"
	s.runner.StoreExecution(executionID, &executionRequestExitNonZero)
	mockAPIExecuteExitNonZero(s.apiMock)

	wsURL, err := webSocketURL("ws", s.server, s.router, s.runner.ID(), executionID)
	s.Require().NoError(err)
	connection, _, err := websocket.DefaultDialer.Dial(wsURL.String(), nil)
	s.Require().NoError(err)

	messages, err := helpers.ReceiveAllWebSocketMessages(connection)
	s.Require().Error(err)
	s.True(websocket.IsCloseError(err, websocket.CloseNormalClosure))

	controlMessages := helpers.WebSocketControlMessages(messages)
	s.Equal(2, len(controlMessages))
	s.Equal(&dto.WebSocketMessage{Type: dto.WebSocketExit, ExitCode: 42}, controlMessages[1])
}

func TestWebsocketTLS(t *testing.T) {
	runnerID := "runner-id"
	r, apiMock := newNomadAllocationWithMockedAPIClient(runnerID)

	executionID := tests.DefaultExecutionID
	r.StoreExecution(executionID, &executionRequestLs)
	mockAPIExecuteLs(apiMock)

	runnerManager := &runner.ManagerMock{}
	runnerManager.On("Get", r.ID()).Return(r, nil)
	router := NewRouter(runnerManager, nil)

	server, err := helpers.StartTLSServer(t, router)
	require.NoError(t, err)
	defer server.Close()

	wsURL, err := webSocketURL("wss", server, router, runnerID, executionID)
	require.NoError(t, err)

	config := &tls.Config{RootCAs: nil, InsecureSkipVerify: true} //nolint:gosec // test needs self-signed cert
	d := websocket.Dialer{TLSClientConfig: config}
	connection, _, err := d.Dial(wsURL.String(), nil)
	require.NoError(t, err)

	message, err := helpers.ReceiveNextWebSocketMessage(connection)
	require.NoError(t, err)
	assert.Equal(t, dto.WebSocketMetaStart, message.Type)
	_, err = helpers.ReceiveAllWebSocketMessages(connection)
	require.Error(t, err)
	assert.True(t, websocket.IsCloseError(err, websocket.CloseNormalClosure))
}

func TestWebSocketProxyStopsReadingTheWebSocketAfterClosingIt(t *testing.T) {
	apiMock := &nomad.ExecutorAPIMock{}
	executionID := tests.DefaultExecutionID
	r, wsURL := newRunnerWithNotMockedRunnerManager(t, apiMock, executionID)

	logger, hook := test.NewNullLogger()
	log = logger.WithField("pkg", "api")

	r.StoreExecution(executionID, &executionRequestHead)
	mockAPIExecute(apiMock, &executionRequestHead,
		func(_ string, ctx context.Context, _ string, _ bool, _ io.Reader, _, _ io.Writer) (int, error) {
			return 0, nil
		})
	connection, _, err := websocket.DefaultDialer.Dial(wsURL.String(), nil)
	require.NoError(t, err)

	_, err = helpers.ReceiveAllWebSocketMessages(connection)
	require.Error(t, err)
	assert.True(t, websocket.IsCloseError(err, websocket.CloseNormalClosure))
	for _, logMsg := range hook.Entries {
		if logMsg.Level < logrus.InfoLevel {
			assert.Fail(t, logMsg.Message)
		}
	}
}

// --- Test suite specific test helpers ---

func newNomadAllocationWithMockedAPIClient(runnerID string) (runner.Runner, *nomad.ExecutorAPIMock) {
	executorAPIMock := &nomad.ExecutorAPIMock{}
	manager := &runner.ManagerMock{}
	manager.On("Return", mock.Anything).Return(nil)
	r := runner.NewNomadJob(runnerID, nil, executorAPIMock, manager.Return)
	return r, executorAPIMock
}

func newRunnerWithNotMockedRunnerManager(t *testing.T, apiMock *nomad.ExecutorAPIMock, executionID string) (
	r runner.Runner, wsURL *url.URL) {
	t.Helper()
	apiMock.On("MarkRunnerAsUsed", mock.AnythingOfType("string"), mock.AnythingOfType("int")).Return(nil)
	apiMock.On("DeleteJob", mock.AnythingOfType("string")).Return(nil)
	apiMock.On("RegisterRunnerJob", mock.AnythingOfType("*api.Job")).Return(nil)
	call := apiMock.On("WatchEventStream", mock.Anything, mock.Anything, mock.Anything)
	call.Run(func(args mock.Arguments) {
		<-context.Background().Done()
		call.ReturnArguments = mock.Arguments{nil}
	})
	runnerManager := runner.NewNomadRunnerManager(apiMock, context.Background())
	router := NewRouter(runnerManager, nil)
	server := httptest.NewServer(router)

	runnerID := tests.DefaultRunnerID
	runnerJob := runner.NewNomadJob(runnerID, nil, apiMock, runnerManager.Return)
	e, err := environment.NewNomadEnvironment(0, apiMock, "job \"template-0\" {}")
	require.NoError(t, err)
	eID, err := nomad.EnvironmentIDFromRunnerID(runnerID)
	require.NoError(t, err)
	e.SetID(eID)
	e.SetPrewarmingPoolSize(0)
	runnerManager.StoreEnvironment(e)
	e.AddRunner(runnerJob)

	r, err = runnerManager.Claim(e.ID(), int(tests.DefaultTestTimeout.Seconds()))
	require.NoError(t, err)
	wsURL, err = webSocketURL("ws", server, router, r.ID(), executionID)
	require.NoError(t, err)
	return r, wsURL
}

func webSocketURL(scheme string, server *httptest.Server, router *mux.Router,
	runnerID string, executionID string,
) (*url.URL, error) {
	websocketURL, err := url.Parse(server.URL)
	if err != nil {
		return nil, err
	}
	path, err := router.Get(WebsocketPath).URL(RunnerIDKey, runnerID)
	if err != nil {
		return nil, err
	}
	websocketURL.Scheme = scheme
	websocketURL.Path = path.Path
	websocketURL.RawQuery = fmt.Sprintf("executionID=%s", executionID)
	return websocketURL, nil
}

func (s *WebSocketTestSuite) webSocketURL(scheme, runnerID, executionID string) (*url.URL, error) {
	return webSocketURL(scheme, s.server, s.router, runnerID, executionID)
}

var executionRequestLs = dto.ExecutionRequest{Command: "ls"}

// mockAPIExecuteLs mocks the ExecuteCommand of an ExecutorApi to act as if
// 'ls existing-file non-existing-file' was executed.
func mockAPIExecuteLs(api *nomad.ExecutorAPIMock) {
	mockAPIExecute(api, &executionRequestLs,
		func(_ string, _ context.Context, _ string, _ bool, _ io.Reader, stdout, stderr io.Writer) (int, error) {
			_, _ = stdout.Write([]byte("existing-file\n"))
			_, _ = stderr.Write([]byte("ls: cannot access 'non-existing-file': No such file or directory\n"))
			return 0, nil
		})
}

var executionRequestHead = dto.ExecutionRequest{Command: "head -n 1"}

// mockAPIExecuteHead mocks the ExecuteCommand of an ExecutorApi to act as if 'head -n 1' was executed.
func mockAPIExecuteHead(api *nomad.ExecutorAPIMock) {
	mockAPIExecute(api, &executionRequestHead,
		func(_ string, _ context.Context, _ string, _ bool,
			stdin io.Reader, stdout io.Writer, stderr io.Writer,
		) (int, error) {
			scanner := bufio.NewScanner(stdin)
			for !scanner.Scan() {
				scanner = bufio.NewScanner(stdin)
			}
			_, _ = stdout.Write(scanner.Bytes())
			return 0, nil
		})
}

var executionRequestSleep = dto.ExecutionRequest{Command: "sleep infinity"}

// mockAPIExecuteSleep mocks the ExecuteCommand method of an ExecutorAPI to sleep
// until the execution receives a SIGQUIT.
func mockAPIExecuteSleep(api *nomad.ExecutorAPIMock) <-chan bool {
	canceled := make(chan bool, 1)

	mockAPIExecute(api, &executionRequestSleep,
		func(_ string, ctx context.Context, _ string, _ bool,
			stdin io.Reader, stdout io.Writer, stderr io.Writer,
		) (int, error) {
			var err error
			buffer := make([]byte, 1) //nolint:makezero // if the length is zero, the Read call never reads anything
			for n := 0; !(n == 1 && buffer[0] == runner.SIGQUIT); n, err = stdin.Read(buffer) {
				if err != nil {
					return 0, fmt.Errorf("error while reading stdin: %w", err)
				}
			}
			close(canceled)
			return 0, ctx.Err()
		})
	return canceled
}

var executionRequestError = dto.ExecutionRequest{Command: "error"}

// mockAPIExecuteError mocks the ExecuteCommand method of an ExecutorApi to return an error.
func mockAPIExecuteError(api *nomad.ExecutorAPIMock) {
	mockAPIExecute(api, &executionRequestError,
		func(_ string, _ context.Context, _ string, _ bool, _ io.Reader, _, _ io.Writer) (int, error) {
			return 0, tests.ErrDefault
		})
}

var executionRequestExitNonZero = dto.ExecutionRequest{Command: "exit 42"}

// mockAPIExecuteExitNonZero mocks the ExecuteCommand method of an ExecutorApi to exit with exit status 42.
func mockAPIExecuteExitNonZero(api *nomad.ExecutorAPIMock) {
	mockAPIExecute(api, &executionRequestExitNonZero,
		func(_ string, _ context.Context, _ string, _ bool, _ io.Reader, _, _ io.Writer) (int, error) {
			return 42, nil
		})
}

// mockAPIExecute mocks the ExecuteCommand method of an ExecutorApi to call the given method run when the command
// corresponding to the given ExecutionRequest is called.
func mockAPIExecute(api *nomad.ExecutorAPIMock, request *dto.ExecutionRequest,
	run func(runnerId string, ctx context.Context, command string, tty bool,
		stdin io.Reader, stdout, stderr io.Writer) (int, error)) {
	call := api.On("ExecuteCommand",
		mock.AnythingOfType("string"),
		mock.Anything,
		request.FullCommand(),
		mock.AnythingOfType("bool"),
		mock.AnythingOfType("bool"),
		mock.Anything,
		mock.Anything,
		mock.Anything)
	call.Run(func(args mock.Arguments) {
		exit, err := run(args.Get(0).(string),
			args.Get(1).(context.Context),
			args.Get(2).(string),
			args.Get(3).(bool),
			args.Get(5).(io.Reader),
			args.Get(6).(io.Writer),
			args.Get(7).(io.Writer))
		call.ReturnArguments = mock.Arguments{exit, err}
	})
}
