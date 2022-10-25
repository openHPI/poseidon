package e2e

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/gorilla/websocket"
	"github.com/openHPI/poseidon/internal/api"
	"github.com/openHPI/poseidon/pkg/dto"
	"github.com/openHPI/poseidon/tests"
	"github.com/openHPI/poseidon/tests/helpers"
	"github.com/stretchr/testify/require"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

func (s *E2ETestSuite) TestProvideRunnerRoute() {
	for _, environmentID := range environmentIDs {
		s.Run(environmentID.ToString(), func() {
			runnerRequestByteString, err := json.Marshal(dto.RunnerRequest{ExecutionEnvironmentID: int(environmentID)})
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
		})
	}
}

// ProvideRunner creates a runner with the given RunnerRequest via an external request.
// It needs a running Poseidon instance to work.
func ProvideRunner(request *dto.RunnerRequest) (string, error) {
	runnerURL := helpers.BuildURL(api.BasePath, api.RunnersPath)
	runnerRequestByteString, err := json.Marshal(request)
	if err != nil {
		return "", err
	}
	reader := strings.NewReader(string(runnerRequestByteString))
	resp, err := http.Post(runnerURL, "application/json", reader) //nolint:gosec // runnerURL is not influenced by a user
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

// CopyFiles sends the dto.UpdateFileSystemRequest to Poseidon.
// It needs a running Poseidon instance to work.
func CopyFiles(runnerID string, request *dto.UpdateFileSystemRequest) (*http.Response, error) {
	data, err := json.Marshal(request)
	if err != nil {
		return nil, err
	}
	r := bytes.NewReader(data)
	return helpers.HTTPPatch(helpers.BuildURL(api.BasePath, api.RunnersPath, runnerID, api.UpdateFileSystemPath),
		"application/json", r)
}

func (s *E2ETestSuite) TestDeleteRunnerRoute() {
	for _, environmentID := range environmentIDs {
		s.Run(environmentID.ToString(), func() {
			runnerID, err := ProvideRunner(&dto.RunnerRequest{ExecutionEnvironmentID: int(environmentID)})
			s.NoError(err)

			s.Run("Deleting the runner returns NoContent", func() {
				resp, err := helpers.HTTPDelete(helpers.BuildURL(api.BasePath, api.RunnersPath, runnerID), nil)
				s.NoError(err)
				s.Equal(http.StatusNoContent, resp.StatusCode)
			})

			s.Run("Deleting it again returns Gone", func() {
				resp, err := helpers.HTTPDelete(helpers.BuildURL(api.BasePath, api.RunnersPath, runnerID), nil)
				s.NoError(err)
				s.Equal(http.StatusGone, resp.StatusCode)
			})

			s.Run("Deleting non-existing runner returns Gone", func() {
				resp, err := helpers.HTTPDelete(helpers.BuildURL(api.BasePath, api.RunnersPath, tests.NonExistingStringID), nil)
				s.NoError(err)
				s.Equal(http.StatusGone, resp.StatusCode)
			})
		})
	}
}

func (s *E2ETestSuite) TestListFileSystem_Nomad() {
	runnerID, err := ProvideRunner(&dto.RunnerRequest{
		ExecutionEnvironmentID: tests.DefaultEnvironmentIDAsInteger,
	})
	require.NoError(s.T(), err)

	s.Run("No files", func() {
		getFileURL, err := url.Parse(helpers.BuildURL(api.BasePath, api.RunnersPath, runnerID, api.UpdateFileSystemPath))
		s.Require().NoError(err)
		response, err := http.Get(getFileURL.String())
		s.Require().NoError(err)
		s.Equal(http.StatusOK, response.StatusCode)
		data, err := io.ReadAll(response.Body)
		s.NoError(err)
		s.Equal("{\"files\": []}", string(data))
	})

	s.Run("With file", func() {
		resp, err := CopyFiles(runnerID, &dto.UpdateFileSystemRequest{
			Copy: []dto.File{{Path: tests.DefaultFileName, Content: []byte{}}},
		})
		s.Require().NoError(err)
		s.Equal(http.StatusNoContent, resp.StatusCode)

		getFileURL, err := url.Parse(helpers.BuildURL(api.BasePath, api.RunnersPath, runnerID, api.UpdateFileSystemPath))
		s.Require().NoError(err)
		response, err := http.Get(getFileURL.String())
		s.Require().NoError(err)
		s.Equal(http.StatusOK, response.StatusCode)

		listFilesResponse := new(dto.ListFileSystemResponse)
		err = json.NewDecoder(response.Body).Decode(listFilesResponse)
		s.Require().NoError(err)
		s.Require().Equal(len(listFilesResponse.Files), 1)
		fileHeader := listFilesResponse.Files[0]
		s.Equal(dto.FilePath("./"+tests.DefaultFileName), fileHeader.Name)
		s.Equal(dto.EntryTypeRegularFile, fileHeader.EntryType)
		// ToDo: Reconsider if those files should be owned by root.
		s.Equal("root", fileHeader.Owner)
		s.Equal("root", fileHeader.Group)
		s.Equal("rwxr--r--", fileHeader.Permissions)
	})
}

//nolint:funlen // there are a lot of tests for the files route, this function can be a little longer than 100 lines ;)
func (s *E2ETestSuite) TestCopyFilesRoute() {
	for _, environmentID := range environmentIDs {
		s.Run(environmentID.ToString(), func() {
			runnerID, err := ProvideRunner(&dto.RunnerRequest{ExecutionEnvironmentID: int(environmentID)})
			s.NoError(err)

			request := &dto.UpdateFileSystemRequest{
				Copy: []dto.File{{Path: tests.DefaultFileName, Content: []byte(tests.DefaultFileContent)}},
			}

			s.Run("File copy with valid payload succeeds", func() {
				resp, err := CopyFiles(runnerID, request)
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
				request = &dto.UpdateFileSystemRequest{
					Copy: []dto.File{
						{Path: dto.FilePath(relativeFilePath), Content: []byte(relativeFileContent)},
						{Path: dto.FilePath(absoluteFilePath), Content: []byte(absoluteFileContent)},
					},
				}
				s.Require().NoError(err)

				resp, err := CopyFiles(runnerID, request)
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
				request = &dto.UpdateFileSystemRequest{
					Delete: []dto.FilePath{tests.DefaultFileName},
				}
				s.Require().NoError(err)

				resp, err := CopyFiles(runnerID, request)
				s.NoError(err)
				s.Equal(http.StatusNoContent, resp.StatusCode)

				s.Run("File content can no longer be printed", func() {
					stdout, stderr := s.PrintContentOfFileOnRunner(runnerID, tests.DefaultFileName)
					s.Equal("", stdout)
					s.Contains(stderr, "No such file or directory")
				})
			})

			s.Run("File copy happens after file deletion", func() {
				request = &dto.UpdateFileSystemRequest{
					Delete: []dto.FilePath{tests.DefaultFileName},
					Copy:   []dto.File{{Path: tests.DefaultFileName, Content: []byte(tests.DefaultFileContent)}},
				}
				s.Require().NoError(err)

				resp, err := CopyFiles(runnerID, request)
				s.NoError(err)
				s.Equal(http.StatusNoContent, resp.StatusCode)
				_ = resp.Body.Close()

				s.Run("File content can be printed on runner", func() {
					s.assertFileContent(runnerID, tests.DefaultFileName, tests.DefaultFileContent)
				})
			})

			s.Run("File copy with invalid payload returns bad request", func() {
				resp, err := helpers.HTTPPatch(helpers.BuildURL(api.BasePath, api.RunnersPath, runnerID, api.UpdateFileSystemPath),
					"text/html", strings.NewReader(""))
				s.NoError(err)
				s.Equal(http.StatusBadRequest, resp.StatusCode)
			})

			s.Run("Copying to non-existing runner returns Gone", func() {
				resp, err := CopyFiles(tests.NonExistingStringID, request)
				s.NoError(err)
				s.Equal(http.StatusGone, resp.StatusCode)
			})
		})
	}
}

func (s *E2ETestSuite) TestCopyFilesRoute_PermissionDenied() {
	s.Run("Nomad/If one file produces permission denied error, others are still copied", func() {
		runnerID, err := ProvideRunner(&dto.RunnerRequest{
			ExecutionEnvironmentID: tests.DefaultEnvironmentIDAsInteger,
		})
		s.NoError(err)

		newFileContent := []byte("New content")
		copyFilesRequestByteString, err := json.Marshal(&dto.UpdateFileSystemRequest{
			Copy: []dto.File{
				{Path: "/proc/1/environ", Content: []byte(tests.DefaultFileContent)},
				{Path: tests.DefaultFileName, Content: newFileContent},
			},
		})
		s.Require().NoError(err)

		resp, err := helpers.HTTPPatch(helpers.BuildURL(api.BasePath, api.RunnersPath, runnerID, api.UpdateFileSystemPath),
			"application/json", bytes.NewReader(copyFilesRequestByteString))
		s.NoError(err)
		s.Equal(http.StatusInternalServerError, resp.StatusCode)
		internalServerError := new(dto.InternalServerError)
		err = json.NewDecoder(resp.Body).Decode(internalServerError)
		s.NoError(err)
		s.Contains(internalServerError.Message, "Cannot open: ")
		_ = resp.Body.Close()

		s.Run("File content can be printed on runner", func() {
			s.assertFileContent(runnerID, tests.DefaultFileName, string(newFileContent))
		})
	})

	s.Run("AWS/If one file produces permission denied error, others are still copied", func() {
		for _, environmentID := range environmentIDs {
			if environmentID == tests.DefaultEnvironmentIDAsInteger {
				continue
			}
			s.Run(environmentID.ToString(), func() {
				runnerID, err := ProvideRunner(&dto.RunnerRequest{ExecutionEnvironmentID: int(environmentID)})
				s.NoError(err)

				newFileContent := []byte("New content")
				copyFilesRequestByteString, err := json.Marshal(&dto.UpdateFileSystemRequest{
					Copy: []dto.File{
						{Path: "/proc/1/environ", Content: []byte(tests.DefaultFileContent)},
						{Path: tests.DefaultFileName, Content: newFileContent},
					},
				})
				s.Require().NoError(err)

				resp, err := helpers.HTTPPatch(helpers.BuildURL(api.BasePath, api.RunnersPath, runnerID, api.UpdateFileSystemPath),
					"application/json", bytes.NewReader(copyFilesRequestByteString))
				s.NoError(err)
				s.Equal(http.StatusNoContent, resp.StatusCode)
				_ = resp.Body.Close()

				stdout, stderr := s.PrintContentOfFileOnRunner(runnerID, tests.DefaultFileName)
				s.Equal(string(newFileContent), stdout)
				s.Contains(stderr, "Exception")
			})
		}
	})
}

func (s *E2ETestSuite) TestCopyFilesRoute_ProtectedFolders() {
	runnerID, err := ProvideRunner(&dto.RunnerRequest{ExecutionEnvironmentID: tests.DefaultEnvironmentIDAsInteger})
	s.NoError(err)

	// Initialization of protected folder
	newFileContent := []byte("New content")
	protectedFolderPath := dto.FilePath("protectedFolder/")
	copyFilesRequestByteString, err := json.Marshal(&dto.UpdateFileSystemRequest{
		Copy: []dto.File{
			{Path: protectedFolderPath},
			{Path: protectedFolderPath + tests.DefaultFileName, Content: newFileContent},
		},
	})
	s.Require().NoError(err)

	resp, err := helpers.HTTPPatch(helpers.BuildURL(api.BasePath, api.RunnersPath, runnerID, api.UpdateFileSystemPath),
		"application/json", bytes.NewReader(copyFilesRequestByteString))
	s.NoError(err)
	s.Equal(http.StatusNoContent, resp.StatusCode)

	// User manipulates protected folder
	s.Run("User can create files", func() {
		webSocketURL, err := ProvideWebSocketURL(&s.Suite, runnerID, &dto.ExecutionRequest{
			Command:             fmt.Sprintf("touch %s/userfile", protectedFolderPath),
			TimeLimit:           int(tests.DefaultTestTimeout.Seconds()),
			PrivilegedExecution: false,
		})
		s.Require().NoError(err)
		connection, err := ConnectToWebSocket(webSocketURL)
		s.Require().NoError(err)
		messages, err := helpers.ReceiveAllWebSocketMessages(connection)
		s.Require().Error(err)
		s.Equal(&websocket.CloseError{Code: websocket.CloseNormalClosure}, err)
		stdout, stderr, _ := helpers.WebSocketOutputMessages(messages)
		s.Empty(stdout)
		s.Empty(stderr)
	})

	s.Run("User can not delete protected folder", func() {
		webSocketURL, err := ProvideWebSocketURL(&s.Suite, runnerID, &dto.ExecutionRequest{
			Command:             fmt.Sprintf("rm -fr %s", protectedFolderPath),
			TimeLimit:           int(tests.DefaultTestTimeout.Seconds()),
			PrivilegedExecution: false,
		})
		s.Require().NoError(err)
		connection, err := ConnectToWebSocket(webSocketURL)
		s.Require().NoError(err)
		messages, err := helpers.ReceiveAllWebSocketMessages(connection)
		s.Require().Error(err)
		s.Equal(&websocket.CloseError{Code: websocket.CloseNormalClosure}, err)
		stdout, stderr, _ := helpers.WebSocketOutputMessages(messages)
		s.Empty(stdout)
		s.Contains(stderr, "Operation not permitted")
	})

	s.Run("User can not delete protected file", func() {
		webSocketURL, err := ProvideWebSocketURL(&s.Suite, runnerID, &dto.ExecutionRequest{
			Command:             fmt.Sprintf("rm -f %s", protectedFolderPath+tests.DefaultFileName),
			TimeLimit:           int(tests.DefaultTestTimeout.Seconds()),
			PrivilegedExecution: false,
		})
		s.Require().NoError(err)
		connection, err := ConnectToWebSocket(webSocketURL)
		s.Require().NoError(err)
		messages, err := helpers.ReceiveAllWebSocketMessages(connection)
		s.Require().Error(err)
		s.Equal(&websocket.CloseError{Code: websocket.CloseNormalClosure}, err)
		stdout, stderr, _ := helpers.WebSocketOutputMessages(messages)
		s.Empty(stdout)
		s.Contains(stderr, "Operation not permitted")
	})

	s.Run("User can not write protected file", func() {
		webSocketURL, err := ProvideWebSocketURL(&s.Suite, runnerID, &dto.ExecutionRequest{
			Command:             fmt.Sprintf("echo Hi >> %s", protectedFolderPath+tests.DefaultFileName),
			TimeLimit:           int(tests.DefaultTestTimeout.Seconds()),
			PrivilegedExecution: false,
		})
		s.Require().NoError(err)
		connection, err := ConnectToWebSocket(webSocketURL)
		s.Require().NoError(err)
		messages, err := helpers.ReceiveAllWebSocketMessages(connection)
		s.Require().Error(err)
		s.Equal(&websocket.CloseError{Code: websocket.CloseNormalClosure}, err)
		stdout, stderr, _ := helpers.WebSocketOutputMessages(messages)
		s.Empty(stdout)
		s.Contains(stderr, "Permission denied")
	})

	s.Run("File content is not manipulated", func() {
		s.assertFileContent(runnerID, string(protectedFolderPath+tests.DefaultFileName), string(newFileContent))
	})
}

func (s *E2ETestSuite) TestGetFileContent_Nomad() {
	runnerID, err := ProvideRunner(&dto.RunnerRequest{
		ExecutionEnvironmentID: tests.DefaultEnvironmentIDAsInteger,
	})
	require.NoError(s.T(), err)

	s.Run("Not Found", func() {
		getFileURL, err := url.Parse(helpers.BuildURL(api.BasePath, api.RunnersPath, runnerID, api.FileContentRawPath))
		s.Require().NoError(err)
		getFileURL.RawQuery = fmt.Sprintf("%s=%s", api.PathKey, tests.DefaultFileName)
		response, err := http.Get(getFileURL.String())
		s.Require().NoError(err)
		s.Equal(http.StatusFailedDependency, response.StatusCode)
	})

	s.Run("Ok", func() {
		newFileContent := []byte("New content")
		resp, err := CopyFiles(runnerID, &dto.UpdateFileSystemRequest{
			Copy: []dto.File{{Path: tests.DefaultFileName, Content: newFileContent}},
		})
		s.Require().NoError(err)
		s.Equal(http.StatusNoContent, resp.StatusCode)

		getFileURL, err := url.Parse(helpers.BuildURL(api.BasePath, api.RunnersPath, runnerID, api.FileContentRawPath))
		s.Require().NoError(err)
		getFileURL.RawQuery = fmt.Sprintf("%s=%s", api.PathKey, tests.DefaultFileName)
		response, err := http.Get(getFileURL.String())
		s.Require().NoError(err)
		s.Equal(http.StatusOK, response.StatusCode)
		s.Equal(strconv.Itoa(len(newFileContent)), response.Header.Get("Content-Length"))
		s.Equal("attachment; filename=\""+tests.DefaultFileName+"\"", response.Header.Get("Content-Disposition"))
		content, err := io.ReadAll(response.Body)
		s.Require().NoError(err)
		s.Equal(newFileContent, content)
	})
}

func (s *E2ETestSuite) TestRunnerGetsDestroyedAfterInactivityTimeout() {
	for _, environmentID := range environmentIDs {
		s.Run(environmentID.ToString(), func() {
			inactivityTimeout := 2 // seconds
			runnerID, err := ProvideRunner(&dto.RunnerRequest{
				ExecutionEnvironmentID: int(environmentID),
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
				log.Warn("")
				executionTerminated <- true
			}()
			s.Require().True(tests.ChannelReceivesSomething(executionTerminated, time.Duration(inactivityTimeout+5)*time.Second))
			s.Equal(dto.WebSocketMetaTimeout, lastMessage.Type)
		})
	}
}

func (s *E2ETestSuite) assertFileContent(runnerID, fileName, expectedContent string) {
	stdout, stderr := s.PrintContentOfFileOnRunner(runnerID, fileName)
	s.Equal(expectedContent, stdout)
	s.Equal("", stderr)
}

func (s *E2ETestSuite) PrintContentOfFileOnRunner(runnerID, filename string) (stdout, stderr string) {
	webSocketURL, err := ProvideWebSocketURL(&s.Suite, runnerID, &dto.ExecutionRequest{
		Command:   fmt.Sprintf("cat %s", filename),
		TimeLimit: int(tests.DefaultTestTimeout.Seconds()),
	})
	s.Require().NoError(err)
	connection, err := ConnectToWebSocket(webSocketURL)
	s.Require().NoError(err)

	messages, err := helpers.ReceiveAllWebSocketMessages(connection)
	s.Require().Error(err)
	s.Equal(&websocket.CloseError{Code: websocket.CloseNormalClosure}, err)

	stdout, stderr, _ = helpers.WebSocketOutputMessages(messages)
	return stdout, stderr
}
