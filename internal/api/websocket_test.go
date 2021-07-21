package api

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/internal/nomad"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/internal/runner"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/pkg/dto"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/tests"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/tests/helpers"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestWebSocketTestSuite(t *testing.T) {
	suite.Run(t, new(WebSocketTestSuite))
}

type WebSocketTestSuite struct {
	suite.Suite
	router      *mux.Router
	executionID runner.ExecutionID
	runner      runner.Runner
	apiMock     *nomad.ExecutorAPIMock
	server      *httptest.Server
}

func (s *WebSocketTestSuite) SetupTest() {
	runnerID := "runner-id"
	s.runner, s.apiMock = newNomadAllocationWithMockedAPIClient(runnerID)

	// default execution
	s.executionID = "execution-id"
	s.runner.Add(s.executionID, &executionRequestHead)
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
		<-time.After(100 * time.Millisecond)
		s.apiMock.AssertCalled(s.T(), "ExecuteCommand",
			mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything)
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
	executionID := runner.ExecutionID("sleeping-execution")
	s.runner.Add(executionID, &executionRequestSleep)
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
	executionID := runner.ExecutionID("time-out-execution")
	limitExecution := executionRequestSleep
	limitExecution.TimeLimit = 2
	s.runner.Add(executionID, &limitExecution)
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
	executionID := runner.ExecutionID("ls-execution")
	s.runner.Add(executionID, &executionRequestLs)
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
	executionID := runner.ExecutionID("error-execution")
	s.runner.Add(executionID, &executionRequestError)
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
	executionID := runner.ExecutionID("exit-execution")
	s.runner.Add(executionID, &executionRequestExitNonZero)
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

	executionID := runner.ExecutionID("execution-id")
	r.Add(executionID, &executionRequestLs)
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

func TestRawToCodeOceanWriter(t *testing.T) {
	testMessage := "test"
	var message []byte

	connectionMock := &webSocketConnectionMock{}
	connectionMock.On("WriteMessage", mock.AnythingOfType("int"), mock.AnythingOfType("[]uint8")).
		Run(func(args mock.Arguments) {
			var ok bool
			message, ok = args.Get(1).([]byte)
			require.True(t, ok)
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

	expected, err := json.Marshal(struct {
		Type string `json:"type"`
		Data string `json:"data"`
	}{string(dto.WebSocketOutputStdout), testMessage})
	require.NoError(t, err)
	assert.Equal(t, expected, message)
}

func TestCodeOceanToRawReaderReturnsOnlyAfterOneByteWasRead(t *testing.T) {
	reader := newCodeOceanToRawReader(nil)

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

	connection := &webSocketConnectionMock{}
	connection.On("WriteMessage", mock.AnythingOfType("int"), mock.AnythingOfType("[]uint8")).Return(nil)
	connection.On("CloseHandler").Return(nil)
	connection.On("SetCloseHandler", mock.Anything).Return()
	call := connection.On("NextReader")
	call.Run(func(_ mock.Arguments) {
		call.Return(websocket.TextMessage, <-messages, nil)
	})

	reader := newCodeOceanToRawReader(connection)
	cancel := reader.startReadInputLoop()
	defer cancel()

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

// --- Test suite specific test helpers ---

func newNomadAllocationWithMockedAPIClient(runnerID string) (runner.Runner, *nomad.ExecutorAPIMock) {
	executorAPIMock := &nomad.ExecutorAPIMock{}
	manager := &runner.ManagerMock{}
	manager.On("Return", mock.Anything).Return(nil)
	r := runner.NewNomadJob(runnerID, nil, executorAPIMock, manager)
	return r, executorAPIMock
}

func webSocketURL(scheme string, server *httptest.Server, router *mux.Router,
	runnerID string, executionID runner.ExecutionID,
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

func (s *WebSocketTestSuite) webSocketURL(scheme, runnerID string, executionID runner.ExecutionID) (*url.URL, error) {
	return webSocketURL(scheme, s.server, s.router, runnerID, executionID)
}

var executionRequestLs = dto.ExecutionRequest{Command: "ls"}

// mockAPIExecuteLs mocks the ExecuteCommand of an ExecutorApi to act as if
// 'ls existing-file non-existing-file' was executed.
func mockAPIExecuteLs(api *nomad.ExecutorAPIMock) {
	mockAPIExecute(api, &executionRequestLs,
		func(_ string, _ context.Context, _ []string, _ bool, _ io.Reader, stdout, stderr io.Writer) (int, error) {
			_, _ = stdout.Write([]byte("existing-file\n"))
			_, _ = stderr.Write([]byte("ls: cannot access 'non-existing-file': No such file or directory\n"))
			return 0, nil
		})
}

var executionRequestHead = dto.ExecutionRequest{Command: "head -n 1"}

// mockAPIExecuteHead mocks the ExecuteCommand of an ExecutorApi to act as if 'head -n 1' was executed.
func mockAPIExecuteHead(api *nomad.ExecutorAPIMock) {
	mockAPIExecute(api, &executionRequestHead,
		func(_ string, _ context.Context, _ []string, _ bool,
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
		func(_ string, ctx context.Context, _ []string, _ bool,
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
		func(_ string, _ context.Context, _ []string, _ bool, _ io.Reader, _, _ io.Writer) (int, error) {
			return 0, tests.ErrDefault
		})
}

var executionRequestExitNonZero = dto.ExecutionRequest{Command: "exit 42"}

// mockAPIExecuteExitNonZero mocks the ExecuteCommand method of an ExecutorApi to exit with exit status 42.
func mockAPIExecuteExitNonZero(api *nomad.ExecutorAPIMock) {
	mockAPIExecute(api, &executionRequestExitNonZero,
		func(_ string, _ context.Context, _ []string, _ bool, _ io.Reader, _, _ io.Writer) (int, error) {
			return 42, nil
		})
}

// mockAPIExecute mocks the ExecuteCommand method of an ExecutorApi to call the given method run when the command
// corresponding to the given ExecutionRequest is called.
func mockAPIExecute(api *nomad.ExecutorAPIMock, request *dto.ExecutionRequest,
	run func(runnerId string, ctx context.Context, command []string, tty bool,
		stdin io.Reader, stdout, stderr io.Writer) (int, error),
) {
	call := api.On("ExecuteCommand",
		mock.AnythingOfType("string"),
		mock.Anything,
		request.FullCommand(),
		mock.AnythingOfType("bool"),
		mock.Anything,
		mock.Anything,
		mock.Anything)
	call.Run(func(args mock.Arguments) {
		exit, err := run(args.Get(0).(string),
			args.Get(1).(context.Context),
			args.Get(2).([]string),
			args.Get(3).(bool),
			args.Get(4).(io.Reader),
			args.Get(5).(io.Writer),
			args.Get(6).(io.Writer))
		call.ReturnArguments = mock.Arguments{exit, err}
	})
}
