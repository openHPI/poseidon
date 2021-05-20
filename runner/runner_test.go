package runner

import (
	"context"
	"encoding/json"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/api/dto"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/nomad"
	"testing"
	"time"
)

func TestIdIsStored(t *testing.T) {
	runner := NewNomadAllocation("42", nil)
	assert.Equal(t, "42", runner.Id())
}

func TestMarshalRunner(t *testing.T) {
	runner := NewNomadAllocation("42", nil)
	marshal, err := json.Marshal(runner)
	assert.NoError(t, err)
	assert.Equal(t, "{\"runnerId\":\"42\"}", string(marshal))
}

func TestExecutionRequestIsStored(t *testing.T) {
	runner := NewNomadAllocation("42", nil)
	executionRequest := &dto.ExecutionRequest{
		Command:     "command",
		TimeLimit:   10,
		Environment: nil,
	}
	id := ExecutionId("test-execution")
	runner.Add(id, executionRequest)
	storedExecutionRunner, ok := runner.Pop(id)

	assert.True(t, ok, "Getting an execution should not return ok false")
	assert.Equal(t, executionRequest, storedExecutionRunner)
}

func TestNewContextReturnsNewContextWithRunner(t *testing.T) {
	runner := NewNomadAllocation("testRunner", nil)
	ctx := context.Background()
	newCtx := NewContext(ctx, runner)
	storedRunner := newCtx.Value(runnerContextKey).(Runner)

	assert.NotEqual(t, ctx, newCtx)
	assert.Equal(t, runner, storedRunner)
}

func TestFromContextReturnsRunner(t *testing.T) {
	runner := NewNomadAllocation("testRunner", nil)
	ctx := NewContext(context.Background(), runner)
	storedRunner, ok := FromContext(ctx)

	assert.True(t, ok)
	assert.Equal(t, runner, storedRunner)
}

func TestFromContextReturnsIsNotOkWhenContextHasNoRunner(t *testing.T) {
	ctx := context.Background()
	_, ok := FromContext(ctx)

	assert.False(t, ok)
}

func TestExecuteCallsAPI(t *testing.T) {
	apiMock := &nomad.ExecutorApiMock{}
	apiMock.On("ExecuteCommand", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(0, nil)
	runner := NewNomadAllocation("testRunner", apiMock)

	request := &dto.ExecutionRequest{Command: "echo 'Hello World!'"}
	runner.Execute(request, nil, nil, nil)

	<-time.After(50 * time.Millisecond)
	apiMock.AssertCalled(t, "ExecuteCommand", "testRunner", mock.Anything, request.FullCommand(), mock.Anything, mock.Anything, mock.Anything)
}

func TestExecuteReturnsAfterTimeout(t *testing.T) {
	apiMock := newApiMockWithTimeLimitHandling()
	runner := NewNomadAllocation("testRunner", apiMock)

	timeLimit := 1
	execution := &dto.ExecutionRequest{TimeLimit: timeLimit}
	exit, _ := runner.Execute(execution, nil, nil, nil)

	select {
	case <-exit:
		assert.FailNow(t, "Execute should not terminate instantly")
	case <-time.After(50 * time.Millisecond):
	}

	select {
	case <-time.After(time.Duration(timeLimit) * time.Second):
		assert.FailNow(t, "Execute should return after the time limit")
	case exitCode := <-exit:
		assert.Equal(t, uint8(0), exitCode.Code)
	}
}

func newApiMockWithTimeLimitHandling() (apiMock *nomad.ExecutorApiMock) {
	apiMock = &nomad.ExecutorApiMock{}
	apiMock.
		On("ExecuteCommand", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) {
			ctx := args.Get(1).(context.Context)
			<-ctx.Done()
		}).
		Return(0, nil)
	return
}
