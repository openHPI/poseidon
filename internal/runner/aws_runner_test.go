package runner

import (
	"context"
	"encoding/base64"
	"github.com/gorilla/websocket"
	"github.com/openHPI/poseidon/internal/config"
	"github.com/openHPI/poseidon/pkg/dto"
	"github.com/openHPI/poseidon/tests"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestAWSExecutionRequestIsStored(t *testing.T) {
	r, err := NewAWSFunctionWorkload(nil, nil)
	assert.NoError(t, err)
	executionRequest := &dto.ExecutionRequest{
		Command:     "command",
		TimeLimit:   10,
		Environment: nil,
	}
	r.StoreExecution(defaultExecutionID, executionRequest)
	assert.True(t, r.ExecutionExists(defaultExecutionID))
	storedExecutionRunner, ok := r.executions.Pop(defaultExecutionID)
	assert.True(t, ok, "Getting an execution should not return ok false")
	assert.Equal(t, executionRequest, storedExecutionRunner)
}

type awsEndpointMock struct {
	hasConnected bool
	ctx          context.Context
	receivedData string
}

func (a *awsEndpointMock) handler(w http.ResponseWriter, r *http.Request) {
	upgrader := websocket.Upgrader{}
	c, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer c.Close()

	a.hasConnected = true
	for a.ctx.Err() == nil {
		_, message, err := c.ReadMessage()
		if err != nil {
			break
		}
		a.receivedData = string(message)
	}
}

func TestAWSFunctionWorkload_ExecuteInteractively(t *testing.T) {
	environment := &ExecutionEnvironmentMock{}
	environment.On("Image").Return("testImage or AWS endpoint")
	r, err := NewAWSFunctionWorkload(environment, nil)
	require.NoError(t, err)

	var cancel context.CancelFunc
	awsMock := &awsEndpointMock{}
	s := httptest.NewServer(http.HandlerFunc(awsMock.handler))

	t.Run("establishes WebSocket connection to AWS endpoint", func(t *testing.T) {
		// Convert http://127.0.0.1 to ws://127.0.0.1
		config.Config.AWS.Endpoint = "ws" + strings.TrimPrefix(s.URL, "http")
		awsMock.ctx, cancel = context.WithCancel(context.Background())
		cancel()

		r.StoreExecution(defaultExecutionID, &dto.ExecutionRequest{})
		exit, _, err := r.ExecuteInteractively(defaultExecutionID, nil, io.Discard, io.Discard)
		require.NoError(t, err)
		<-exit
		assert.True(t, awsMock.hasConnected)
	})

	t.Run("sends execution request", func(t *testing.T) {
		awsMock.ctx, cancel = context.WithTimeout(context.Background(), tests.ShortTimeout)
		defer cancel()
		command := "sl"
		request := &dto.ExecutionRequest{Command: command}
		r.StoreExecution(defaultExecutionID, request)

		_, cancel, err := r.ExecuteInteractively(defaultExecutionID, nil, io.Discard, io.Discard)
		require.NoError(t, err)
		<-time.After(tests.ShortTimeout)
		cancel()

		expectedRequestData := "{\"action\":\"" + environment.Image() +
			"\",\"cmd\":[\"env\",\"CODEOCEAN=true\",\"sh\",\"-c\",\"unset \\\"${!AWS@}\\\" \\u0026\\u0026 " + command +
			"\"],\"files\":{}}"
		assert.Equal(t, expectedRequestData, awsMock.receivedData)
	})
}

func TestAWSFunctionWorkload_UpdateFileSystem(t *testing.T) {
	environment := &ExecutionEnvironmentMock{}
	environment.On("Image").Return("testImage or AWS endpoint")
	r, err := NewAWSFunctionWorkload(environment, nil)
	require.NoError(t, err)

	var cancel context.CancelFunc
	awsMock := &awsEndpointMock{}
	s := httptest.NewServer(http.HandlerFunc(awsMock.handler))

	// Convert http://127.0.0.1 to ws://127.0.0.1
	config.Config.AWS.Endpoint = "ws" + strings.TrimPrefix(s.URL, "http")
	awsMock.ctx, cancel = context.WithTimeout(context.Background(), tests.ShortTimeout)
	defer cancel()
	command := "sl"
	request := &dto.ExecutionRequest{Command: command}
	r.StoreExecution(defaultExecutionID, request)
	myFile := dto.File{Path: "myPath", Content: []byte("myContent")}

	err = r.UpdateFileSystem(&dto.UpdateFileSystemRequest{Copy: []dto.File{myFile}})
	assert.NoError(t, err)
	_, execCancel, err := r.ExecuteInteractively(defaultExecutionID, nil, io.Discard, io.Discard)
	require.NoError(t, err)
	<-time.After(tests.ShortTimeout)
	execCancel()

	expectedRequestData := "{\"action\":\"" + environment.Image() +
		"\",\"cmd\":[\"env\",\"CODEOCEAN=true\",\"sh\",\"-c\",\"unset \\\"${!AWS@}\\\" \\u0026\\u0026 " + command + "\"]," +
		"\"files\":{\"" + string(myFile.Path) + "\":\"" + base64.StdEncoding.EncodeToString(myFile.Content) + "\"}}"
	assert.Equal(t, expectedRequestData, awsMock.receivedData)
}

func TestAWSFunctionWorkload_Destroy(t *testing.T) {
	hasDestroyBeenCalled := false
	r, err := NewAWSFunctionWorkload(nil, func(_ Runner) error {
		hasDestroyBeenCalled = true
		return nil
	})
	require.NoError(t, err)

	err = r.Destroy()
	assert.NoError(t, err)
	assert.True(t, hasDestroyBeenCalled)
}
