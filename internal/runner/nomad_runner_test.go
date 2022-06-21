package runner

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/openHPI/poseidon/internal/nomad"
	"github.com/openHPI/poseidon/pkg/dto"
	"github.com/openHPI/poseidon/pkg/nullio"
	"github.com/openHPI/poseidon/pkg/storage"
	"github.com/openHPI/poseidon/tests"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"io"
	"regexp"
	"strings"
	"testing"
	"time"
)

const defaultExecutionID = "execution-id"

func TestIdIsStored(t *testing.T) {
	runner := NewNomadJob(tests.DefaultRunnerID, nil, nil, nil)
	assert.Equal(t, tests.DefaultRunnerID, runner.ID())
}

func TestMappedPortsAreStoredCorrectly(t *testing.T) {
	runner := NewNomadJob(tests.DefaultRunnerID, tests.DefaultPortMappings, nil, nil)
	assert.Equal(t, tests.DefaultMappedPorts, runner.MappedPorts())

	runner = NewNomadJob(tests.DefaultRunnerID, nil, nil, nil)
	assert.Empty(t, runner.MappedPorts())
}

func TestMarshalRunner(t *testing.T) {
	runner := NewNomadJob(tests.DefaultRunnerID, nil, nil, nil)
	marshal, err := json.Marshal(runner)
	assert.NoError(t, err)
	assert.Equal(t, "{\"runnerId\":\""+tests.DefaultRunnerID+"\"}", string(marshal))
}

func TestExecutionRequestIsStored(t *testing.T) {
	runner := NewNomadJob(tests.DefaultRunnerID, nil, nil, nil)
	executionRequest := &dto.ExecutionRequest{
		Command:     "command",
		TimeLimit:   10,
		Environment: nil,
	}
	id := "test-execution"
	runner.StoreExecution(id, executionRequest)
	storedExecutionRunner, ok := runner.executions.Pop(id)

	assert.True(t, ok, "Getting an execution should not return ok false")
	assert.Equal(t, executionRequest, storedExecutionRunner)
}

func TestNewContextReturnsNewContextWithRunner(t *testing.T) {
	runner := NewNomadJob(tests.DefaultRunnerID, nil, nil, nil)
	ctx := context.Background()
	newCtx := NewContext(ctx, runner)
	storedRunner, ok := newCtx.Value(runnerContextKey).(Runner)
	require.True(t, ok)

	assert.NotEqual(t, ctx, newCtx)
	assert.Equal(t, runner, storedRunner)
}

func TestFromContextReturnsRunner(t *testing.T) {
	runner := NewNomadJob(tests.DefaultRunnerID, nil, nil, nil)
	ctx := NewContext(context.Background(), runner)
	storedRunner, ok := FromContext(ctx)

	assert.True(t, ok)
	assert.Equal(t, runner, storedRunner)
}

func TestFromContextReturnsIsNotOkWhenContextHasNoRunner(t *testing.T) {
	ctx := context.Background()
	_, ok := FromContext(ctx)

	assert.False(t, ok)
}

func TestDestroyReturnsRunner(t *testing.T) {
	manager := &ManagerMock{}
	manager.On("Return", mock.Anything).Return(nil)
	runner := NewRunner(tests.DefaultRunnerID, manager)
	err := runner.Destroy()
	assert.NoError(t, err)
	manager.AssertCalled(t, "Return", runner)
}

func TestExecuteInteractivelyTestSuite(t *testing.T) {
	suite.Run(t, new(ExecuteInteractivelyTestSuite))
}

type ExecuteInteractivelyTestSuite struct {
	suite.Suite
	runner                   *NomadJob
	apiMock                  *nomad.ExecutorAPIMock
	timer                    *InactivityTimerMock
	manager                  *ManagerMock
	mockedExecuteCommandCall *mock.Call
	mockedTimeoutPassedCall  *mock.Call
}

func (s *ExecuteInteractivelyTestSuite) SetupTest() {
	s.apiMock = &nomad.ExecutorAPIMock{}
	s.mockedExecuteCommandCall = s.apiMock.
		On("ExecuteCommand", mock.Anything, mock.Anything, mock.Anything, true, mock.Anything, mock.Anything, mock.Anything).
		Return(0, nil)
	s.timer = &InactivityTimerMock{}
	s.timer.On("ResetTimeout").Return()
	s.mockedTimeoutPassedCall = s.timer.On("TimeoutPassed").Return(false)
	s.manager = &ManagerMock{}
	s.manager.On("Return", mock.Anything).Return(nil)

	s.runner = &NomadJob{
		executions:      storage.NewLocalStorage[*dto.ExecutionRequest](),
		InactivityTimer: s.timer,
		id:              tests.DefaultRunnerID,
		api:             s.apiMock,
		onDestroy:       s.manager.Return,
	}
}

func (s *ExecuteInteractivelyTestSuite) TestReturnsErrorWhenExecutionDoesNotExist() {
	_, _, err := s.runner.ExecuteInteractively("non-existent-id", nil, nil, nil)
	s.ErrorIs(err, ErrorUnknownExecution)
}

func (s *ExecuteInteractivelyTestSuite) TestCallsApi() {
	request := &dto.ExecutionRequest{Command: "echo 'Hello World!'"}
	s.runner.StoreExecution(defaultExecutionID, request)
	_, _, err := s.runner.ExecuteInteractively(defaultExecutionID, nil, nil, nil)
	s.Require().NoError(err)

	time.Sleep(tests.ShortTimeout)
	s.apiMock.AssertCalled(s.T(), "ExecuteCommand", tests.DefaultRunnerID, mock.Anything, request.FullCommand(),
		true, mock.Anything, mock.Anything, mock.Anything)
}

func (s *ExecuteInteractivelyTestSuite) TestReturnsAfterTimeout() {
	s.mockedExecuteCommandCall.Run(func(args mock.Arguments) {
		select {}
	}).Return(0, nil)

	timeLimit := 1
	executionRequest := &dto.ExecutionRequest{TimeLimit: timeLimit}
	s.runner.StoreExecution(defaultExecutionID, executionRequest)
	exit, _, err := s.runner.ExecuteInteractively(defaultExecutionID, &nullio.ReadWriter{}, nil, nil)
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
}

func (s *ExecuteInteractivelyTestSuite) TestSendsSignalAfterTimeout() {
	quit := make(chan struct{})
	s.mockedExecuteCommandCall.Run(func(args mock.Arguments) {
		stdin, ok := args.Get(4).(io.Reader)
		s.Require().True(ok)
		buffer := make([]byte, 1) //nolint:makezero,lll // If the length is zero, the Read call never reads anything. gofmt want this alignment.
		for n := 0; !(n == 1 && buffer[0] == SIGQUIT); {
			time.After(tests.ShortTimeout)
			n, _ = stdin.Read(buffer) //nolint:errcheck,lll // Read returns EOF errors but that is expected. This nolint makes the line too long.
			if n > 0 {
				log.WithField("buffer", fmt.Sprintf("%x", buffer[0])).Info("Received Stdin")
			}
		}
		log.Info("After loop")
		close(quit)
	}).Return(0, nil)
	timeLimit := 1
	executionRequest := &dto.ExecutionRequest{TimeLimit: timeLimit}
	s.runner.StoreExecution(defaultExecutionID, executionRequest)
	_, _, err := s.runner.ExecuteInteractively(defaultExecutionID, bytes.NewBuffer(make([]byte, 1)), nil, nil)
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
	s.mockedExecuteCommandCall.Run(func(args mock.Arguments) {
		select {}
	})
	timeLimit := 1
	executionRequest := &dto.ExecutionRequest{TimeLimit: timeLimit}
	s.runner.StoreExecution(defaultExecutionID, executionRequest)
	_, _, err := s.runner.ExecuteInteractively(defaultExecutionID, bytes.NewBuffer(make([]byte, 1)), nil, nil)
	s.Require().NoError(err)
	<-time.After(executionTimeoutGracePeriod + time.Duration(timeLimit)*time.Second + tests.ShortTimeout)
	s.manager.AssertCalled(s.T(), "Return", s.runner)
}

func (s *ExecuteInteractivelyTestSuite) TestResetTimerGetsCalled() {
	executionRequest := &dto.ExecutionRequest{}
	s.runner.StoreExecution(defaultExecutionID, executionRequest)
	_, _, err := s.runner.ExecuteInteractively(defaultExecutionID, nil, nil, nil)
	s.Require().NoError(err)
	s.timer.AssertCalled(s.T(), "ResetTimeout")
}

func (s *ExecuteInteractivelyTestSuite) TestExitHasTimeoutErrorIfRunnerTimesOut() {
	s.mockedTimeoutPassedCall.Return(true)
	executionRequest := &dto.ExecutionRequest{}
	s.runner.StoreExecution(defaultExecutionID, executionRequest)
	exitChannel, _, err := s.runner.ExecuteInteractively(defaultExecutionID, &nullio.ReadWriter{}, nil, nil)
	s.Require().NoError(err)
	exit := <-exitChannel
	s.Equal(ErrorRunnerInactivityTimeout, exit.Err)
}

func TestUpdateFileSystemTestSuite(t *testing.T) {
	suite.Run(t, new(UpdateFileSystemTestSuite))
}

type UpdateFileSystemTestSuite struct {
	suite.Suite
	runner                   *NomadJob
	timer                    *InactivityTimerMock
	apiMock                  *nomad.ExecutorAPIMock
	mockedExecuteCommandCall *mock.Call
	command                  []string
	stdin                    *bytes.Buffer
}

func (s *UpdateFileSystemTestSuite) SetupTest() {
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
	s.mockedExecuteCommandCall = s.apiMock.On("ExecuteCommand", tests.DefaultRunnerID, mock.Anything,
		mock.Anything, false, mock.Anything, mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) {
			var ok bool
			s.command, ok = args.Get(2).([]string)
			s.Require().True(ok)
			s.stdin, ok = args.Get(4).(*bytes.Buffer)
			s.Require().True(ok)
		}).Return(0, nil)
}

func (s *UpdateFileSystemTestSuite) TestUpdateFileSystemForRunnerPerformsTarExtractionWithAbsoluteNamesOnRunner() {
	// note: this method tests an implementation detail of the method UpdateFileSystemOfRunner method
	// if the implementation changes, delete this test and write a new one
	copyRequest := &dto.UpdateFileSystemRequest{}
	err := s.runner.UpdateFileSystem(copyRequest)
	s.NoError(err)
	s.apiMock.AssertCalled(s.T(), "ExecuteCommand", mock.Anything, mock.Anything, mock.Anything,
		false, mock.Anything, mock.Anything, mock.Anything)
	s.Regexp("tar --extract --absolute-names", s.command)
}

func (s *UpdateFileSystemTestSuite) TestUpdateFileSystemForRunnerReturnsErrorIfExitCodeIsNotZero() {
	s.mockedExecuteCommandCall.Return(1, nil)
	copyRequest := &dto.UpdateFileSystemRequest{}
	err := s.runner.UpdateFileSystem(copyRequest)
	s.ErrorIs(err, ErrorFileCopyFailed)
}

func (s *UpdateFileSystemTestSuite) TestUpdateFileSystemForRunnerReturnsErrorIfApiCallDid() {
	s.mockedExecuteCommandCall.Return(0, tests.ErrDefault)
	copyRequest := &dto.UpdateFileSystemRequest{}
	err := s.runner.UpdateFileSystem(copyRequest)
	s.ErrorIs(err, nomad.ErrorExecutorCommunicationFailed)
}

func (s *UpdateFileSystemTestSuite) TestFilesToCopyAreIncludedInTarArchive() {
	copyRequest := &dto.UpdateFileSystemRequest{Copy: []dto.File{
		{Path: tests.DefaultFileName, Content: []byte(tests.DefaultFileContent)}}}
	err := s.runner.UpdateFileSystem(copyRequest)
	s.NoError(err)
	s.apiMock.AssertCalled(s.T(), "ExecuteCommand", mock.Anything, mock.Anything, mock.Anything, false,
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
		{Path: tests.DefaultFileName, Content: []byte(tests.DefaultFileContent)}}}
	err := s.runner.UpdateFileSystem(copyRequest)
	s.Require().NoError(err)

	tarFiles := s.readFilesFromTarArchive(s.stdin)
	s.Len(tarFiles, 1)
	// tar is extracted in the active workdir of the container, file will be put relative to that
	s.Equal(tests.DefaultFileName, tarFiles[0].Name)
}

func (s *UpdateFileSystemTestSuite) TestFilesWithAbsolutePathArePutInAbsoluteLocation() {
	copyRequest := &dto.UpdateFileSystemRequest{Copy: []dto.File{
		{Path: tests.FileNameWithAbsolutePath, Content: []byte(tests.DefaultFileContent)}}}
	err := s.runner.UpdateFileSystem(copyRequest)
	s.Require().NoError(err)

	tarFiles := s.readFilesFromTarArchive(s.stdin)
	s.Len(tarFiles, 1)
	s.Equal(tarFiles[0].Name, tests.FileNameWithAbsolutePath)
}

func (s *UpdateFileSystemTestSuite) TestDirectoriesAreMarkedAsDirectoryInTar() {
	copyRequest := &dto.UpdateFileSystemRequest{Copy: []dto.File{{Path: tests.DefaultDirectoryName, Content: []byte{}}}}
	err := s.runner.UpdateFileSystem(copyRequest)
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
	err := s.runner.UpdateFileSystem(copyRequest)
	s.NoError(err)
	s.apiMock.AssertCalled(s.T(), "ExecuteCommand", mock.Anything, mock.Anything, mock.Anything, false,
		mock.Anything, mock.Anything, mock.Anything)
	s.Regexp(fmt.Sprintf("rm[^;]+%s' *;", regexp.QuoteMeta(tests.DefaultFileName)), s.command)
}

func (s *UpdateFileSystemTestSuite) TestFilesToRemoveGetEscaped() {
	copyRequest := &dto.UpdateFileSystemRequest{Delete: []dto.FilePath{"/some/potentially/harmful'filename"}}
	err := s.runner.UpdateFileSystem(copyRequest)
	s.NoError(err)
	s.apiMock.AssertCalled(s.T(), "ExecuteCommand", mock.Anything, mock.Anything, mock.Anything, false,
		mock.Anything, mock.Anything, mock.Anything)
	s.Contains(strings.Join(s.command, " "), "'/some/potentially/harmful'\\''filename'")
}

func (s *UpdateFileSystemTestSuite) TestResetTimerGetsCalled() {
	copyRequest := &dto.UpdateFileSystemRequest{}
	err := s.runner.UpdateFileSystem(copyRequest)
	s.NoError(err)
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

// NewRunner creates a new runner with the provided id and manager.
func NewRunner(id string, manager Accessor) Runner {
	var handler DestroyRunnerHandler
	if manager != nil {
		handler = manager.Return
	} else {
		handler = func(_ Runner) error { return nil }
	}
	return NewNomadJob(id, nil, nil, handler)
}
