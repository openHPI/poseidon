package api

import (
	"bufio"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

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
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
)

func TestWebSocketTestSuite(t *testing.T) {
	suite.Run(t, new(WebSocketTestSuite))
}

type WebSocketTestSuite struct {
	tests.MemoryLeakTestSuite
	router      *mux.Router
	executionID string
	runner      runner.Runner
	apiMock     *nomad.ExecutorAPIMock
	server      *httptest.Server
}

func (s *WebSocketTestSuite) SetupTest() {
	s.MemoryLeakTestSuite.SetupTest()
	runnerID := "runner-id"
	s.runner, s.apiMock = newNomadAllocationWithMockedAPIClient(s.TestCtx, runnerID)

	// default execution
	s.executionID = tests.DefaultExecutionID
	s.runner.StoreExecution(s.executionID, &executionRequestLs)
	mockAPIExecuteLs(s.apiMock)

	runnerManager := &runner.ManagerMock{}
	runnerManager.On("Get", s.runner.ID()).Return(s.runner, nil)
	s.router = NewRouter(runnerManager, nil)
	s.server = httptest.NewServer(s.router)
}

func (s *WebSocketTestSuite) TearDownTest() {
	defer s.MemoryLeakTestSuite.TearDownTest()
	s.server.Close()
	err := s.runner.Destroy(nil)
	s.Require().NoError(err)
}

func (s *WebSocketTestSuite) TestWebsocketConnectionCanBeEstablished() {
	wsURL, err := s.webSocketURL("ws", s.runner.ID(), s.executionID)
	s.Require().NoError(err)
	conn, _, err := websocket.DefaultDialer.Dial(wsURL.String(), nil)
	s.Require().NoError(err)

	<-time.After(tests.ShortTimeout)
	err = conn.Close()
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
	_, err = io.ReadAll(response.Body)
	s.Require().NoError(err)
}

func (s *WebSocketTestSuite) TestWebsocketConnection() {
	s.runner.StoreExecution(s.executionID, &executionRequestHead)
	mockAPIExecuteHead(s.apiMock)
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
		s.Require().Len(controlMessages, 1)
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
	s.Require().Len(errMessages, 1)
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
	s.Len(controlMessages, 2)
	s.Equal(&dto.WebSocketMessage{Type: dto.WebSocketExit, ExitCode: 42}, controlMessages[1])
}

func (s *MainTestSuite) TestWebsocketTLS() {
	runnerID := "runner-id"
	r, apiMock := newNomadAllocationWithMockedAPIClient(s.TestCtx, runnerID)

	executionID := tests.DefaultExecutionID
	r.StoreExecution(executionID, &executionRequestLs)
	mockAPIExecuteLs(apiMock)

	runnerManager := &runner.ManagerMock{}
	runnerManager.On("Get", r.ID()).Return(r, nil)
	router := NewRouter(runnerManager, nil)

	server, err := helpers.StartTLSServer(s.T(), router)
	s.Require().NoError(err)
	defer server.Close()

	wsURL, err := webSocketURL("wss", server, router, runnerID, executionID)
	s.Require().NoError(err)

	config := &tls.Config{RootCAs: nil, InsecureSkipVerify: true} //nolint:gosec // test needs self-signed cert
	d := websocket.Dialer{TLSClientConfig: config}
	connection, _, err := d.Dial(wsURL.String(), nil)
	s.Require().NoError(err)

	message, err := helpers.ReceiveNextWebSocketMessage(connection)
	s.Require().NoError(err)
	s.Equal(dto.WebSocketMetaStart, message.Type)
	_, err = helpers.ReceiveAllWebSocketMessages(connection)
	s.Require().Error(err)
	s.True(websocket.IsCloseError(err, websocket.CloseNormalClosure))
	s.Require().NoError(r.Destroy(nil))
}

func (s *MainTestSuite) TestWebSocketProxyStopsReadingTheWebSocketAfterClosingIt() {
	apiMock := &nomad.ExecutorAPIMock{}
	executionID := tests.DefaultExecutionID

	r, wsURL, cleanup := newRunnerWithNotMockedRunnerManager(s, apiMock, executionID)
	defer cleanup()

	logger, hook := test.NewNullLogger()
	log = logger.WithField("pkg", "api")

	r.StoreExecution(executionID, &executionRequestHead)
	mockAPIExecute(apiMock, &executionRequestHead,
		func(_ context.Context, _ string, _ string, _ bool, _ io.Reader, _, _ io.Writer) (int, error) {
			return 0, nil
		})
	connection, _, err := websocket.DefaultDialer.Dial(wsURL.String(), nil)
	s.Require().NoError(err)

	_, err = helpers.ReceiveAllWebSocketMessages(connection)
	s.Require().Error(err)
	s.True(websocket.IsCloseError(err, websocket.CloseNormalClosure))
	for _, logMsg := range hook.Entries {
		if logMsg.Level < logrus.InfoLevel {
			s.Fail(logMsg.Message)
		}
	}
}

// --- Test suite specific test helpers ---

func newNomadAllocationWithMockedAPIClient(ctx context.Context, runnerID string) (runner.Runner, *nomad.ExecutorAPIMock) {
	executorAPIMock := &nomad.ExecutorAPIMock{}
	executorAPIMock.On("DeleteJob", mock.AnythingOfType("string")).Return(nil)
	manager := &runner.ManagerMock{}
	manager.On("Return", mock.Anything).Return(nil)
	r := runner.NewNomadJob(ctx, runnerID, nil, executorAPIMock, nil)
	return r, executorAPIMock
}

func newRunnerWithNotMockedRunnerManager(s *MainTestSuite, apiMock *nomad.ExecutorAPIMock, executionID string) (
	r runner.Runner, wsURL *url.URL, cleanup func(),
) {
	s.T().Helper()
	apiMock.On("SetRunnerMetaUsed", mock.AnythingOfType("string"), mock.AnythingOfType("bool"), mock.AnythingOfType("int")).Return(nil)
	apiMock.On("LoadRunnerIDs", mock.AnythingOfType("string")).Return([]string{}, nil)
	apiMock.On("DeleteJob", mock.AnythingOfType("string")).Return(nil)
	apiMock.On("RegisterRunnerJob", mock.AnythingOfType("*api.Job")).Return(nil)
	call := apiMock.On("WatchEventStream", mock.Anything, mock.Anything, mock.Anything)
	call.Run(func(_ mock.Arguments) {
		<-s.TestCtx.Done()
		call.ReturnArguments = mock.Arguments{nil}
	})

	runnerManager := runner.NewNomadRunnerManager(s.TestCtx, apiMock)
	router := NewRouter(runnerManager, nil)
	s.ExpectedGoroutineIncrease++ // The server is not closing properly. Therefore, we don't even try.
	server := httptest.NewServer(router)

	runnerID := tests.DefaultRunnerID
	runnerJob := runner.NewNomadJob(s.TestCtx, runnerID, nil, apiMock, nil)
	nomadEnvironment, err := environment.NewNomadEnvironment(s.TestCtx, 0, apiMock, `job "template-0" {}`)
	s.Require().NoError(err)
	eID, err := nomad.EnvironmentIDFromRunnerID(runnerID)
	s.Require().NoError(err)
	nomadEnvironment.SetID(eID)
	nomadEnvironment.SetPrewarmingPoolSize(0)
	runnerManager.StoreEnvironment(nomadEnvironment)
	nomadEnvironment.AddRunner(runnerJob)

	r, err = runnerManager.Claim(nomadEnvironment.ID(), int(tests.DefaultTestTimeout.Seconds()))
	s.Require().NoError(err)
	wsURL, err = webSocketURL("ws", server, router, r.ID(), executionID)
	s.Require().NoError(err)

	return r, wsURL, func() {
		err = r.Destroy(tests.ErrCleanupDestroyReason)
		s.Require().NoError(err)
		err = nomadEnvironment.Delete(tests.ErrCleanupDestroyReason)
		s.Require().NoError(err)
	}
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
	websocketURL.RawQuery = "executionID=" + executionID
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
		func(_ context.Context, _ string, _ string, _ bool, _ io.Reader, stdout, stderr io.Writer) (int, error) {
			_, _ = stdout.Write([]byte("existing-file\n"))
			_, _ = stderr.Write([]byte("ls: cannot access 'non-existing-file': No such file or directory\n"))
			return 0, nil
		})
}

var executionRequestHead = dto.ExecutionRequest{Command: "head -n 1"}

// mockAPIExecuteHead mocks the ExecuteCommand of an ExecutorApi to act as if 'head -n 1' was executed.
func mockAPIExecuteHead(api *nomad.ExecutorAPIMock) {
	mockAPIExecute(api, &executionRequestHead,
		func(_ context.Context, _ string, _ string, _ bool,
			stdin io.Reader, stdout io.Writer, _ io.Writer,
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
		func(ctx context.Context, _ string, _ string, _ bool,
			stdin io.Reader, _ io.Writer, _ io.Writer,
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
		func(_ context.Context, _ string, _ string, _ bool, _ io.Reader, _, _ io.Writer) (int, error) {
			return 0, tests.ErrDefault
		})
}

var executionRequestExitNonZero = dto.ExecutionRequest{Command: "exit 42"}

// mockAPIExecuteExitNonZero mocks the ExecuteCommand method of an ExecutorApi to exit with exit status 42.
func mockAPIExecuteExitNonZero(api *nomad.ExecutorAPIMock) {
	mockAPIExecute(api, &executionRequestExitNonZero,
		func(_ context.Context, _ string, _ string, _ bool, _ io.Reader, _, _ io.Writer) (int, error) {
			return 42, nil
		})
}

// mockAPIExecute mocks the ExecuteCommand method of an ExecutorApi to call the given method run when the command
// corresponding to the given ExecutionRequest is called.
func mockAPIExecute(api *nomad.ExecutorAPIMock, request *dto.ExecutionRequest,
	run func(ctx context.Context, runnerId string, command string, tty bool,
		stdin io.Reader, stdout, stderr io.Writer) (int, error),
) {
	tests.RemoveMethodFromMock(&api.Mock, "ExecuteCommand")
	call := api.On("ExecuteCommand",
		mock.Anything,
		mock.AnythingOfType("string"),
		request.FullCommand(),
		mock.AnythingOfType("bool"),
		mock.AnythingOfType("bool"),
		mock.Anything,
		mock.Anything,
		mock.Anything)
	call.Run(func(args mock.Arguments) {
		mockCtx, errCtx := args.Get(0).(context.Context)
		mockRunnerID, errRunnerID := args.Get(1).(string)
		mockCommand, errCommand := args.Get(2).(string)
		mockTty, errTty := args.Get(3).(bool)
		mockStdin, errStdin := args.Get(5).(io.Reader)
		mockStdout, errStdout := args.Get(6).(io.Writer)
		mockStderr, errStderr := args.Get(7).(io.Writer)

		if !errCtx || !errRunnerID || !errCommand || !errTty || !errStdin || !errStdout || !errStderr {
			call.ReturnArguments = mock.Arguments{1, fmt.Errorf(
				"%w: errCtx=%v, errRunnerId=%v, errCommand=%v, errTty=%v, errStdin=%v, errStdout=%v, errStderr=%v",
				tests.ErrDefault, errCtx, errRunnerID, errCommand, errTty, errStdin, errStdout, errStderr)}
		} else {
			exit, err := run(mockCtx, mockRunnerID, mockCommand, mockTty, mockStdin, mockStdout, mockStderr)
			call.ReturnArguments = mock.Arguments{exit, err}
		}
	})
}
