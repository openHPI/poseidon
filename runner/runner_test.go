package runner

import (
	"encoding/json"
	"github.com/stretchr/testify/assert"
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
