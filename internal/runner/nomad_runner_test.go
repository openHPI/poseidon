package runner

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/openHPI/poseidon/internal/nomad"
	"github.com/openHPI/poseidon/pkg/dto"
	"github.com/openHPI/poseidon/pkg/logging"
	"github.com/openHPI/poseidon/pkg/nullio"
	"github.com/openHPI/poseidon/pkg/storage"
	"github.com/openHPI/poseidon/tests"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
)

const defaultExecutionID = "execution-id"

func (s *MainTestSuite) TestIdIsStored() {
	apiMock := &nomad.ExecutorAPIMock{}
	apiMock.On("DeleteJob", mock.AnythingOfType("string")).Return(nil)
	runner := NewNomadJob(s.TestCtx, tests.DefaultRunnerID, nil, apiMock, func(_ Runner) error { return nil })
	s.Equal(tests.DefaultRunnerID, runner.ID())
	s.Require().NoError(runner.Destroy(nil))
}

func (s *MainTestSuite) TestMappedPortsAreStoredCorrectly() {
	apiMock := &nomad.ExecutorAPIMock{}
	apiMock.On("DeleteJob", mock.AnythingOfType("string")).Return(nil)

	runner := NewNomadJob(s.TestCtx, tests.DefaultRunnerID, tests.DefaultPortMappings, apiMock, func(_ Runner) error { return nil })
	s.Equal(tests.DefaultMappedPorts, runner.MappedPorts())
	s.Require().NoError(runner.Destroy(nil))

	runner = NewNomadJob(s.TestCtx, tests.DefaultRunnerID, nil, apiMock, func(_ Runner) error { return nil })
	s.Empty(runner.MappedPorts())
	s.Require().NoError(runner.Destroy(nil))
}

func (s *MainTestSuite) TestMarshalRunner() {
	apiMock := &nomad.ExecutorAPIMock{}
	apiMock.On("DeleteJob", mock.AnythingOfType("string")).Return(nil)
	runner := NewNomadJob(s.TestCtx, tests.DefaultRunnerID, nil, apiMock, func(_ Runner) error { return nil })
	marshal, err := json.Marshal(runner)
	s.Require().NoError(err)
	s.Equal("{\"runnerId\":\""+tests.DefaultRunnerID+"\"}", string(marshal))
	s.Require().NoError(runner.Destroy(nil))
}

func (s *MainTestSuite) TestExecutionRequestIsStored() {
	apiMock := &nomad.ExecutorAPIMock{}
	apiMock.On("DeleteJob", mock.AnythingOfType("string")).Return(nil)
	runner := NewNomadJob(s.TestCtx, tests.DefaultRunnerID, nil, apiMock, func(_ Runner) error { return nil })
	executionRequest := &dto.ExecutionRequest{
		Command:     "command",
		TimeLimit:   10,
		Environment: nil,
	}
	id := "test-execution"
	runner.StoreExecution(id, executionRequest)
	storedExecutionRunner, ok := runner.executions.Pop(id)

	s.True(ok, "Getting an execution should not return ok false")
	s.Equal(executionRequest, storedExecutionRunner)
	s.Require().NoError(runner.Destroy(nil))
}

func (s *MainTestSuite) TestNewContextReturnsNewContextWithRunner() {
	apiMock := &nomad.ExecutorAPIMock{}
	apiMock.On("DeleteJob", mock.AnythingOfType("string")).Return(nil)
	runner := NewNomadJob(s.TestCtx, tests.DefaultRunnerID, nil, apiMock, func(_ Runner) error { return nil })
	ctx := context.Background()
	newCtx := NewContext(ctx, runner)
	storedRunner, ok := newCtx.Value(runnerContextKey).(Runner)
	s.Require().True(ok)

	s.NotEqual(ctx, newCtx)
	s.Equal(runner, storedRunner)
	s.Require().NoError(runner.Destroy(nil))
}

func (s *MainTestSuite) TestFromContextReturnsRunner() {
	apiMock := &nomad.ExecutorAPIMock{}
	apiMock.On("DeleteJob", mock.AnythingOfType("string")).Return(nil)
	runner := NewNomadJob(s.TestCtx, tests.DefaultRunnerID, nil, apiMock, func(_ Runner) error { return nil })
	ctx := NewContext(context.Background(), runner)
	storedRunner, ok := FromContext(ctx)

	s.True(ok)
	s.Equal(runner, storedRunner)
	s.Require().NoError(runner.Destroy(nil))
}

func (s *MainTestSuite) TestFromContextReturnsIsNotOkWhenContextHasNoRunner() {
	ctx := context.Background()
	_, ok := FromContext(ctx)

	s.False(ok)
}

func (s *MainTestSuite) TestDestroyDoesNotPropagateToNomadForSomeReasons() {
	apiMock := &nomad.ExecutorAPIMock{}
	timer := &InactivityTimerMock{}
	timer.On("StopTimeout").Return()
	ctx, cancel := context.WithCancel(s.TestCtx)
	r := &NomadJob{
		executions:      storage.NewLocalStorage[*dto.ExecutionRequest](),
		InactivityTimer: timer,
		id:              tests.DefaultRunnerID,
		api:             apiMock,
		ctx:             ctx,
		cancel:          cancel,
	}

	s.Run("destroy removes the runner only locally for OOM Killed Allocations", func() {
		err := r.Destroy(ErrOOMKilled)
		s.Require().NoError(err)
		apiMock.AssertExpectations(s.T())
	})

	s.Run("destroy removes the runner only locally for rescheduled allocations", func() {
		err := r.Destroy(nomad.ErrAllocationRescheduled)
		s.Require().NoError(err)
		apiMock.AssertExpectations(s.T())
	})
}

func TestExecuteInteractivelyTestSuite(t *testing.T) {
	suite.Run(t, new(ExecuteInteractivelyTestSuite))
}

type ExecuteInteractivelyTestSuite struct {
	tests.MemoryLeakTestSuite
	runner                   *NomadJob
	apiMock                  *nomad.ExecutorAPIMock
	timer                    *InactivityTimerMock
	manager                  *ManagerMock
	mockedExecuteCommandCall *mock.Call
	mockedTimeoutPassedCall  *mock.Call
}

func (s *ExecuteInteractivelyTestSuite) SetupTest() {
	s.MemoryLeakTestSuite.SetupTest()
	s.apiMock = &nomad.ExecutorAPIMock{}
	s.mockedExecuteCommandCall = s.apiMock.On("ExecuteCommand", mock.Anything, mock.Anything, mock.Anything,
		true, false, mock.Anything, mock.Anything, mock.Anything).
		Return(0, nil)
	s.apiMock.On("DeleteJob", mock.AnythingOfType("string")).Return(nil)
	s.timer = &InactivityTimerMock{}
	s.timer.On("StopTimeout").Return()
	s.timer.On("ResetTimeout").Return()
	s.mockedTimeoutPassedCall = s.timer.On("TimeoutPassed").Return(false)
	s.manager = &ManagerMock{}
	s.manager.On("Return", mock.Anything).Return(nil)

	ctx, cancel := context.WithCancel(context.Background())
	s.runner = &NomadJob{
		executions:      storage.NewLocalStorage[*dto.ExecutionRequest](),
		InactivityTimer: s.timer,
		id:              tests.DefaultRunnerID,
		api:             s.apiMock,
		ctx:             ctx,
		cancel:          cancel,
	}
}

func (s *ExecuteInteractivelyTestSuite) TestReturnsErrorWhenExecutionDoesNotExist() {
	_, _, err := s.runner.ExecuteInteractively(context.Background(), "non-existent-id", nil, nil, nil)
	s.ErrorIs(err, ErrUnknownExecution)
}

func (s *ExecuteInteractivelyTestSuite) TestCallsApi() {
	request := &dto.ExecutionRequest{Command: "echo 'Hello World!'"}
	s.runner.StoreExecution(defaultExecutionID, request)
	_, _, err := s.runner.ExecuteInteractively(s.TestCtx, defaultExecutionID, nil, nil, nil)
	s.Require().NoError(err)

	time.Sleep(tests.ShortTimeout)
	s.apiMock.AssertCalled(s.T(), "ExecuteCommand", mock.Anything, tests.DefaultRunnerID, request.FullCommand(),
		true, false, mock.Anything, mock.Anything, mock.Anything)
}

func (s *ExecuteInteractivelyTestSuite) TestReturnsAfterTimeout() {
	hangingRequest, cancel := context.WithCancel(s.TestCtx)
	s.mockedExecuteCommandCall.Run(func(_ mock.Arguments) {
		<-hangingRequest.Done()
	}).Return(0, nil)

	timeLimit := 1
	executionRequest := &dto.ExecutionRequest{TimeLimit: timeLimit}
	s.runner.StoreExecution(defaultExecutionID, executionRequest)
	exit, _, err := s.runner.ExecuteInteractively(s.TestCtx, defaultExecutionID, &nullio.ReadWriter{}, nil, nil)
	s.Require().NoError(err)

	select {
	case <-exit:
		s.FailNow("ExecuteInteractively should not terminate instantly")
	case <-time.After(tests.ShortTimeout):
	}

	select {
	case <-time.After(time.Duration(timeLimit) * time.Second):
		s.FailNow("ExecuteInteractively should return after the time limit")
	case exitInfo := <-exit:
		s.Equal(uint8(255), exitInfo.Code)
	}

	cancel()
	<-time.After(tests.ShortTimeout)
}

func (s *ExecuteInteractivelyTestSuite) TestSendsSignalAfterTimeout() {
	quit := make(chan struct{})
	s.mockedExecuteCommandCall.Run(func(args mock.Arguments) {
		stdin, ok := args.Get(5).(io.Reader)
		s.Require().True(ok)
		buffer := make([]byte, 1) //nolint:makezero // If the length is zero, the Read call never reads anything. gofmt want this alignment.
		for n := 0; !(n == 1 && buffer[0] == SIGQUIT); {
			<-time.After(tests.ShortTimeout)
			n, _ = stdin.Read(buffer) //nolint:errcheck // Read returns EOF errors but that is expected.
			if n > 0 {
				log.WithField("buffer", strconv.FormatUint(uint64(buffer[0]), 16)).Info("Received Stdin")
			}
		}
		log.Info("After loop")
		close(quit)
	}).Return(0, nil)
	timeLimit := 1
	executionRequest := &dto.ExecutionRequest{TimeLimit: timeLimit}
	s.runner.StoreExecution(defaultExecutionID, executionRequest)
	_, _, err := s.runner.ExecuteInteractively(
		s.TestCtx, defaultExecutionID, bytes.NewBuffer(make([]byte, 1)), nil, nil)
	s.Require().NoError(err)
	log.Info("Before waiting")
	select {
	case <-time.After(2 * (time.Duration(timeLimit) * time.Second)):
		s.FailNow("The execution should receive a SIGQUIT after the timeout")
	case <-quit:
		log.Info("Received quit")
	}
}

func (s *ExecuteInteractivelyTestSuite) TestDestroysRunnerAfterTimeoutAndSignal() {
	s.mockedExecuteCommandCall.Run(func(_ mock.Arguments) {
		<-s.TestCtx.Done()
	})
	runnerDestroyed := false
	s.runner.onDestroy = func(_ Runner) error {
		runnerDestroyed = true
		return nil
	}
	timeLimit := 1
	executionRequest := &dto.ExecutionRequest{TimeLimit: timeLimit}
	s.runner.cancel = func() {}
	s.runner.StoreExecution(defaultExecutionID, executionRequest)

	_, _, err := s.runner.ExecuteInteractively(
		s.TestCtx, defaultExecutionID, bytes.NewBuffer(make([]byte, 1)), nil, nil)
	s.Require().NoError(err)

	<-time.After(executionTimeoutGracePeriod + time.Duration(timeLimit)*time.Second)
	// Even if we expect the timeout to be exceeded now, Poseidon sometimes take a couple of hundred ms longer.
	<-time.After(2 * tests.ShortTimeout)
	s.manager.AssertNotCalled(s.T(), "Return", s.runner)
	s.apiMock.AssertCalled(s.T(), "DeleteJob", s.runner.ID())
	s.True(runnerDestroyed)
}

func (s *ExecuteInteractivelyTestSuite) TestResetTimerGetsCalled() {
	executionRequest := &dto.ExecutionRequest{}
	s.runner.StoreExecution(defaultExecutionID, executionRequest)
	exit, _, err := s.runner.ExecuteInteractively(s.TestCtx, defaultExecutionID, nil, nil, nil)
	s.Require().NoError(err)

	select {
	case <-exit:
	case <-time.After(tests.ShortTimeout):
		s.Fail("ExecuteInteractively did not quit instantly as expected")
	}
	s.timer.AssertCalled(s.T(), "ResetTimeout")
}

func (s *ExecuteInteractivelyTestSuite) TestExitHasTimeoutErrorIfRunnerTimesOut() {
	hangingRequest, cancel := context.WithCancel(s.TestCtx)
	s.mockedExecuteCommandCall.Run(func(_ mock.Arguments) {
		<-hangingRequest.Done()
	}).Return(0, nil)
	s.mockedTimeoutPassedCall.Return(true)
	executionRequest := &dto.ExecutionRequest{}
	s.runner.StoreExecution(defaultExecutionID, executionRequest)

	exitChannel, _, err := s.runner.ExecuteInteractively(
		s.TestCtx, defaultExecutionID, &nullio.ReadWriter{}, nil, nil)
	s.Require().NoError(err)
	err = s.runner.Destroy(ErrRunnerInactivityTimeout)
	s.Require().NoError(err)
	exit := <-exitChannel
	s.Require().ErrorIs(exit.Err, ErrRunnerInactivityTimeout)

	cancel()
	<-time.After(tests.ShortTimeout)
}

func (s *ExecuteInteractivelyTestSuite) TestDestroyReasonIsPassedToExecution() {
	s.mockedExecuteCommandCall.Run(func(_ mock.Arguments) {
		<-s.TestCtx.Done()
	}).Return(0, nil)
	s.mockedTimeoutPassedCall.Return(true)
	executionRequest := &dto.ExecutionRequest{}
	s.runner.StoreExecution(defaultExecutionID, executionRequest)

	exitChannel, _, err := s.runner.ExecuteInteractively(
		s.TestCtx, defaultExecutionID, &nullio.ReadWriter{}, nil, nil)
	s.Require().NoError(err)
	err = s.runner.Destroy(ErrOOMKilled)
	s.Require().NoError(err)
	exit := <-exitChannel
	s.ErrorIs(exit.Err, ErrOOMKilled)
}

func (s *ExecuteInteractivelyTestSuite) TestSuspectedOOMKilledExecutionWaitsForVerification() {
	s.mockedExecuteCommandCall.Return(128, nil)
	executionRequest := &dto.ExecutionRequest{}
	s.Run("Actually OOM Killed", func() {
		s.runner.StoreExecution(defaultExecutionID, executionRequest)
		exitChannel, _, err := s.runner.ExecuteInteractively(
			s.TestCtx, defaultExecutionID, &nullio.ReadWriter{}, nil, nil)
		s.Require().NoError(err)

		select {
		case <-exitChannel:
			s.FailNow("For exit code 128 Poseidon should wait a while to verify the OOM Kill assumption.")
		case <-time.After(tests.ShortTimeout):
			// All good. Poseidon waited.
		}

		err = s.runner.Destroy(ErrOOMKilled)
		s.Require().NoError(err)
		exit := <-exitChannel
		s.ErrorIs(exit.Err, ErrOOMKilled)
	})

	ctx, cancel := context.WithCancel(context.Background())
	s.runner.ctx = ctx
	s.runner.cancel = cancel
	s.Run("Not OOM Killed", func() {
		s.runner.StoreExecution(defaultExecutionID, executionRequest)
		exitChannel, _, err := s.runner.ExecuteInteractively(
			s.TestCtx, defaultExecutionID, &nullio.ReadWriter{}, nil, nil)
		s.Require().NoError(err)

		select {
		case <-time.After(tests.ShortTimeout + time.Second):
			s.FailNow("Poseidon should not wait too long for verifying the OOM Kill assumption.")
		case exit := <-exitChannel:
			s.Equal(uint8(128), exit.Code)
			s.Require().NoError(exit.Err)
		}
	})
}

func TestUpdateFileSystemTestSuite(t *testing.T) {
	suite.Run(t, new(UpdateFileSystemTestSuite))
}

type UpdateFileSystemTestSuite struct {
	tests.MemoryLeakTestSuite
	runner                   *NomadJob
	timer                    *InactivityTimerMock
	apiMock                  *nomad.ExecutorAPIMock
	mockedExecuteCommandCall *mock.Call
	command                  string
	stdin                    *bytes.Buffer
}

func (s *UpdateFileSystemTestSuite) SetupTest() {
	s.MemoryLeakTestSuite.SetupTest()
	s.apiMock = &nomad.ExecutorAPIMock{}
	s.timer = &InactivityTimerMock{}
	s.timer.On("ResetTimeout").Return()
	s.timer.On("TimeoutPassed").Return(false)
	s.runner = &NomadJob{
		executions:      storage.NewLocalStorage[*dto.ExecutionRequest](),
		InactivityTimer: s.timer,
		id:              tests.DefaultRunnerID,
		api:             s.apiMock,
	}
	s.mockedExecuteCommandCall = s.apiMock.On("ExecuteCommand", mock.Anything, tests.DefaultRunnerID,
		mock.Anything, false, mock.AnythingOfType("bool"), mock.Anything, mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) {
			var ok bool
			s.command, ok = args.Get(2).(string)
			s.Require().True(ok)
			s.stdin, ok = args.Get(5).(*bytes.Buffer)
			s.Require().True(ok)
		}).Return(0, nil)
}

func (s *UpdateFileSystemTestSuite) TestUpdateFileSystemForRunnerPerformsTarExtractionWithAbsoluteNamesOnRunner() {
	// note: this method tests an implementation detail of the method UpdateFileSystemOfRunner method
	// if the implementation changes, delete this test and write a new one
	copyRequest := &dto.UpdateFileSystemRequest{}
	err := s.runner.UpdateFileSystem(s.TestCtx, copyRequest)
	s.Require().NoError(err)
	s.apiMock.AssertCalled(s.T(), "ExecuteCommand", mock.Anything, mock.Anything, mock.Anything,
		false, mock.AnythingOfType("bool"), mock.Anything, mock.Anything, mock.Anything)
	s.Regexp("tar --extract --absolute-names", s.command)
}

func (s *UpdateFileSystemTestSuite) TestUpdateFileSystemForRunnerReturnsErrorIfExitCodeIsNotZero() {
	s.mockedExecuteCommandCall.Return(1, nil)
	copyRequest := &dto.UpdateFileSystemRequest{}
	err := s.runner.UpdateFileSystem(s.TestCtx, copyRequest)
	s.ErrorIs(err, ErrFileCopyFailed)
}

func (s *UpdateFileSystemTestSuite) TestUpdateFileSystemForRunnerReturnsErrorIfApiCallDid() {
	s.mockedExecuteCommandCall.Return(0, tests.ErrDefault)
	copyRequest := &dto.UpdateFileSystemRequest{}
	err := s.runner.UpdateFileSystem(s.TestCtx, copyRequest)
	s.ErrorIs(err, nomad.ErrExecutorCommunicationFailed)
}

func (s *UpdateFileSystemTestSuite) TestFilesToCopyAreIncludedInTarArchive() {
	copyRequest := &dto.UpdateFileSystemRequest{Copy: []dto.File{
		{Path: tests.DefaultFileName, Content: []byte(tests.DefaultFileContent)},
	}}
	err := s.runner.UpdateFileSystem(s.TestCtx, copyRequest)
	s.Require().NoError(err)
	s.apiMock.AssertCalled(s.T(), "ExecuteCommand", mock.Anything, mock.Anything, mock.Anything, false, true,
		mock.Anything, mock.Anything, mock.Anything)

	tarFiles := s.readFilesFromTarArchive(s.stdin)
	s.Len(tarFiles, 1)
	tarFile := tarFiles[0]
	s.True(strings.HasSuffix(tarFile.Name, tests.DefaultFileName))
	s.Equal(byte(tar.TypeReg), tarFile.TypeFlag)
	s.Equal(tests.DefaultFileContent, tarFile.Content)
}

func (s *UpdateFileSystemTestSuite) TestTarFilesContainCorrectPathForRelativeFilePath() {
	copyRequest := &dto.UpdateFileSystemRequest{Copy: []dto.File{
		{Path: tests.DefaultFileName, Content: []byte(tests.DefaultFileContent)},
	}}
	err := s.runner.UpdateFileSystem(s.TestCtx, copyRequest)
	s.Require().NoError(err)

	tarFiles := s.readFilesFromTarArchive(s.stdin)
	s.Len(tarFiles, 1)
	// tar is extracted in the active workdir of the container, file will be put relative to that
	s.Equal(tests.DefaultFileName, tarFiles[0].Name)
}

func (s *UpdateFileSystemTestSuite) TestFilesWithAbsolutePathArePutInAbsoluteLocation() {
	copyRequest := &dto.UpdateFileSystemRequest{Copy: []dto.File{
		{Path: tests.FileNameWithAbsolutePath, Content: []byte(tests.DefaultFileContent)},
	}}
	err := s.runner.UpdateFileSystem(s.TestCtx, copyRequest)
	s.Require().NoError(err)

	tarFiles := s.readFilesFromTarArchive(s.stdin)
	s.Len(tarFiles, 1)
	s.Equal(tests.FileNameWithAbsolutePath, tarFiles[0].Name)
}

func (s *UpdateFileSystemTestSuite) TestDirectoriesAreMarkedAsDirectoryInTar() {
	copyRequest := &dto.UpdateFileSystemRequest{Copy: []dto.File{{Path: tests.DefaultDirectoryName, Content: []byte{}}}}
	err := s.runner.UpdateFileSystem(s.TestCtx, copyRequest)
	s.Require().NoError(err)

	tarFiles := s.readFilesFromTarArchive(s.stdin)
	s.Len(tarFiles, 1)
	tarFile := tarFiles[0]
	s.True(strings.HasSuffix(tarFile.Name+"/", tests.DefaultDirectoryName))
	s.Equal(byte(tar.TypeDir), tarFile.TypeFlag)
	s.Equal("", tarFile.Content)
}

func (s *UpdateFileSystemTestSuite) TestFilesToRemoveGetRemoved() {
	copyRequest := &dto.UpdateFileSystemRequest{Delete: []dto.FilePath{tests.DefaultFileName}}
	err := s.runner.UpdateFileSystem(s.TestCtx, copyRequest)
	s.Require().NoError(err)
	s.apiMock.AssertCalled(s.T(), "ExecuteCommand", mock.Anything, mock.Anything, mock.Anything, false, true,
		mock.Anything, mock.Anything, mock.Anything)
	s.Regexp(fmt.Sprintf("rm[^;]+%s' *;", regexp.QuoteMeta(tests.DefaultFileName)), s.command)
}

func (s *UpdateFileSystemTestSuite) TestFilesToRemoveGetEscaped() {
	copyRequest := &dto.UpdateFileSystemRequest{Delete: []dto.FilePath{"/some/potentially/harmful'filename"}}
	err := s.runner.UpdateFileSystem(s.TestCtx, copyRequest)
	s.Require().NoError(err)
	s.apiMock.AssertCalled(s.T(), "ExecuteCommand", mock.Anything, mock.Anything, mock.Anything, false, true,
		mock.Anything, mock.Anything, mock.Anything)
	s.Contains(s.command, "'/some/potentially/harmful'\\\\''filename'")
}

func (s *UpdateFileSystemTestSuite) TestResetTimerGetsCalled() {
	copyRequest := &dto.UpdateFileSystemRequest{}
	err := s.runner.UpdateFileSystem(s.TestCtx, copyRequest)
	s.Require().NoError(err)
	s.timer.AssertCalled(s.T(), "ResetTimeout")
}

type TarFile struct {
	Name     string
	Content  string
	TypeFlag byte
}

func (s *UpdateFileSystemTestSuite) readFilesFromTarArchive(tarArchive io.Reader) (files []TarFile) {
	reader := tar.NewReader(tarArchive)
	for {
		hdr, err := reader.Next()
		if err != nil {
			break
		}
		bf, err := io.ReadAll(reader)
		s.Require().NoError(err)
		files = append(files, TarFile{Name: hdr.Name, Content: string(bf), TypeFlag: hdr.Typeflag})
	}
	return files
}

func (s *UpdateFileSystemTestSuite) TestGetFileContentReturnsErrorIfExitCodeIsNotZero() {
	s.mockedExecuteCommandCall.RunFn = nil
	s.mockedExecuteCommandCall.Return(1, nil)
	err := s.runner.GetFileContent(s.TestCtx, "", logging.NewLoggingResponseWriter(nil), false)
	s.ErrorIs(err, ErrFileNotFound)
}

func (s *UpdateFileSystemTestSuite) TestFileCopyIsCanceledOnRunnerDestroy() {
	s.mockedExecuteCommandCall.Run(func(args mock.Arguments) {
		ctx, ok := args.Get(1).(context.Context)
		s.Require().True(ok)

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
	})
	ctx, cancel := context.WithCancel(context.Background())
	s.runner.ctx = ctx
	s.runner.cancel = cancel

	<-time.After(2 * tests.ShortTimeout)
	s.runner.cancel()
}
