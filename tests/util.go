package tests

import (
	"bytes"
	"context"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
	"io"
	"os"
	"regexp"
	"runtime"
	"runtime/pprof"
	"strconv"
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

var numGoroutines = regexp.MustCompile(`^goroutine profile: total (\d*)\n`)

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
	s.ExpectedGoroutingIncrease = 0
	s.goroutinesBefore = &bytes.Buffer{}

	err := pprof.Lookup("goroutine").WriteTo(s.goroutinesBefore, 1)
	s.Require().NoError(err)
	match := numGoroutines.FindSubmatch(s.goroutinesBefore.Bytes())
	if match == nil {
		s.Fail("gouroutines could not be parsed: " + s.goroutinesBefore.String())
	}

	// We do not use runtime.NumGoroutine() to not create inconsistency to the Lookup.
	s.goroutineCountBefore, err = strconv.Atoi(string(match[1]))
	if err != nil {
		s.Fail("number of goroutines could not be parsed: " + err.Error())
	}

	ctx, cancel := context.WithCancel(context.Background())
	s.TestCtx = ctx
	s.testCtxCancel = cancel
}

func (s *MemoryLeakTestSuite) TearDownTest() {
	s.testCtxCancel()
	runtime.Gosched()          // Flush done Goroutines
	<-time.After(ShortTimeout) // Just to make sure
	goroutinesAfter := runtime.NumGoroutine()
	s.Equal(s.goroutineCountBefore+s.ExpectedGoroutingIncrease, goroutinesAfter)

	if s.goroutineCountBefore+s.ExpectedGoroutingIncrease != goroutinesAfter {
		_, err := io.Copy(os.Stderr, s.goroutinesBefore)
		s.NoError(err)
		err = pprof.Lookup("goroutine").WriteTo(os.Stderr, 1)
		s.NoError(err)
	}
}

func RemoveMethodFromMock(m *mock.Mock, method string) {
	for i, call := range m.ExpectedCalls {
		if call.Method == method {
			m.ExpectedCalls = append(m.ExpectedCalls[:i], m.ExpectedCalls[i+1:]...)
			return
		}
	}
}
