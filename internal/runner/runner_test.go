package runner

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/internal/nomad"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/pkg/dto"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/pkg/nullio"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/tests"
	"io"
	"regexp"
	"strings"
	"testing"
	"time"
)

func TestIdIsStored(t *testing.T) {
	runner := NewNomadJob(tests.DefaultJobID, nil, nil, nil)
	assert.Equal(t, tests.DefaultJobID, runner.ID())
}

func TestMappedPortsAreStoredCorrectly(t *testing.T) {
	runner := NewNomadJob(tests.DefaultJobID, tests.DefaultPortMappings, nil, nil)
	assert.Equal(t, tests.DefaultMappedPorts, runner.MappedPorts())

	runner = NewNomadJob(tests.DefaultJobID, nil, nil, nil)
	assert.Empty(t, runner.MappedPorts())
}

func TestMarshalRunner(t *testing.T) {
	runner := NewNomadJob(tests.DefaultJobID, nil, nil, nil)
	marshal, err := json.Marshal(runner)
	assert.NoError(t, err)
	assert.Equal(t, "{\"runnerId\":\""+tests.DefaultJobID+"\"}", string(marshal))
}

func TestExecutionRequestIsStored(t *testing.T) {
	runner := NewNomadJob(tests.DefaultJobID, nil, nil, nil)
	executionRequest := &dto.ExecutionRequest{
		Command:     "command",
		TimeLimit:   10,
		Environment: nil,
	}
	id := ExecutionID("test-execution")
	runner.Add(id, executionRequest)
	storedExecutionRunner, ok := runner.Pop(id)

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
		ExecutionStorage: NewLocalExecutionStorage(),
		InactivityTimer:  s.timer,
		id:               tests.DefaultRunnerID,
		api:              s.apiMock,
		manager:          s.manager,
	}
}

func (s *ExecuteInteractivelyTestSuite) TestCallsApi() {
	request := &dto.ExecutionRequest{Command: "echo 'Hello World!'"}
	s.runner.ExecuteInteractively(request, nil, nil, nil)

	time.Sleep(tests.ShortTimeout)
	s.apiMock.AssertCalled(s.T(), "ExecuteCommand", tests.DefaultRunnerID, mock.Anything, request.FullCommand(),
		true, mock.Anything, mock.Anything, mock.Anything)
}

func (s *ExecuteInteractivelyTestSuite) TestReturnsAfterTimeout() {
	s.mockedExecuteCommandCall.Run(func(args mock.Arguments) {
		select {}
	}).Return(0, nil)

	timeLimit := 1
	execution := &dto.ExecutionRequest{TimeLimit: timeLimit}
	exit, _ := s.runner.ExecuteInteractively(execution, &nullio.ReadWriter{}, nil, nil)

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
		buffer := make([]byte, 1)                                                  //nolint:makezero,lll // If the length is zero, the Read call never reads anything. gofmt want this alignment.
		for n := 0; !(n == 1 && buffer[0] == SIGQUIT); n, _ = stdin.Read(buffer) { //nolint:errcheck,lll // Read returns EOF errors but that is expected. This nolint makes the line too long.
			time.After(tests.ShortTimeout)
		}
		close(quit)
	}).Return(0, nil)
	timeLimit := 1
	execution := &dto.ExecutionRequest{TimeLimit: timeLimit}
	_, _ = s.runner.ExecuteInteractively(execution, bytes.NewBuffer(make([]byte, 1)), nil, nil)
	select {
	case <-time.After(2 * (time.Duration(timeLimit) * time.Second)):
		s.FailNow("The execution should receive a SIGQUIT after the timeout")
	case <-quit:
	}
}

func (s *ExecuteInteractivelyTestSuite) TestDestroysRunnerAfterTimeoutAndSignal() {
	s.mockedExecuteCommandCall.Run(func(args mock.Arguments) {
		select {}
	})
	timeLimit := 1
	execution := &dto.ExecutionRequest{TimeLimit: timeLimit}
	_, _ = s.runner.ExecuteInteractively(execution, bytes.NewBuffer(make([]byte, 1)), nil, nil)
	<-time.After(executionTimeoutGracePeriod + time.Duration(timeLimit)*time.Second + tests.ShortTimeout)
	s.manager.AssertCalled(s.T(), "Return", s.runner)
}

func (s *ExecuteInteractivelyTestSuite) TestResetTimerGetsCalled() {
	execution := &dto.ExecutionRequest{}
	s.runner.ExecuteInteractively(execution, nil, nil, nil)
	s.timer.AssertCalled(s.T(), "ResetTimeout")
}

func (s *ExecuteInteractivelyTestSuite) TestExitHasTimeoutErrorIfRunnerTimesOut() {
	s.mockedTimeoutPassedCall.Return(true)
	execution := &dto.ExecutionRequest{}
	exitChannel, _ := s.runner.ExecuteInteractively(execution, &nullio.ReadWriter{}, nil, nil)
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
		ExecutionStorage: NewLocalExecutionStorage(),
		InactivityTimer:  s.timer,
		id:               tests.DefaultRunnerID,
		api:              s.apiMock,
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

func TestInactivityTimerTestSuite(t *testing.T) {
	suite.Run(t, new(InactivityTimerTestSuite))
}

type InactivityTimerTestSuite struct {
	suite.Suite
	runner   Runner
	manager  *ManagerMock
	returned chan bool
}

func (s *InactivityTimerTestSuite) SetupTest() {
	s.returned = make(chan bool, 1)
	s.manager = &ManagerMock{}
	s.manager.On("Return", mock.Anything).Run(func(_ mock.Arguments) {
		s.returned <- true
	}).Return(nil)

	s.runner = NewRunner(tests.DefaultRunnerID, s.manager)

	s.runner.SetupTimeout(tests.ShortTimeout)
}

func (s *InactivityTimerTestSuite) TearDownTest() {
	s.runner.StopTimeout()
}

func (s *InactivityTimerTestSuite) TestRunnerIsReturnedAfterTimeout() {
	s.True(tests.ChannelReceivesSomething(s.returned, 2*tests.ShortTimeout))
}

func (s *InactivityTimerTestSuite) TestRunnerIsNotReturnedBeforeTimeout() {
	s.False(tests.ChannelReceivesSomething(s.returned, tests.ShortTimeout/2))
}

func (s *InactivityTimerTestSuite) TestResetTimeoutExtendsTheDeadline() {
	time.Sleep(3 * tests.ShortTimeout / 4)
	s.runner.ResetTimeout()
	s.False(tests.ChannelReceivesSomething(s.returned, 3*tests.ShortTimeout/4),
		"Because of the reset, the timeout should not be reached by now.")
	s.True(tests.ChannelReceivesSomething(s.returned, 5*tests.ShortTimeout/4),
		"After reset, the timout should be reached by now.")
}

func (s *InactivityTimerTestSuite) TestStopTimeoutStopsTimeout() {
	s.runner.StopTimeout()
	s.False(tests.ChannelReceivesSomething(s.returned, 2*tests.ShortTimeout))
}

func (s *InactivityTimerTestSuite) TestTimeoutPassedReturnsFalseBeforeDeadline() {
	s.False(s.runner.TimeoutPassed())
}

func (s *InactivityTimerTestSuite) TestTimeoutPassedReturnsTrueAfterDeadline() {
	time.Sleep(2 * tests.ShortTimeout)
	s.True(s.runner.TimeoutPassed())
}

func (s *InactivityTimerTestSuite) TestTimerIsNotResetAfterDeadline() {
	time.Sleep(2 * tests.ShortTimeout)
	// We need to empty the returned channel so Return can send to it again.
	tests.ChannelReceivesSomething(s.returned, 0)
	s.runner.ResetTimeout()
	s.False(tests.ChannelReceivesSomething(s.returned, 2*tests.ShortTimeout))
}

func (s *InactivityTimerTestSuite) TestSetupTimeoutStopsOldTimeout() {
	s.runner.SetupTimeout(3 * tests.ShortTimeout)
	s.False(tests.ChannelReceivesSomething(s.returned, 2*tests.ShortTimeout))
	s.True(tests.ChannelReceivesSomething(s.returned, 2*tests.ShortTimeout))
}

func (s *InactivityTimerTestSuite) TestTimerIsInactiveWhenDurationIsZero() {
	s.runner.SetupTimeout(0)
	s.False(tests.ChannelReceivesSomething(s.returned, tests.ShortTimeout))
}

// NewRunner creates a new runner with the provided id and manager.
func NewRunner(id string, manager Manager) Runner {
	return NewNomadJob(id, nil, nil, manager)
}
