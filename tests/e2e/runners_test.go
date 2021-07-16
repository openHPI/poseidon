package e2e

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/gorilla/websocket"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/internal/api"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/pkg/dto"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/tests"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/tests/helpers"
	"io"
	"net/http"
	"strings"
	"time"
)

func (s *E2ETestSuite) TestProvideRunnerRoute() {
	runnerRequestByteString, err := json.Marshal(dto.RunnerRequest{
		ExecutionEnvironmentID: tests.DefaultEnvironmentIDAsInteger,
	})
	s.Require().NoError(err)
	reader := bytes.NewReader(runnerRequestByteString)

	s.Run("valid request returns a runner", func() {
		resp, err := http.Post(helpers.BuildURL(api.BasePath, api.RunnersPath), "application/json", reader)
		s.Require().NoError(err)
		s.Equal(http.StatusOK, resp.StatusCode)

		runnerResponse := new(dto.RunnerResponse)
		err = json.NewDecoder(resp.Body).Decode(runnerResponse)
		s.Require().NoError(err)
		s.NotEmpty(runnerResponse.ID)
	})

	s.Run("invalid request returns bad request", func() {
		resp, err := http.Post(helpers.BuildURL(api.BasePath, api.RunnersPath), "application/json", strings.NewReader(""))
		s.Require().NoError(err)
		s.Equal(http.StatusBadRequest, resp.StatusCode)
	})

	s.Run("requesting runner of unknown execution environment returns not found", func() {
		runnerRequestByteString, err := json.Marshal(dto.RunnerRequest{
			ExecutionEnvironmentID: tests.NonExistingIntegerID,
		})
		s.Require().NoError(err)
		reader := bytes.NewReader(runnerRequestByteString)
		resp, err := http.Post(helpers.BuildURL(api.BasePath, api.RunnersPath), "application/json", reader)
		s.Require().NoError(err)
		s.Equal(http.StatusNotFound, resp.StatusCode)
	})
}

// ProvideRunner creates a runner with the given RunnerRequest via an external request.
// It needs a running Poseidon instance to work.
func ProvideRunner(request *dto.RunnerRequest) (string, error) {
	url := helpers.BuildURL(api.BasePath, api.RunnersPath)
	runnerRequestByteString, err := json.Marshal(request)
	if err != nil {
		return "", err
	}
	reader := strings.NewReader(string(runnerRequestByteString))
	resp, err := http.Post(url, "application/json", reader) //nolint:gosec // url is not influenced by a user
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		//nolint:goerr113 // dynamic error is ok in here, as it is a test
		return "", fmt.Errorf("expected response code 200 when getting runner, got %v", resp.StatusCode)
	}
	runnerResponse := new(dto.RunnerResponse)
	err = json.NewDecoder(resp.Body).Decode(runnerResponse)
	if err != nil {
		return "", err
	}
	return runnerResponse.ID, nil
}

func (s *E2ETestSuite) TestDeleteRunnerRoute() {
	runnerID, err := ProvideRunner(&dto.RunnerRequest{
		ExecutionEnvironmentID: tests.DefaultEnvironmentIDAsInteger,
	})
	s.NoError(err)

	s.Run("Deleting the runner returns NoContent", func() {
		resp, err := helpers.HTTPDelete(helpers.BuildURL(api.BasePath, api.RunnersPath, runnerID), nil)
		s.NoError(err)
		s.Equal(http.StatusNoContent, resp.StatusCode)
	})

	s.Run("Deleting it again returns NotFound", func() {
		resp, err := helpers.HTTPDelete(helpers.BuildURL(api.BasePath, api.RunnersPath, runnerID), nil)
		s.NoError(err)
		s.Equal(http.StatusNotFound, resp.StatusCode)
	})

	s.Run("Deleting non-existing runner returns NotFound", func() {
		resp, err := helpers.HTTPDelete(helpers.BuildURL(api.BasePath, api.RunnersPath, tests.NonExistingStringID), nil)
		s.NoError(err)
		s.Equal(http.StatusNotFound, resp.StatusCode)
	})
}

//nolint:funlen // there are a lot of tests for the files route, this function can be a little longer than 100 lines ;)
func (s *E2ETestSuite) TestCopyFilesRoute() {
	runnerID, err := ProvideRunner(&dto.RunnerRequest{
		ExecutionEnvironmentID: tests.DefaultEnvironmentIDAsInteger,
	})
	s.NoError(err)
	copyFilesRequestByteString, err := json.Marshal(&dto.UpdateFileSystemRequest{
		Copy: []dto.File{{Path: tests.DefaultFileName, Content: []byte(tests.DefaultFileContent)}},
	})
	s.Require().NoError(err)
	sendCopyRequest := func(reader io.Reader) (*http.Response, error) {
		return helpers.HTTPPatch(helpers.BuildURL(api.BasePath, api.RunnersPath, runnerID, api.UpdateFileSystemPath),
			"application/json", reader)
	}

	s.Run("File copy with valid payload succeeds", func() {
		resp, err := sendCopyRequest(bytes.NewReader(copyFilesRequestByteString))
		s.NoError(err)
		s.Equal(http.StatusNoContent, resp.StatusCode)

		s.Run("File content can be printed on runner", func() {
			s.assertFileContent(runnerID, tests.DefaultFileName, tests.DefaultFileContent)
		})
	})

	s.Run("Files are put in correct location", func() {
		relativeFilePath := "relative/file/path.txt"
		relativeFileContent := "Relative file content"
		absoluteFilePath := "/tmp/absolute/file/path.txt"
		absoluteFileContent := "Absolute file content"
		testFilePathsCopyRequestString, err := json.Marshal(&dto.UpdateFileSystemRequest{
			Copy: []dto.File{
				{Path: dto.FilePath(relativeFilePath), Content: []byte(relativeFileContent)},
				{Path: dto.FilePath(absoluteFilePath), Content: []byte(absoluteFileContent)},
			},
		})
		s.Require().NoError(err)

		resp, err := sendCopyRequest(bytes.NewReader(testFilePathsCopyRequestString))
		s.NoError(err)
		s.Equal(http.StatusNoContent, resp.StatusCode)

		s.Run("File content of file with relative path can be printed on runner", func() {
			// the print command is executed in the context of the default working directory of the container
			s.assertFileContent(runnerID, relativeFilePath, relativeFileContent)
		})

		s.Run("File content of file with absolute path can be printed on runner", func() {
			s.assertFileContent(runnerID, absoluteFilePath, absoluteFileContent)
		})
	})

	s.Run("File deletion request deletes file on runner", func() {
		copyFilesRequestByteString, err := json.Marshal(&dto.UpdateFileSystemRequest{
			Delete: []dto.FilePath{tests.DefaultFileName},
		})
		s.Require().NoError(err)

		resp, err := sendCopyRequest(bytes.NewReader(copyFilesRequestByteString))
		s.NoError(err)
		s.Equal(http.StatusNoContent, resp.StatusCode)

		s.Run("File content can no longer be printed", func() {
			stdout, stderr := s.PrintContentOfFileOnRunner(runnerID, tests.DefaultFileName)
			s.Equal("", stdout)
			s.Contains(stderr, "No such file or directory")
		})
	})

	s.Run("File copy happens after file deletion", func() {
		copyFilesRequestByteString, err := json.Marshal(&dto.UpdateFileSystemRequest{
			Delete: []dto.FilePath{tests.DefaultFileName},
			Copy:   []dto.File{{Path: tests.DefaultFileName, Content: []byte(tests.DefaultFileContent)}},
		})
		s.Require().NoError(err)

		resp, err := sendCopyRequest(bytes.NewReader(copyFilesRequestByteString))
		s.NoError(err)
		s.Equal(http.StatusNoContent, resp.StatusCode)
		_ = resp.Body.Close()

		s.Run("File content can be printed on runner", func() {
			s.assertFileContent(runnerID, tests.DefaultFileName, tests.DefaultFileContent)
		})
	})

	s.Run("If one file produces permission denied error, others are still copied", func() {
		newFileContent := []byte("New content")
		copyFilesRequestByteString, err := json.Marshal(&dto.UpdateFileSystemRequest{
			Copy: []dto.File{
				{Path: "/dev/sda", Content: []byte(tests.DefaultFileContent)},
				{Path: tests.DefaultFileName, Content: newFileContent},
			},
		})
		s.Require().NoError(err)

		resp, err := sendCopyRequest(bytes.NewReader(copyFilesRequestByteString))
		s.NoError(err)
		s.Equal(http.StatusInternalServerError, resp.StatusCode)
		internalServerError := new(dto.InternalServerError)
		err = json.NewDecoder(resp.Body).Decode(internalServerError)
		s.NoError(err)
		s.Contains(internalServerError.Message, "Cannot open: Permission denied")
		_ = resp.Body.Close()

		s.Run("File content can be printed on runner", func() {
			s.assertFileContent(runnerID, tests.DefaultFileName, string(newFileContent))
		})
	})

	s.Run("File copy with invalid payload returns bad request", func() {
		resp, err := helpers.HTTPPatch(helpers.BuildURL(api.BasePath, api.RunnersPath, runnerID, api.UpdateFileSystemPath),
			"text/html", strings.NewReader(""))
		s.NoError(err)
		s.Equal(http.StatusBadRequest, resp.StatusCode)
	})

	s.Run("Copying to non-existing runner returns NotFound", func() {
		resp, err := helpers.HTTPPatch(
			helpers.BuildURL(api.BasePath, api.RunnersPath, tests.NonExistingStringID, api.UpdateFileSystemPath),
			"application/json", bytes.NewReader(copyFilesRequestByteString))
		s.NoError(err)
		s.Equal(http.StatusNotFound, resp.StatusCode)
	})
}

func (s *E2ETestSuite) TestRunnerGetsDestroyedAfterInactivityTimeout() {
	inactivityTimeout := 5 // seconds
	runnerID, err := ProvideRunner(&dto.RunnerRequest{
		ExecutionEnvironmentID: tests.DefaultEnvironmentIDAsInteger,
		InactivityTimeout:      inactivityTimeout,
	})
	s.Require().NoError(err)

	executionTerminated := make(chan bool)
	var lastMessage *dto.WebSocketMessage
	go func() {
		webSocketURL, err := ProvideWebSocketURL(&s.Suite, runnerID, &dto.ExecutionRequest{Command: "sleep infinity"})
		s.Require().NoError(err)
		connection, err := ConnectToWebSocket(webSocketURL)
		s.Require().NoError(err)

		messages, err := helpers.ReceiveAllWebSocketMessages(connection)
		if !s.Equal(&websocket.CloseError{Code: websocket.CloseNormalClosure}, err) {
			s.Fail("websocket abnormal closure")
		}
		controlMessages := helpers.WebSocketControlMessages(messages)
		s.Require().NotEmpty(controlMessages)
		lastMessage = controlMessages[len(controlMessages)-1]
		executionTerminated <- true
	}()
	s.Require().True(tests.ChannelReceivesSomething(executionTerminated, time.Duration(inactivityTimeout+5)*time.Second))
	s.Equal(dto.WebSocketMetaTimeout, lastMessage.Type)
}

func (s *E2ETestSuite) assertFileContent(runnerID, fileName, expectedContent string) {
	stdout, stderr := s.PrintContentOfFileOnRunner(runnerID, fileName)
	s.Equal(expectedContent, stdout)
	s.Equal("", stderr)
}

func (s *E2ETestSuite) PrintContentOfFileOnRunner(runnerID, filename string) (stdout, stderr string) {
	webSocketURL, err := ProvideWebSocketURL(&s.Suite, runnerID,
		&dto.ExecutionRequest{Command: fmt.Sprintf("cat %s", filename)})
	s.Require().NoError(err)
	connection, err := ConnectToWebSocket(webSocketURL)
	s.Require().NoError(err)

	messages, err := helpers.ReceiveAllWebSocketMessages(connection)
	s.Require().Error(err)
	s.Equal(&websocket.CloseError{Code: websocket.CloseNormalClosure}, err)

	stdout, stderr, _ = helpers.WebSocketOutputMessages(messages)
	return stdout, stderr
}
