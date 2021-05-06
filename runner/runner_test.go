package runner

import (
	"context"
	"encoding/json"
	"github.com/stretchr/testify/assert"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/api/dto"
	"testing"
)

func TestIdIsStored(t *testing.T) {
	runner := NewExerciseRunner("42")
	assert.Equal(t, "42", runner.Id())
}

func TestStatusIsStored(t *testing.T) {
	runner := NewExerciseRunner("42")
	for _, status := range []Status{StatusReady, StatusRunning, StatusTimeout, StatusFinished} {
		runner.SetStatus(status)
		assert.Equal(t, status, runner.Status(), "The status is returned as it is stored")
	}
}

func TestDefaultStatus(t *testing.T) {
	runner := NewExerciseRunner("42")
	assert.Equal(t, StatusReady, runner.status)
}

func TestMarshalRunner(t *testing.T) {
	runner := NewExerciseRunner("42")
	marshal, err := json.Marshal(runner)
	assert.NoError(t, err)
	assert.Equal(t, "{\"runnerId\":\"42\",\"status\":\"ready\"}", string(marshal))
}

func TestExecutionRequestIsStored(t *testing.T) {
	runner := NewExerciseRunner("42")
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
	runner := NewExerciseRunner("testRunner")
	ctx := context.Background()
	newCtx := NewContext(ctx, runner)
	storedRunner := newCtx.Value(runnerContextKey).(Runner)

	assert.NotEqual(t, ctx, newCtx)
	assert.Equal(t, runner, storedRunner)
}

func TestFromContextReturnsRunner(t *testing.T) {
	runner := NewExerciseRunner("testRunner")
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