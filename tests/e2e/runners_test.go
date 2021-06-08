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
)

func (s *E2ETestSuite) TestProvideRunnerRoute() {
	runnerRequestString, _ := json.Marshal(dto.RunnerRequest{})
	reader := strings.NewReader(string(runnerRequestString))
	resp, err := http.Post(helpers.BuildURL(api.BasePath, api.RunnersPath), "application/json", reader)
	s.NoError(err)
	s.Equal(http.StatusOK, resp.StatusCode, "The response code should be ok")

	runnerResponse := new(dto.RunnerResponse)
	err = json.NewDecoder(resp.Body).Decode(runnerResponse)
	s.NoError(err)

	s.True(runnerResponse.Id != "", "The response contains a runner id")
}

// ProvideRunner creates a runner with the given RunnerRequest via an external request.
// It needs a running Poseidon instance to work.
func ProvideRunner(request *dto.RunnerRequest) (string, error) {
	url := helpers.BuildURL(api.BasePath, api.RunnersPath)
	runnerRequestString, _ := json.Marshal(request)
	reader := strings.NewReader(string(runnerRequestString))
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
	runnerId, err := ProvideRunner(&dto.RunnerRequest{})
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
		resp, err := helpers.HttpDelete(helpers.BuildURL(api.BasePath, api.RunnersPath, tests.NonExistingId), nil)
		s.NoError(err)
		s.Equal(http.StatusNotFound, resp.StatusCode)
	})
}

func (s *E2ETestSuite) TestCopyFilesRoute() {
	runnerID, err := ProvideRunner(&dto.RunnerRequest{})
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
			s.Equal(tests.DefaultFileContent, s.PrintContentOfFileOnRunner(runnerID, tests.DefaultFileName))
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
			s.Equal(relativeFileContent, s.PrintContentOfFileOnRunner(runnerID, relativeFilePath))
		})

		s.Run("File content of file with absolute path can be printed on runner", func() {
			s.Equal(absoluteFileContent, s.PrintContentOfFileOnRunner(runnerID, absoluteFilePath))
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
			s.Contains(s.PrintContentOfFileOnRunner(runnerID, tests.DefaultFileName), "No such file or directory")
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
			s.Equal(tests.DefaultFileContent, s.PrintContentOfFileOnRunner(runnerID, tests.DefaultFileName))
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
			s.Equal(string(newFileContent), s.PrintContentOfFileOnRunner(runnerID, tests.DefaultFileName))
		})
	})

	s.Run("File copy with invalid payload returns bad request", func() {
		resp, err := helpers.HttpPatch(helpers.BuildURL(api.BasePath, api.RunnersPath, runnerID, api.UpdateFileSystemPath), "text/html", strings.NewReader(""))
		s.NoError(err)
		s.Equal(http.StatusBadRequest, resp.StatusCode)
	})

	s.Run("Copying to non-existing runner returns NotFound", func() {
		resp, err := helpers.HttpPatch(helpers.BuildURL(api.BasePath, api.RunnersPath, tests.NonExistingId, api.UpdateFileSystemPath), "application/json", bytes.NewReader(copyFilesRequestByteString))
		s.NoError(err)
		s.Equal(http.StatusNotFound, resp.StatusCode)
	})
}

func (s *E2ETestSuite) PrintContentOfFileOnRunner(runnerId string, filename string) string {
	webSocketURL, _ := ProvideWebSocketURL(&s.Suite, runnerId, &dto.ExecutionRequest{Command: fmt.Sprintf("cat %s", filename)})
	connection, _ := ConnectToWebSocket(webSocketURL)

	messages, err := helpers.ReceiveAllWebSocketMessages(connection)
	s.Require().Error(err)
	s.Equal(&websocket.CloseError{Code: websocket.CloseNormalClosure}, err)

	stdout, _, _ := helpers.WebSocketOutputMessages(messages)
	return stdout
}
