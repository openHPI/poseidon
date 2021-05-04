package pool

import (
	"github.com/stretchr/testify/assert"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/runner"
	"testing"
)

const runnerId = "s0m3-r4nd0m-1d"

func TestAddedRunnerCanBeRetrieved(t *testing.T) {
	runnerPool := NewLocalRunnerPool()
	runnerPool.AddRunner(runner.NewExerciseRunner(runnerId))
	_, ok := runnerPool.GetRunner(runnerId)
	assert.True(t, ok, "A saved runner should be retrievable")
}

func TestDeletedRunnersAreNotAccessible(t *testing.T) {
	pool := NewLocalRunnerPool()
	pool.AddRunner(runner.NewExerciseRunner(runnerId))
	pool.DeleteRunner(runnerId)
	_, ok := pool.GetRunner(runnerId)
	assert.False(t, ok, "A deleted runner should not be accessible")
}
