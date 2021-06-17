package runner

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/api/dto"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/nomad"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/tests"
	"io"
	"regexp"
	"strings"
	"testing"
	"time"
)

func TestIdIsStored(t *testing.T) {
	runner := NewNomadJob(tests.DefaultJobID, nil)
	assert.Equal(t, tests.DefaultJobID, runner.Id())
}

func TestMarshalRunner(t *testing.T) {
	runner := NewNomadJob(tests.DefaultJobID, nil)
	marshal, err := json.Marshal(runner)
	assert.NoError(t, err)
	assert.Equal(t, "{\"runnerId\":\""+tests.DefaultJobID+"\"}", string(marshal))
}

func TestExecutionRequestIsStored(t *testing.T) {
	runner := NewNomadJob(tests.DefaultJobID, nil)
	executionRequest := &dto.ExecutionRequest{
		Command:     "command",
		TimeLimit:   10,
		Environment: nil,
	}
	id := ExecutionId("test-execution")
	runner.Add(id, executionRequest)
	storedExecutionRunner, ok := runner.Pop(id)

	assert.True(t, ok, "Getting an execution should not return ok false")
	assert.Equal(t, executionRequest, storedExecutionRunner)
}

func TestNewContextReturnsNewContextWithRunner(t *testing.T) {
	runner := NewNomadJob(tests.DefaultRunnerID, nil)
	ctx := context.Background()
	newCtx := NewContext(ctx, runner)
	storedRunner := newCtx.Value(runnerContextKey).(Runner)

	assert.NotEqual(t, ctx, newCtx)
	assert.Equal(t, runner, storedRunner)
}

func TestFromContextReturnsRunner(t *testing.T) {
	runner := NewNomadJob(tests.DefaultRunnerID, nil)
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

func TestExecuteCallsAPI(t *testing.T) {
	apiMock := &nomad.ExecutorAPIMock{}
	apiMock.On("ExecuteCommand", mock.Anything, mock.Anything, mock.Anything, true, mock.Anything, mock.Anything, mock.Anything).Return(0, nil)
	runner := NewNomadJob(tests.DefaultRunnerID, apiMock)

	request := &dto.ExecutionRequest{Command: "echo 'Hello World!'"}
	runner.ExecuteInteractively(request, nil, nil, nil)

	time.Sleep(tests.ShortTimeout)
	s.apiMock.AssertCalled(s.T(), "ExecuteCommand", tests.DefaultRunnerID, mock.Anything, request.FullCommand(), true, mock.Anything, mock.Anything, mock.Anything)
}

func TestExecuteReturnsAfterTimeout(t *testing.T) {
	apiMock := newApiMockWithTimeLimitHandling()
	runner := NewNomadJob(tests.DefaultRunnerID, apiMock)

	timeLimit := 1
	execution := &dto.ExecutionRequest{TimeLimit: timeLimit}
	exit, _ := runner.ExecuteInteractively(execution, nil, nil, nil)

	select {
	case <-exit:
		assert.FailNow(t, "ExecuteInteractively should not terminate instantly")
	case <-time.After(50 * time.Millisecond):
	}

	select {
	case <-time.After(time.Duration(timeLimit) * time.Second):
		assert.FailNow(t, "ExecuteInteractively should return after the time limit")
	case exitInfo := <-exit:
		assert.Equal(t, uint8(0), exitInfo.Code)
	}
}

func newApiMockWithTimeLimitHandling() (apiMock *nomad.ExecutorAPIMock) {
	apiMock = &nomad.ExecutorAPIMock{}
	apiMock.
		On("ExecuteCommand", mock.Anything, mock.Anything, mock.Anything, true, mock.Anything, mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) {
			ctx := args.Get(1).(context.Context)
			<-ctx.Done()
		}).
		Return(0, nil)
	return
}

func TestUpdateFileSystemTestSuite(t *testing.T) {
	suite.Run(t, new(UpdateFileSystemTestSuite))
}

type UpdateFileSystemTestSuite struct {
	suite.Suite
	runner                   *NomadJob
	apiMock                  *nomad.ExecutorAPIMock
	mockedExecuteCommandCall *mock.Call
	command                  []string
	stdin                    *bytes.Buffer
}

func (s *UpdateFileSystemTestSuite) SetupTest() {
	s.apiMock = &nomad.ExecutorAPIMock{}
	s.runner = NewNomadJob(tests.DefaultRunnerID, s.apiMock)
	s.mockedExecuteCommandCall = s.apiMock.On("ExecuteCommand", tests.DefaultRunnerID, mock.Anything, mock.Anything, false, mock.Anything, mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) {
			s.command = args.Get(2).([]string)
			s.stdin = args.Get(4).(*bytes.Buffer)
		})
}

func (s *UpdateFileSystemTestSuite) TestUpdateFileSystemForRunnerPerformsTarExtractionWithAbsoluteNamesOnRunner() {
	// note: this method tests an implementation detail of the method UpdateFileSystemOfRunner method
	// if the implementation changes, delete this test and write a new one
	s.mockedExecuteCommandCall.Return(0, nil)
	copyRequest := &dto.UpdateFileSystemRequest{}
	err := s.runner.UpdateFileSystem(copyRequest)
	s.NoError(err)
	s.apiMock.AssertCalled(s.T(), "ExecuteCommand", mock.Anything, mock.Anything, mock.Anything, false, mock.Anything, mock.Anything, mock.Anything)
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
	s.mockedExecuteCommandCall.Return(0, nil)
	copyRequest := &dto.UpdateFileSystemRequest{Copy: []dto.File{{Path: tests.DefaultFileName, Content: []byte(tests.DefaultFileContent)}}}
	err := s.runner.UpdateFileSystem(copyRequest)
	s.NoError(err)
	s.apiMock.AssertCalled(s.T(), "ExecuteCommand", mock.Anything, mock.Anything, mock.Anything, false, mock.Anything, mock.Anything, mock.Anything)

	tarFiles := s.readFilesFromTarArchive(s.stdin)
	s.Len(tarFiles, 1)
	tarFile := tarFiles[0]
	s.True(strings.HasSuffix(tarFile.Name, tests.DefaultFileName))
	s.Equal(byte(tar.TypeReg), tarFile.TypeFlag)
	s.Equal(tests.DefaultFileContent, tarFile.Content)
}

func (s *UpdateFileSystemTestSuite) TestTarFilesContainCorrectPathForRelativeFilePath() {
	s.mockedExecuteCommandCall.Return(0, nil)
	copyRequest := &dto.UpdateFileSystemRequest{Copy: []dto.File{{Path: tests.DefaultFileName, Content: []byte(tests.DefaultFileContent)}}}
	_ = s.runner.UpdateFileSystem(copyRequest)

	tarFiles := s.readFilesFromTarArchive(s.stdin)
	s.Len(tarFiles, 1)
	// tar is extracted in the active workdir of the container, file will be put relative to that
	s.Equal(tests.DefaultFileName, tarFiles[0].Name)
}

func (s *UpdateFileSystemTestSuite) TestFilesWithAbsolutePathArePutInAbsoluteLocation() {
	s.mockedExecuteCommandCall.Return(0, nil)
	copyRequest := &dto.UpdateFileSystemRequest{Copy: []dto.File{{Path: tests.FileNameWithAbsolutePath, Content: []byte(tests.DefaultFileContent)}}}
	_ = s.runner.UpdateFileSystem(copyRequest)

	tarFiles := s.readFilesFromTarArchive(s.stdin)
	s.Len(tarFiles, 1)
	s.Equal(tarFiles[0].Name, tests.FileNameWithAbsolutePath)
}

func (s *UpdateFileSystemTestSuite) TestDirectoriesAreMarkedAsDirectoryInTar() {
	s.mockedExecuteCommandCall.Return(0, nil)
	copyRequest := &dto.UpdateFileSystemRequest{Copy: []dto.File{{Path: tests.DefaultDirectoryName, Content: []byte{}}}}
	_ = s.runner.UpdateFileSystem(copyRequest)

	tarFiles := s.readFilesFromTarArchive(s.stdin)
	s.Len(tarFiles, 1)
	tarFile := tarFiles[0]
	s.True(strings.HasSuffix(tarFile.Name+"/", tests.DefaultDirectoryName))
	s.Equal(byte(tar.TypeDir), tarFile.TypeFlag)
	s.Equal("", tarFile.Content)
}

func (s *UpdateFileSystemTestSuite) TestFilesToRemoveGetRemoved() {
	s.mockedExecuteCommandCall.Return(0, nil)
	copyRequest := &dto.UpdateFileSystemRequest{Delete: []dto.FilePath{tests.DefaultFileName}}
	err := s.runner.UpdateFileSystem(copyRequest)
	s.NoError(err)
	s.apiMock.AssertCalled(s.T(), "ExecuteCommand", mock.Anything, mock.Anything, mock.Anything, false, mock.Anything, mock.Anything, mock.Anything)
	s.Regexp(fmt.Sprintf("rm[^;]+%s' *;", regexp.QuoteMeta(tests.DefaultFileName)), s.command)
}

func (s *UpdateFileSystemTestSuite) TestFilesToRemoveGetEscaped() {
	s.mockedExecuteCommandCall.Return(0, nil)
	copyRequest := &dto.UpdateFileSystemRequest{Delete: []dto.FilePath{"/some/potentially/harmful'filename"}}
	err := s.runner.UpdateFileSystem(copyRequest)
	s.NoError(err)
	s.apiMock.AssertCalled(s.T(), "ExecuteCommand", mock.Anything, mock.Anything, mock.Anything, false, mock.Anything, mock.Anything, mock.Anything)
	s.Contains(strings.Join(s.command, " "), "'/some/potentially/harmful'\\''filename'")
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
		bf, _ := io.ReadAll(reader)
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
	s.returned = make(chan bool)
	s.runner = NewRunner(tests.DefaultRunnerID)
	s.manager = &ManagerMock{}
	s.manager.On("Return", mock.Anything).Run(func(_ mock.Arguments) {
		s.returned <- true
	}).Return(nil)

	s.runner.SetupTimeout(tests.ShortTimeout, s.runner, s.manager)
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
	time.Sleep(2 * tests.ShortTimeout / 4)
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
	<-s.returned
	s.runner.ResetTimeout()
	s.False(tests.ChannelReceivesSomething(s.returned, 2*tests.ShortTimeout))
}

func (s *InactivityTimerTestSuite) TestSetupTimoutStopsOldTimout() {
	s.runner.SetupTimeout(3*tests.ShortTimeout, s.runner, s.manager)
	s.False(tests.ChannelReceivesSomething(s.returned, 2*tests.ShortTimeout))
	s.True(tests.ChannelReceivesSomething(s.returned, 2*tests.ShortTimeout))
}

func (s *InactivityTimerTestSuite) TestTimerIsInactiveWhenDurationIsZero() {
	s.runner.SetupTimeout(0, s.runner, s.manager)
	s.False(tests.ChannelReceivesSomething(s.returned, tests.ShortTimeout))
}

// NewRunner creates a new runner with the provided id.
func NewRunner(id string) Runner {
	return NewNomadJob(id, nil)
}
