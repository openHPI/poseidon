package util

import (
	"context"
	"github.com/openHPI/poseidon/pkg/dto"
	"github.com/openHPI/poseidon/tests"
	"github.com/stretchr/testify/suite"
	"testing"
	"time"
)

type MainTestSuite struct {
	tests.MemoryLeakTestSuite
}

func TestMainTestSuite(t *testing.T) {
	suite.Run(t, new(MainTestSuite))
}

func (s *MainTestSuite) TestMergeContext_Deadline() {
	ctxWithoutDeadline := context.Background()
	earlyDeadline := time.Now().Add(time.Second)
	ctxWithEarlyDeadline, cancel := context.WithDeadline(context.Background(), earlyDeadline)
	defer cancel()
	ctxWithLateDeadline, cancel := context.WithDeadline(context.Background(), time.Now().Add(time.Hour))
	defer cancel()

	ctx := NewMergeContext([]context.Context{ctxWithoutDeadline, ctxWithEarlyDeadline, ctxWithLateDeadline})
	deadline, ok := ctx.Deadline()

	s.True(ok)
	s.Equal(earlyDeadline, deadline, "The ealiest deadline is returned")
}

func (s *MainTestSuite) TestMergeContext_Done() {
	ctxWithoutDeadline := context.Background()
	ctxWithEarlyDeadline, cancel := context.WithTimeout(context.Background(), 2*tests.ShortTimeout)
	defer cancel()
	ctxWithLateDeadline, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	ctx := NewMergeContext([]context.Context{ctxWithoutDeadline, ctxWithEarlyDeadline, ctxWithLateDeadline})

	select {
	case <-ctx.Done():
		s.Fail("mergeContext is done before any of its parents")
		return
	case <-time.After(tests.ShortTimeout):
	}

	select {
	case <-ctx.Done():
	case <-time.After(3 * tests.ShortTimeout):
		s.Fail("mergeContext is not done after the earliest of its parents")
		return
	}
}

func (s *MainTestSuite) TestMergeContext_Err() {
	ctxWithoutDeadline := context.Background()
	ctxCancelled, cancel := context.WithCancel(context.Background())
	ctx := NewMergeContext([]context.Context{ctxWithoutDeadline, ctxCancelled})

	s.NoError(ctx.Err())
	cancel()
	s.Error(ctx.Err())
}

func (s *MainTestSuite) TestMergeContext_Value() {
	ctxWithAValue := context.WithValue(context.Background(), dto.ContextKey("keyA"), "valueA")
	ctxWithAnotherValue := context.WithValue(context.Background(), dto.ContextKey("keyB"), "valueB")
	ctx := NewMergeContext([]context.Context{ctxWithAValue, ctxWithAnotherValue})

	s.Equal("valueA", ctx.Value(dto.ContextKey("keyA")))
	s.Equal("valueB", ctx.Value(dto.ContextKey("keyB")))
	s.Nil(ctx.Value("keyC"))
}
