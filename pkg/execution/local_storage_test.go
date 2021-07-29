package execution

import (
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/pkg/dto"
	"testing"
)

var (
	testExecutionRequest = dto.ExecutionRequest{
		Command:   "echo 'Hello Poseidon'",
		TimeLimit: 42,
		Environment: map[string]string{
			"PATH": "/bin",
		},
	}
	anotherTestExecutionRequest = dto.ExecutionRequest{
		Command:     "echo 'Bye Poseidon'",
		TimeLimit:   1337,
		Environment: nil,
	}
	testID = ID("test")
)

func TestNewLocalExecutionStorage(t *testing.T) {
	storage := NewLocalStorage()
	assert.Zero(t, len(storage.executions))
}

func TestLocalStorage(t *testing.T) {
	storage := NewLocalStorage()

	t.Run("cannot pop when id does not exist", func(t *testing.T) {
		request, ok := storage.Pop(testID)
		assert.False(t, ok)
		assert.Nil(t, request)
	})

	t.Run("adds execution request", func(t *testing.T) {
		storage.Add(testID, &testExecutionRequest)

		request, ok := storage.executions[testID]
		assert.Equal(t, len(storage.executions), 1)
		assert.True(t, ok)
		assert.Equal(t, testExecutionRequest, *request)
	})

	t.Run("overwrites execution request", func(t *testing.T) {
		storage.Add(testID, &anotherTestExecutionRequest)

		request, ok := storage.executions[testID]
		assert.Equal(t, len(storage.executions), 1)
		assert.True(t, ok)
		require.NotNil(t, request)
		assert.Equal(t, anotherTestExecutionRequest, *request)
	})

	t.Run("removes execution request", func(t *testing.T) {
		request, ok := storage.Pop(testID)

		assert.True(t, ok)
		require.NotNil(t, request)
		assert.Equal(t, anotherTestExecutionRequest, *request)

		request, ok = storage.executions[testID]
		assert.Nil(t, request)
		assert.False(t, ok)
	})
}
