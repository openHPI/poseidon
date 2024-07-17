package tests

import (
	"bytes"
	"context"
	"io"
	"os"
	"regexp"
	"runtime"
	"runtime/pprof"
	"strconv"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
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
	ExpectedGoroutineIncrease int
	//nolint:containedctx // We have to embed the context into the struct because we have no control over the parameters of testify.
	TestCtx              context.Context
	testCtxCancel        context.CancelFunc
	goroutineCountBefore int
	goroutinesBefore     *bytes.Buffer
}

func (s *MemoryLeakTestSuite) lookupGoroutines() (debugOutput *bytes.Buffer, goroutineCount int) {
	debugOutput = &bytes.Buffer{}
	err := pprof.Lookup("goroutine").WriteTo(debugOutput, 1)
	s.Require().NoError(err)
	match := numGoroutines.FindSubmatch(debugOutput.Bytes())
	if match == nil {
		s.Fail("gouroutines could not be parsed: " + debugOutput.String())
	}

	// We do not use runtime.NumGoroutine() to not create inconsistency to the Lookup.
	goroutineCount, err = strconv.Atoi(string(match[1]))
	if err != nil {
		s.Fail("number of goroutines could not be parsed: " + err.Error())
	}
	return debugOutput, goroutineCount
}

func (s *MemoryLeakTestSuite) SetupTest() {
	runtime.Gosched()          // Flush done Goroutines
	<-time.After(ShortTimeout) // Just to make sure
	s.ExpectedGoroutineIncrease = 0
	s.goroutinesBefore, s.goroutineCountBefore = s.lookupGoroutines()

	ctx, cancel := context.WithCancel(context.Background())
	s.TestCtx = ctx
	s.testCtxCancel = cancel
}

func (s *MemoryLeakTestSuite) TearDownTest() {
	s.testCtxCancel()
	runtime.Gosched()          // Flush done Goroutines
	<-time.After(ShortTimeout) // Just to make sure

	goroutinesAfter, goroutineCountAfter := s.lookupGoroutines()
	s.Equal(s.goroutineCountBefore+s.ExpectedGoroutineIncrease, goroutineCountAfter)
	if s.goroutineCountBefore+s.ExpectedGoroutineIncrease != goroutineCountAfter {
		_, err := io.Copy(os.Stderr, s.goroutinesBefore)
		s.NoError(err)
		_, err = io.Copy(os.Stderr, goroutinesAfter)
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
