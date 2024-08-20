package nomad

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/openHPI/poseidon/internal/config"
	"github.com/openHPI/poseidon/pkg/nullio"
	"github.com/openHPI/poseidon/tests"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
)

func TestExecuteCommandTestSuite(t *testing.T) {
	suite.Run(t, new(ExecuteCommandTestSuite))
}

type ExecuteCommandTestSuite struct {
	tests.MemoryLeakTestSuite
	allocationID string
	//nolint:containedctx // See #630.
	ctx            context.Context
	testCommand    string
	expectedStdout string
	expectedStderr string
	apiMock        *apiQuerierMock
	nomadAPIClient APIClient
}

func (s *ExecuteCommandTestSuite) SetupTest() {
	s.MemoryLeakTestSuite.SetupTest()
	s.allocationID = "test-allocation-id"
	s.ctx = context.Background()
	s.testCommand = "echo \"do nothing\""
	s.expectedStdout = "stdout"
	s.expectedStderr = "stderr"
	s.apiMock = &apiQuerierMock{}
	s.nomadAPIClient = APIClient{apiQuerier: s.apiMock}
}

const withTTY = true

func (s *ExecuteCommandTestSuite) TestWithSeparateStderr() {
	config.Config.Server.InteractiveStderr = true
	commandExitCode := 42
	stderrExitCode := 1

	var stdout, stderr bytes.Buffer
	var calledStdoutCommand, calledStderrCommand string
	runFn := func(args mock.Arguments) {
		var ok bool
		calledCommand, ok := args.Get(2).(string)
		s.Require().True(ok)
		var out string
		if isStderrCommand := strings.Contains(calledCommand, "mkfifo"); isStderrCommand {
			calledStderrCommand = calledCommand
			out = s.expectedStderr
		} else {
			calledStdoutCommand = calledCommand
			out = s.expectedStdout
		}

		writer, ok := args.Get(5).(io.Writer)
		s.Require().True(ok)
		_, err := writer.Write([]byte(out))
		s.Require().NoError(err)
	}

	s.apiMock.On("Execute", mock.Anything, s.allocationID, mock.Anything, withTTY,
		mock.AnythingOfType("nullio.Reader"), mock.Anything, mock.Anything).Run(runFn).Return(stderrExitCode, nil)
	s.apiMock.On("Execute", mock.Anything, s.allocationID, mock.Anything, withTTY,
		mock.AnythingOfType("*bytes.Buffer"), mock.Anything, mock.Anything).Run(runFn).Return(commandExitCode, nil)

	exitCode, err := s.nomadAPIClient.ExecuteCommand(s.ctx, s.allocationID, s.testCommand, withTTY,
		UnprivilegedExecution, &bytes.Buffer{}, &stdout, &stderr)
	s.Require().NoError(err)

	s.apiMock.AssertNumberOfCalls(s.T(), "Execute", 2)
	s.Equal(commandExitCode, exitCode)

	s.Run("should wrap command in stderr wrapper", func() {
		s.Require().NotEmpty(calledStdoutCommand)
		stderrWrapperCommand := fmt.Sprintf(stderrWrapperCommandFormat, stderrFifoFormat, s.testCommand, stderrFifoFormat)
		stdoutFifoRegexp := strings.ReplaceAll(regexp.QuoteMeta(stderrWrapperCommand), "%d", "\\d*")
		stdoutFifoRegexp = strings.Replace(stdoutFifoRegexp, s.testCommand, ".*", 1)
		s.Regexp(stdoutFifoRegexp, calledStdoutCommand)
	})

	s.Run("should call correct stderr command", func() {
		s.Require().NotEmpty(calledStderrCommand)
		stderrFifoCommand := fmt.Sprintf(stderrFifoCommandFormat, stderrFifoFormat, stderrFifoFormat, stderrFifoFormat)
		stderrFifoRegexp := strings.ReplaceAll(regexp.QuoteMeta(stderrFifoCommand), "%d", "\\d*")
		s.Regexp(stderrFifoRegexp, calledStderrCommand)
	})

	s.Run("should return correct output", func() {
		s.Equal(s.expectedStdout, stdout.String())
		s.Equal(s.expectedStderr, stderr.String())
	})
}

func (s *ExecuteCommandTestSuite) TestWithSeparateStderrReturnsCommandError() {
	config.Config.Server.InteractiveStderr = true

	call := s.mockExecute(mock.AnythingOfType("string"), 0, nil, func(_ mock.Arguments) {})
	call.Run(func(args mock.Arguments) {
		var ok bool
		calledCommand, ok := args.Get(2).(string)
		s.Require().True(ok)

		if isStderrCommand := strings.Contains(calledCommand, "mkfifo"); isStderrCommand {
			// Here we defuse the data race condition of the ReturnArguments being set twice at the same time.
			<-time.After(tests.ShortTimeout)
			call.ReturnArguments = mock.Arguments{1, nil}
		} else {
			call.ReturnArguments = mock.Arguments{1, tests.ErrDefault}
		}
	})

	_, err := s.nomadAPIClient.ExecuteCommand(s.ctx, s.allocationID, s.testCommand, withTTY, UnprivilegedExecution,
		nullio.Reader{}, io.Discard, io.Discard)
	s.Equal(tests.ErrDefault, err)
}

func (s *ExecuteCommandTestSuite) TestWithoutSeparateStderr() {
	config.Config.Server.InteractiveStderr = false
	var stdout, stderr bytes.Buffer
	commandExitCode := 42

	// mock regular call
	expectedCommand := prepareCommandWithoutTTY(s.testCommand, UnprivilegedExecution)
	s.mockExecute(expectedCommand, commandExitCode, nil, func(args mock.Arguments) {
		stdout, ok := args.Get(5).(io.Writer)
		s.Require().True(ok)
		_, err := stdout.Write([]byte(s.expectedStdout))
		s.Require().NoError(err)
		stderr, ok := args.Get(6).(io.Writer)
		s.Require().True(ok)
		_, err = stderr.Write([]byte(s.expectedStderr))
		s.Require().NoError(err)
	})

	exitCode, err := s.nomadAPIClient.ExecuteCommand(s.ctx, s.allocationID, s.testCommand, withTTY,
		UnprivilegedExecution, nullio.Reader{}, &stdout, &stderr)
	s.Require().NoError(err)

	s.apiMock.AssertNumberOfCalls(s.T(), "Execute", 1)
	s.Equal(commandExitCode, exitCode)
	s.Equal(s.expectedStdout, stdout.String())
	s.Equal(s.expectedStderr, stderr.String())
}

func (s *ExecuteCommandTestSuite) TestWithoutSeparateStderrReturnsCommandError() {
	config.Config.Server.InteractiveStderr = false
	expectedCommand := prepareCommandWithoutTTY(s.testCommand, UnprivilegedExecution)
	s.mockExecute(expectedCommand, 1, tests.ErrDefault, func(_ mock.Arguments) {})
	_, err := s.nomadAPIClient.ExecuteCommand(s.ctx, s.allocationID, s.testCommand, withTTY, UnprivilegedExecution,
		nullio.Reader{}, io.Discard, io.Discard)
	s.ErrorIs(err, tests.ErrDefault)
}

func (s *ExecuteCommandTestSuite) mockExecute(command interface{}, exitCode int,
	err error, runFunc func(arguments mock.Arguments),
) *mock.Call {
	return s.apiMock.On("Execute", mock.Anything, s.allocationID, command, withTTY,
		mock.Anything, mock.Anything, mock.Anything).
		Run(runFunc).
		Return(exitCode, err)
}
