package e2e

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/gorilla/websocket"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/api"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/api/dto"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/tests"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/tests/helpers"
	"io"
	"net/http"
	"strings"
	"time"
)

func (s *E2ETestSuite) TestProvideRunnerRoute() {
	runnerRequestByteString, _ := json.Marshal(dto.RunnerRequest{
		ExecutionEnvironmentId: tests.DefaultEnvironmentIDAsInteger,
	})
	reader := bytes.NewReader(runnerRequestByteString)

	s.Run("valid request returns a runner", func() {
		resp, err := http.Post(helpers.BuildURL(api.BasePath, api.RunnersPath), "application/json", reader)
		s.Require().NoError(err)
		s.Equal(http.StatusOK, resp.StatusCode)

		runnerResponse := new(dto.RunnerResponse)
		err = json.NewDecoder(resp.Body).Decode(runnerResponse)
		s.Require().NoError(err)
		s.NotEmpty(runnerResponse.Id)
	})

	s.Run("invalid request returns bad request", func() {
		resp, err := http.Post(helpers.BuildURL(api.BasePath, api.RunnersPath), "application/json", strings.NewReader(""))
		s.Require().NoError(err)
		s.Equal(http.StatusBadRequest, resp.StatusCode)
	})

	s.Run("requesting runner of unknown execution environment returns not found", func() {
		runnerRequestByteString, _ := json.Marshal(dto.RunnerRequest{
			ExecutionEnvironmentId: tests.NonExistingIntegerID,
		})
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
	runnerRequestByteString, _ := json.Marshal(request)
	reader := strings.NewReader(string(runnerRequestByteString))
	resp, err := http.Post(url, "application/json", reader)
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("expected response code 200 when getting runner, got %v", resp.StatusCode)
	}
	runnerResponse := new(dto.RunnerResponse)
	err = json.NewDecoder(resp.Body).Decode(runnerResponse)
	if err != nil {
		return "", err
	}
	return runnerResponse.Id, nil
}

func (s *E2ETestSuite) TestDeleteRunnerRoute() {
	runnerId, err := ProvideRunner(&dto.RunnerRequest{
		ExecutionEnvironmentId: tests.DefaultEnvironmentIDAsInteger,
	})
	s.NoError(err)

	s.Run("Deleting the runner returns NoContent", func() {
		resp, err := helpers.HttpDelete(helpers.BuildURL(api.BasePath, api.RunnersPath, runnerId), nil)
		s.NoError(err)
		s.Equal(http.StatusNoContent, resp.StatusCode)
	})

	s.Run("Deleting it again returns NotFound", func() {
		resp, err := helpers.HttpDelete(helpers.BuildURL(api.BasePath, api.RunnersPath, runnerId), nil)
		s.NoError(err)
		s.Equal(http.StatusNotFound, resp.StatusCode)
	})

	s.Run("Deleting non-existing runner returns NotFound", func() {
		resp, err := helpers.HttpDelete(helpers.BuildURL(api.BasePath, api.RunnersPath, tests.NonExistingStringID), nil)
		s.NoError(err)
		s.Equal(http.StatusNotFound, resp.StatusCode)
	})
}

func (s *E2ETestSuite) TestCopyFilesRoute() {
	runnerID, err := ProvideRunner(&dto.RunnerRequest{
		ExecutionEnvironmentId: tests.DefaultEnvironmentIDAsInteger,
	})
	s.NoError(err)
	copyFilesRequestByteString, _ := json.Marshal(&dto.UpdateFileSystemRequest{
		Copy: []dto.File{{Path: tests.DefaultFileName, Content: []byte(tests.DefaultFileContent)}},
	})
	sendCopyRequest := func(reader io.Reader) (*http.Response, error) {
		return helpers.HttpPatch(helpers.BuildURL(api.BasePath, api.RunnersPath, runnerID, api.UpdateFileSystemPath), "application/json", reader)
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
		testFilePathsCopyRequestString, _ := json.Marshal(&dto.UpdateFileSystemRequest{
			Copy: []dto.File{
				{Path: dto.FilePath(relativeFilePath), Content: []byte(relativeFileContent)},
				{Path: dto.FilePath(absoluteFilePath), Content: []byte(absoluteFileContent)},
			},
		})

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
		copyFilesRequestByteString, _ := json.Marshal(&dto.UpdateFileSystemRequest{
			Delete: []dto.FilePath{tests.DefaultFileName},
		})

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
		copyFilesRequestByteString, _ := json.Marshal(&dto.UpdateFileSystemRequest{
			Delete: []dto.FilePath{tests.DefaultFileName},
			Copy:   []dto.File{{Path: tests.DefaultFileName, Content: []byte(tests.DefaultFileContent)}},
		})

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
		copyFilesRequestByteString, _ := json.Marshal(&dto.UpdateFileSystemRequest{
			Copy: []dto.File{
				{Path: "/dev/sda", Content: []byte(tests.DefaultFileContent)},
				{Path: tests.DefaultFileName, Content: newFileContent},
			},
		})

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
		resp, err := helpers.HttpPatch(helpers.BuildURL(api.BasePath, api.RunnersPath, runnerID, api.UpdateFileSystemPath), "text/html", strings.NewReader(""))
		s.NoError(err)
		s.Equal(http.StatusBadRequest, resp.StatusCode)
	})

	s.Run("Copying to non-existing runner returns NotFound", func() {
		resp, err := helpers.HttpPatch(helpers.BuildURL(api.BasePath, api.RunnersPath, tests.NonExistingStringID, api.UpdateFileSystemPath), "application/json", bytes.NewReader(copyFilesRequestByteString))
		s.NoError(err)
		s.Equal(http.StatusNotFound, resp.StatusCode)
	})
}

func (s *E2ETestSuite) TestRunnerGetsDestroyedAfterInactivityTimeout() {
	inactivityTimeout := 5 // seconds
	runnerID, err := ProvideRunner(&dto.RunnerRequest{
		ExecutionEnvironmentId: tests.DefaultEnvironmentIDAsInteger,
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

func (s *E2ETestSuite) assertFileContent(runnerID, fileName string, expectedContent string) {
	stdout, stderr := s.PrintContentOfFileOnRunner(runnerID, fileName)
	s.Equal(expectedContent, stdout)
	s.Equal("", stderr)
}

func (s *E2ETestSuite) PrintContentOfFileOnRunner(runnerId string, filename string) (string, string) {
	webSocketURL, _ := ProvideWebSocketURL(&s.Suite, runnerId, &dto.ExecutionRequest{Command: fmt.Sprintf("cat %s", filename)})
	connection, _ := ConnectToWebSocket(webSocketURL)

	messages, err := helpers.ReceiveAllWebSocketMessages(connection)
	s.Require().Error(err)
	s.Equal(&websocket.CloseError{Code: websocket.CloseNormalClosure}, err)

	stdout, stderr, _ := helpers.WebSocketOutputMessages(messages)
	return stdout, stderr
}
