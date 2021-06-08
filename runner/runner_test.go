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
	runner := NewNomadAllocation("42", nil)
	assert.Equal(t, "42", runner.Id())
}

func TestMarshalRunner(t *testing.T) {
	runner := NewNomadAllocation("42", nil)
	marshal, err := json.Marshal(runner)
	assert.NoError(t, err)
	assert.Equal(t, "{\"runnerId\":\"42\"}", string(marshal))
}

func TestExecutionRequestIsStored(t *testing.T) {
	runner := NewNomadAllocation("42", nil)
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
	runner := NewNomadAllocation("testRunner", nil)
	ctx := context.Background()
	newCtx := NewContext(ctx, runner)
	storedRunner := newCtx.Value(runnerContextKey).(Runner)

	assert.NotEqual(t, ctx, newCtx)
	assert.Equal(t, runner, storedRunner)
}

func TestFromContextReturnsRunner(t *testing.T) {
	runner := NewNomadAllocation("testRunner", nil)
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
	apiMock := &nomad.ExecutorApiMock{}
	apiMock.On("ExecuteCommand", mock.Anything, mock.Anything, mock.Anything, true, mock.Anything, mock.Anything, mock.Anything).Return(0, nil)
	runner := NewNomadAllocation(tests.DefaultRunnerId, apiMock)

	request := &dto.ExecutionRequest{Command: "echo 'Hello World!'"}
	runner.ExecuteInteractively(request, nil, nil, nil)

	<-time.After(50 * time.Millisecond)
	apiMock.AssertCalled(t, "ExecuteCommand", tests.DefaultRunnerId, mock.Anything, request.FullCommand(), true, mock.Anything, mock.Anything, mock.Anything)
}

func TestExecuteReturnsAfterTimeout(t *testing.T) {
	apiMock := newApiMockWithTimeLimitHandling()
	runner := NewNomadAllocation(tests.DefaultRunnerId, apiMock)

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

func newApiMockWithTimeLimitHandling() (apiMock *nomad.ExecutorApiMock) {
	apiMock = &nomad.ExecutorApiMock{}
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
	runner                   *NomadAllocation
	apiMock                  *nomad.ExecutorApiMock
	mockedExecuteCommandCall *mock.Call
	command                  []string
	stdin                    *bytes.Buffer
}

func (s *UpdateFileSystemTestSuite) SetupTest() {
	s.apiMock = &nomad.ExecutorApiMock{}
	s.runner = NewNomadAllocation(tests.DefaultRunnerId, s.apiMock)
	s.mockedExecuteCommandCall = s.apiMock.On("ExecuteCommand", tests.DefaultRunnerId, mock.Anything, mock.Anything, false, mock.Anything, mock.Anything, mock.Anything).
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
	s.mockedExecuteCommandCall.Return(0, tests.DefaultError)
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
