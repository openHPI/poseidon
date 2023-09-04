package tests

import (
	"bytes"
	"context"
	"github.com/stretchr/testify/suite"
	"io"
	"os"
	"runtime"
	"runtime/pprof"
	"time"
)

// ChannelReceivesSomething waits timeout seconds for something to be received from channel ch.
// If something is received, it returns true. If the timeout expires without receiving anything, it return false.
func ChannelReceivesSomething(ch chan bool, timeout time.Duration) bool {
	select {
	case <-ch:
		return true
	case <-time.After(timeout):
		return false
	}
}

// MemoryLeakTestSuite adds an assertion for checking Goroutine leaks.
// Be aware not to overwrite the SetupTest or TearDownTest function!
type MemoryLeakTestSuite struct {
	suite.Suite
	ExpectedGoroutingIncrease int
	TestCtx                   context.Context
	testCtxCancel             context.CancelFunc
	goroutineCountBefore      int
	goroutinesBefore          *bytes.Buffer
}

func (s *MemoryLeakTestSuite) SetupTest() {
	s.goroutineCountBefore = runtime.NumGoroutine()
	s.ExpectedGoroutingIncrease = 0
	s.goroutinesBefore = &bytes.Buffer{}

	err := pprof.Lookup("goroutine").WriteTo(s.goroutinesBefore, 1)
	s.NoError(err)

	ctx, cancel := context.WithCancel(context.Background())
	s.TestCtx = ctx
	s.testCtxCancel = cancel
}

func (s *MemoryLeakTestSuite) TearDownTest() {
	s.testCtxCancel()
	runtime.Gosched()         // Flush done Goroutines
	<-time.After(TinyTimeout) // Just to make sure
	goroutinesAfter := runtime.NumGoroutine()
	s.Equal(s.goroutineCountBefore+s.ExpectedGoroutingIncrease, goroutinesAfter)

	if s.goroutineCountBefore+s.ExpectedGoroutingIncrease != goroutinesAfter {
		_, err := io.Copy(os.Stderr, s.goroutinesBefore)
		s.NoError(err)
		err = pprof.Lookup("goroutine").WriteTo(os.Stderr, 1)
		s.NoError(err)
	}
}
