package runner

import (
	"context"
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/openHPI/poseidon/internal/config"
	"github.com/openHPI/poseidon/pkg/dto"
	"github.com/openHPI/poseidon/tests"
)

func (s *MainTestSuite) TestAWSExecutionRequestIsStored() {
	environment := &ExecutionEnvironmentMock{}
	environment.On("ID").Return(dto.EnvironmentID(tests.DefaultEnvironmentIDAsInteger))
	runner, err := NewAWSFunctionWorkload(environment, func(_ Runner) error { return nil })
	s.Require().NoError(err)
	executionRequest := &dto.ExecutionRequest{
		Command:     "command",
		TimeLimit:   10,
		Environment: nil,
	}
	runner.StoreExecution(tests.DefaultEnvironmentIDAsString, executionRequest)
	s.True(runner.ExecutionExists(tests.DefaultEnvironmentIDAsString))
	storedExecutionRunner, ok := runner.executions.Pop(tests.DefaultEnvironmentIDAsString)
	s.True(ok, "Getting an execution should not return ok false")
	s.Equal(executionRequest, storedExecutionRunner)

	err = runner.Destroy(nil)
	s.Require().NoError(err)
}

type awsEndpointMock struct {
	hasConnected bool
	//nolint:containedctx // See #630.
	ctx          context.Context
	receivedData string
}

func (a *awsEndpointMock) handler(w http.ResponseWriter, r *http.Request) {
	upgrader := websocket.Upgrader{}
	connection, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer connection.Close()

	a.hasConnected = true
	for a.ctx.Err() == nil {
		_, message, err := connection.ReadMessage()
		if err != nil {
			break
		}
		a.receivedData = string(message)
	}
}

func (s *MainTestSuite) TestAWSFunctionWorkload_ExecuteInteractively() {
	environment := &ExecutionEnvironmentMock{}
	environment.On("ID").Return(dto.EnvironmentID(tests.DefaultEnvironmentIDAsInteger))
	environment.On("Image").Return("testImage or AWS endpoint")
	runnerWorkload, err := NewAWSFunctionWorkload(environment, func(_ Runner) error { return nil })
	s.Require().NoError(err)

	var cancel context.CancelFunc
	awsMock := &awsEndpointMock{}
	sv := httptest.NewServer(http.HandlerFunc(awsMock.handler))
	defer sv.Close()

	s.Run("establishes WebSocket connection to AWS endpoint", func() {
		// Convert http://127.0.0.1 to ws://127.0.0.1
		config.Config.AWS.Endpoint = "ws" + strings.TrimPrefix(sv.URL, "http")
		awsMock.ctx, cancel = context.WithCancel(context.Background()) //nolint:fatcontext // We are resetting the context not making it bigger.
		cancel()

		runnerWorkload.StoreExecution(tests.DefaultEnvironmentIDAsString, &dto.ExecutionRequest{})
		exit, _, err := runnerWorkload.ExecuteInteractively(
			s.TestCtx, tests.DefaultEnvironmentIDAsString, nil, io.Discard, io.Discard)
		s.Require().NoError(err)
		<-exit
		s.True(awsMock.hasConnected)
	})

	s.Run("sends execution request", func() {
		s.T().Skip("The AWS runner ignores its context for executions and waits infinitely for the exit message.")
		awsMock.ctx, cancel = context.WithTimeout(context.Background(), tests.ShortTimeout) //nolint:fatcontext // We are not making the context bigger.
		defer cancel()
		command := "sl"
		request := &dto.ExecutionRequest{Command: command}
		runnerWorkload.StoreExecution(tests.DefaultEnvironmentIDAsString, request)

		_, cancel, err := runnerWorkload.ExecuteInteractively(
			s.TestCtx, tests.DefaultEnvironmentIDAsString, nil, io.Discard, io.Discard)
		s.Require().NoError(err)
		<-time.After(tests.ShortTimeout)
		cancel()

		expectedRequestData := `{"action":"` + environment.Image() +
			`","cmd":["/bin/bash","-c","env CODEOCEAN=true /bin/bash -c \"unset \\\"\\${!AWS@}\\\" \u0026\u0026 ` + command +
			`\""],"files":{}}`
		s.Equal(expectedRequestData, awsMock.receivedData)
	})

	err = runnerWorkload.Destroy(nil)
	s.Require().NoError(err)
}

func (s *MainTestSuite) TestAWSFunctionWorkload_UpdateFileSystem() {
	s.T().Skip("The AWS runner ignores its context for executions and waits infinitely for the exit message.")

	environment := &ExecutionEnvironmentMock{}
	environment.On("ID").Return(dto.EnvironmentID(tests.DefaultEnvironmentIDAsInteger))
	environment.On("Image").Return("testImage or AWS endpoint")
	runnerWorkload, err := NewAWSFunctionWorkload(environment, nil)
	s.Require().NoError(err)

	var cancel context.CancelFunc
	awsMock := &awsEndpointMock{}
	sv := httptest.NewServer(http.HandlerFunc(awsMock.handler))
	defer sv.Close()

	// Convert http://127.0.0.1 to ws://127.0.0.1
	config.Config.AWS.Endpoint = "ws" + strings.TrimPrefix(sv.URL, "http")
	awsMock.ctx, cancel = context.WithTimeout(context.Background(), tests.ShortTimeout)
	defer cancel()
	command := "sl"
	request := &dto.ExecutionRequest{Command: command}
	runnerWorkload.StoreExecution(tests.DefaultEnvironmentIDAsString, request)
	myFile := dto.File{Path: "myPath", Content: []byte("myContent")}

	err = runnerWorkload.UpdateFileSystem(s.TestCtx, &dto.UpdateFileSystemRequest{Copy: []dto.File{myFile}})
	s.Require().NoError(err)
	_, execCancel, err := runnerWorkload.ExecuteInteractively(
		s.TestCtx, tests.DefaultEnvironmentIDAsString, nil, io.Discard, io.Discard)
	s.Require().NoError(err)
	<-time.After(tests.ShortTimeout)
	execCancel()

	expectedRequestData := `{"action":"` + environment.Image() +
		`","cmd":["/bin/bash","-c","env CODEOCEAN=true /bin/bash -c \"unset \\\"\\${!AWS@}\\\" \u0026\u0026 ` + command +
		`\""],"files":{"` + string(myFile.Path) + `":"` + base64.StdEncoding.EncodeToString(myFile.Content) + `"}}`
	s.Equal(expectedRequestData, awsMock.receivedData)

	err = runnerWorkload.Destroy(nil)
	s.Require().NoError(err)
}

func (s *MainTestSuite) TestAWSFunctionWorkload_Destroy() {
	environment := &ExecutionEnvironmentMock{}
	environment.On("ID").Return(dto.EnvironmentID(tests.DefaultEnvironmentIDAsInteger))
	hasDestroyBeenCalled := false
	runnerWorkload, err := NewAWSFunctionWorkload(environment, func(_ Runner) error {
		hasDestroyBeenCalled = true
		return nil
	})
	s.Require().NoError(err)

	var reason error
	err = runnerWorkload.Destroy(reason)
	s.Require().NoError(err)
	s.True(hasDestroyBeenCalled)
}
