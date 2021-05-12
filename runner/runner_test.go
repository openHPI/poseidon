package runner

import (
	"context"
	"encoding/json"
	"github.com/stretchr/testify/assert"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/api/dto"
	"testing"
)

func TestIdIsStored(t *testing.T) {
	runner := NewRunner("42")
	assert.Equal(t, "42", runner.Id())
}

func TestMarshalRunner(t *testing.T) {
	runner := NewRunner("42")
	marshal, err := json.Marshal(runner)
	assert.NoError(t, err)
	assert.Equal(t, "{\"runnerId\":\"42\"}", string(marshal))
}

func TestExecutionRequestIsStored(t *testing.T) {
	runner := NewRunner("42")
	executionRequest := dto.ExecutionRequest{
		Command:     "command",
		TimeLimit:   10,
		Environment: nil,
	}
	id, err := runner.AddExecution(executionRequest)
	storedExecutionRunner, ok := runner.Execution(id)

	assert.NoError(t, err, "AddExecution should not produce an error")
	assert.True(t, ok, "Getting an execution should not return ok false")
	assert.Equal(t, executionRequest, storedExecutionRunner)
}

func TestNewContextReturnsNewContextWithRunner(t *testing.T) {
	runner := NewRunner("testRunner")
	ctx := context.Background()
	newCtx := NewContext(ctx, runner)
	storedRunner := newCtx.Value(runnerContextKey).(Runner)

	assert.NotEqual(t, ctx, newCtx)
	assert.Equal(t, runner, storedRunner)
}

func TestFromContextReturnsRunner(t *testing.T) {
	runner := NewRunner("testRunner")
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
