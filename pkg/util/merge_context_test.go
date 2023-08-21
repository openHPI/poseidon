package util

import (
	"context"
	"github.com/openHPI/poseidon/pkg/dto"
	"github.com/openHPI/poseidon/tests"
	"github.com/stretchr/testify/assert"
	"testing"
	"time"
)

func TestMergeContext_Deadline(t *testing.T) {
	ctxWithoutDeadline := context.Background()
	earlyDeadline := time.Now().Add(time.Second)
	ctxWithEarlyDeadline, cancel := context.WithDeadline(context.Background(), earlyDeadline)
	defer cancel()
	ctxWithLateDeadline, cancel := context.WithDeadline(context.Background(), time.Now().Add(time.Hour))
	defer cancel()

	ctx := NewMergeContext([]context.Context{ctxWithoutDeadline, ctxWithEarlyDeadline, ctxWithLateDeadline})
	deadline, ok := ctx.Deadline()

	assert.True(t, ok)
	assert.Equal(t, earlyDeadline, deadline, "The ealiest deadline is returned")
}

func TestMergeContext_Done(t *testing.T) {
	ctxWithoutDeadline := context.Background()
	ctxWithEarlyDeadline, cancel := context.WithTimeout(context.Background(), 2*tests.ShortTimeout)
	defer cancel()
	ctxWithLateDeadline, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	ctx := NewMergeContext([]context.Context{ctxWithoutDeadline, ctxWithEarlyDeadline, ctxWithLateDeadline})

	select {
	case <-ctx.Done():
		assert.Fail(t, "mergeContext is done before any of its parents")
		return
	case <-time.After(tests.ShortTimeout):
	}

	select {
	case <-ctx.Done():
	case <-time.After(3 * tests.ShortTimeout):
		assert.Fail(t, "mergeContext is not done after the earliest of its parents")
		return
	}
}

func TestMergeContext_Err(t *testing.T) {
	ctxWithoutDeadline := context.Background()
	ctxCancelled, cancel := context.WithCancel(context.Background())
	ctx := NewMergeContext([]context.Context{ctxWithoutDeadline, ctxCancelled})

	assert.NoError(t, ctx.Err())
	cancel()
	assert.Error(t, ctx.Err())
}

func TestMergeContext_Value(t *testing.T) {
	ctxWithAValue := context.WithValue(context.Background(), dto.ContextKey("keyA"), "valueA")
	ctxWithAnotherValue := context.WithValue(context.Background(), dto.ContextKey("keyB"), "valueB")
	ctx := NewMergeContext([]context.Context{ctxWithAValue, ctxWithAnotherValue})

	assert.Equal(t, "valueA", ctx.Value(dto.ContextKey("keyA")))
	assert.Equal(t, "valueB", ctx.Value(dto.ContextKey("keyB")))
	assert.Nil(t, ctx.Value("keyC"))
}
