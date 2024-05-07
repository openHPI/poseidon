package e2e

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/hashicorp/nomad/api"
	"github.com/openHPI/poseidon/internal/nomad"
	"github.com/openHPI/poseidon/pkg/dto"
	"github.com/openHPI/poseidon/pkg/logging"
	"github.com/openHPI/poseidon/tests"
	"github.com/openHPI/poseidon/tests/helpers"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

const (
	EnvPoseidonLogFile      = "POSEIDON_LOG_FILE"
	EnvPoseidonLogFormatter = "POSEIDON_LOGGER_FORMATTER"
)

func (s *E2ETestSuite) TestExecuteCommandRoute() {
	for _, environmentID := range environmentIDs {
		s.Run(environmentID.ToString(), func() {
			runnerID, err := ProvideRunner(&dto.RunnerRequest{ExecutionEnvironmentID: int(environmentID)})
			s.Require().NoError(err)

			webSocketURL, err := ProvideWebSocketURL(runnerID, &dto.ExecutionRequest{Command: "true"})
			s.Require().NoError(err)
			s.NotEqual("", webSocketURL)

			var connection *websocket.Conn
			var connectionClosed bool

			connection, err = ConnectToWebSocket(webSocketURL)
			s.Require().NoError(err, "websocket connects")
			closeHandler := connection.CloseHandler()
			connection.SetCloseHandler(func(code int, text string) error {
				connectionClosed = true
				return closeHandler(code, text)
			})

			startMessage, err := helpers.ReceiveNextWebSocketMessage(connection)
			s.Require().NoError(err)
			s.Equal(&dto.WebSocketMessage{Type: dto.WebSocketMetaStart}, startMessage)

			exitMessage, err := helpers.ReceiveNextWebSocketMessage(connection)
			s.Require().NoError(err)
			s.Equal(&dto.WebSocketMessage{Type: dto.WebSocketExit}, exitMessage)

			_, err = helpers.ReceiveAllWebSocketMessages(connection)
			s.Require().Error(err)
			s.Equal(&websocket.CloseError{Code: websocket.CloseNormalClosure}, err)

			_, _, err = connection.ReadMessage()
			s.True(websocket.IsCloseError(err, websocket.CloseNormalClosure))
			s.True(connectionClosed, "connection should be closed")
		})
	}
}

func (s *E2ETestSuite) TestOutputToStdout() {
	for _, environmentID := range environmentIDs {
		s.Run(environmentID.ToString(), func() {
			stdout, _, _ := ExecuteNonInteractive(&s.Suite, environmentID,
				&dto.ExecutionRequest{Command: "echo -n Hello World"}, nil)
			s.Require().Equal("Hello World", stdout)
		})
	}
}

func (s *E2ETestSuite) TestOutputToStderr() {
	for _, environmentID := range environmentIDs {
		s.Run(environmentID.ToString(), func() {
			stdout, stderr, exitCode := ExecuteNonInteractive(&s.Suite, environmentID,
				&dto.ExecutionRequest{Command: "cat -invalid"}, nil)

			s.NotContains(stdout, "cat: invalid option", "Stdout should not contain the error")
			s.Contains(stderr, "cat: invalid option", "Stderr should contain the error")
			s.Equal(uint8(1), exitCode)
		})
	}
}

func (s *E2ETestSuite) TestUserNomad() {
	s.Run("unprivileged", func() {
		stdout, _, _ := ExecuteNonInteractive(&s.Suite, tests.DefaultEnvironmentIDAsInteger,
			&dto.ExecutionRequest{Command: "id --name --user", PrivilegedExecution: false}, nil)
		s.Require().NotEqual("root", stdout)
	})
	s.Run("privileged", func() {
		stdout, _, _ := ExecuteNonInteractive(&s.Suite, tests.DefaultEnvironmentIDAsInteger,
			&dto.ExecutionRequest{Command: "id --name --user", PrivilegedExecution: true}, nil)
		s.Contains(stdout, "root")
	})
}

// AWS environments do not support stdin at this moment therefore they cannot take this test.
func (s *E2ETestSuite) TestCommandHead() {
	hello := "Hello World!"
	connection, err := ProvideWebSocketConnection(&s.Suite, tests.DefaultEnvironmentIDAsInteger,
		&dto.ExecutionRequest{Command: "head -n 1"}, nil)
	s.Require().NoError(err)

	startMessage, err := helpers.ReceiveNextWebSocketMessage(connection)
	s.Require().NoError(err)
	s.Equal(dto.WebSocketMetaStart, startMessage.Type)

	err = connection.WriteMessage(websocket.TextMessage, []byte(hello+"\n"))
	s.Require().NoError(err)

	messages, err := helpers.ReceiveAllWebSocketMessages(connection)
	s.Require().Error(err)
	s.Equal(&websocket.CloseError{Code: websocket.CloseNormalClosure}, err)
	stdout, _, _ := helpers.WebSocketOutputMessages(messages)
	s.Regexp(fmt.Sprintf(`(%s\r\n?){2}`, hello), stdout)
}

func (s *E2ETestSuite) TestCommandMake() {
	for _, environmentID := range environmentIDs {
		s.Run(environmentID.ToString(), func() {
			expectedOutput := "MeinText"
			request := &dto.UpdateFileSystemRequest{
				Copy: []dto.File{
					{Path: "Makefile", Content: []byte(
						"run:\n\t@echo " + expectedOutput + "\n\n" +
							"test:\n\t@echo Hi\n"),
					},
				},
			}

			stdout, _, _ := ExecuteNonInteractive(&s.Suite, environmentID, &dto.ExecutionRequest{Command: "make run"}, request)
			stdout = regexp.MustCompile(`\r?\n$`).ReplaceAllString(stdout, "")
			s.Equal(expectedOutput, stdout)
		})
	}
}

func (s *E2ETestSuite) TestEnvironmentVariables() {
	for _, environmentID := range environmentIDs {
		s.Run(environmentID.ToString(), func() {
			stdout, _, _ := ExecuteNonInteractive(&s.Suite, environmentID, &dto.ExecutionRequest{
				Command:     "env",
				Environment: map[string]string{"hello": "world"},
			}, nil)

			variables := s.expectEnvironmentVariables(stdout)
			s.Contains(variables, "hello=world")
		})
	}
}

func (s *E2ETestSuite) TestCommandMakeEnvironmentVariables() {
	for _, environmentID := range environmentIDs {
		s.Run(environmentID.ToString(), func() {
			request := &dto.UpdateFileSystemRequest{
				Copy: []dto.File{{Path: "Makefile", Content: []byte("run:\n\t@env\n")}},
			}

			stdout, _, _ := ExecuteNonInteractive(&s.Suite, environmentID, &dto.ExecutionRequest{Command: "make run"}, request)
			s.expectEnvironmentVariables(stdout)
		})
	}
}

func (s *E2ETestSuite) expectEnvironmentVariables(stdout string) []string {
	variables := strings.Split(strings.ReplaceAll(stdout, "\r\n", "\n"), "\n")
	s.Contains(variables, "CODEOCEAN=true")
	for _, envVar := range variables {
		s.False(strings.HasPrefix(envVar, "AWS"))
		s.False(strings.HasPrefix(envVar, "NOMAD_"))
	}
	return variables
}

func (s *E2ETestSuite) TestCommandReturnsAfterTimeout() {
	for _, environmentID := range environmentIDs {
		s.Run(environmentID.ToString(), func() {
			connection, err := ProvideWebSocketConnection(&s.Suite, environmentID,
				&dto.ExecutionRequest{Command: "sleep 4", TimeLimit: 1}, nil)
			s.Require().NoError(err)

			timeoutTestChannel := make(chan bool)
			var messages []*dto.WebSocketMessage
			go func() {
				messages, err = helpers.ReceiveAllWebSocketMessages(connection)
				if !s.Equal(&websocket.CloseError{Code: websocket.CloseNormalClosure}, err) {
					s.T().Fail()
				}
				close(timeoutTestChannel)
			}()

			select {
			case <-time.After(2 * time.Second):
				s.T().Fatal("The execution should have returned by now")
			case <-timeoutTestChannel:
				if s.Equal(&dto.WebSocketMessage{Type: dto.WebSocketMetaTimeout}, messages[len(messages)-1]) {
					return
				}
			}
			s.T().Fail()
		})
	}
}

func (s *E2ETestSuite) TestMemoryMaxLimit_Nomad() {
	maxMemoryLimit := defaultNomadEnvironment.MemoryLimit
	// The operating system is in charge to kill the process and sometimes tolerates small exceeding of the limit.
	maxMemoryLimit = uint(1.1 * float64(maxMemoryLimit))

	stdout, stderr, _ := ExecuteNonInteractive(&s.Suite, tests.DefaultEnvironmentIDAsInteger, &dto.ExecutionRequest{
		// This shell line tries to load maxMemoryLimit Bytes into the memory.
		Command: "</dev/zero head -c " + strconv.Itoa(int(maxMemoryLimit)) + "MB | tail > /dev/null",
	}, nil)
	s.Empty(stdout)
	s.Contains(stderr, "Killed")
}

func (s *E2ETestSuite) TestNomadStderrFifoIsRemoved() {
	runnerID, err := ProvideRunner(&dto.RunnerRequest{
		ExecutionEnvironmentID: tests.DefaultEnvironmentIDAsInteger,
	})
	s.Require().NoError(err)

	webSocketURL, err := ProvideWebSocketURL(runnerID, &dto.ExecutionRequest{Command: "ls -a /tmp/"})
	s.Require().NoError(err)
	connection, err := ConnectToWebSocket(webSocketURL)
	s.Require().NoError(err)

	messages, err := helpers.ReceiveAllWebSocketMessages(connection)
	s.Require().Error(err)
	s.Equal(&websocket.CloseError{Code: websocket.CloseNormalClosure}, err)

	stdout, _, _ := helpers.WebSocketOutputMessages(messages)
	s.Contains(stdout, ".fifo", "there should be a .fifo file during the execution")

	s.NotContains(s.ListTempDirectory(runnerID), ".fifo", "/tmp/ should not contain any .fifo files after the execution")
}

func (s *E2ETestSuite) TestTerminatedByClient() {
	logFile, logFileOk := os.LookupEnv(EnvPoseidonLogFile)
	logFormatter, logFormatterOk := os.LookupEnv(EnvPoseidonLogFormatter)
	if !logFileOk || !logFormatterOk || logFormatter != dto.FormatterJSON {
		s.T().Skipf("The environment variables %s and %s are not set", EnvPoseidonLogFile, EnvPoseidonLogFormatter)
		return
	}
	start := time.Now()

	// The bug of #325 is triggered in about every second execution. Therefore, we perform
	// 10 executions to have a high probability of triggering this (fixed) behavior.
	const runs = 10
	for i := 0; i < runs; i++ {
		<-time.After(time.Duration(i) * time.Second)
		log.WithField("i", i).Info("Run")
		runnerID, err := ProvideRunner(&dto.RunnerRequest{
			ExecutionEnvironmentID: tests.DefaultEnvironmentIDAsInteger,
		})
		s.Require().NoError(err)

		webSocketURL, err := ProvideWebSocketURL(runnerID, &dto.ExecutionRequest{Command: "sleep 2"})
		s.Require().NoError(err)
		connection, err := ConnectToWebSocket(webSocketURL)
		s.Require().NoError(err)

		go func() {
			<-time.After(time.Millisecond)
			err := connection.WriteControl(websocket.CloseMessage,
				websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""), time.Now().Add(time.Second))
			s.NoError(err)
			err = connection.Close()
			s.NoError(err)
		}()

		_, err = helpers.ReceiveAllWebSocketMessages(connection)
		s.Require().Error(err)
	}

	records := parseLogFile(s.T(), logFile, start, time.Now())
	for _, record := range records {
		msg, ok := record["msg"].(string)
		if !ok || msg == "Exec debug message could not be read completely" {
			s.Failf("Found Error", "Ok: %t, message: %s", ok, msg)
		}
	}
}

func parseLogFile(t *testing.T, name string, start time.Time, end time.Time) (logRecords []map[string]interface{}) {
	t.Helper()
	<-time.After(tests.ShortTimeout)
	file, err := os.Open(name)
	require.NoError(t, err)
	defer func(t *testing.T, file *os.File) {
		t.Helper()
		err := file.Close()
		require.NoError(t, err)
	}(t, file)
	fileScanner := bufio.NewScanner(file)
	fileScanner.Split(bufio.ScanLines)
	for fileScanner.Scan() {
		logRecord := map[string]interface{}{}
		err = json.Unmarshal(fileScanner.Bytes(), &logRecord)
		require.NoError(t, err)
		timeString, ok := logRecord["time"].(string)
		require.True(t, ok)
		entryTime, err := time.ParseInLocation(logging.TimestampFormat, timeString, start.Location())
		require.NoError(t, err)
		if entryTime.Before(start) || entryTime.After(end) {
			continue
		}
		logRecords = append(logRecords, logRecord)
	}
	return logRecords
}

func (s *E2ETestSuite) ListTempDirectory(runnerID string) string {
	allocListStub, _, err := nomadClient.Jobs().Allocations(runnerID, true, nil)
	s.Require().NoError(err)
	var runningAllocStub *api.AllocationListStub
	for _, stub := range allocListStub {
		if stub.ClientStatus == api.AllocClientStatusRunning && stub.DesiredStatus == api.AllocDesiredStatusRun {
			runningAllocStub = stub
			break
		}
	}
	alloc, _, err := nomadClient.Allocations().Info(runningAllocStub.ID, nil)
	s.Require().NoError(err)

	var stdout, stderr bytes.Buffer
	exit, err := nomadClient.Allocations().Exec(context.Background(), alloc, nomad.TaskName,
		false, []string{"ls", "-a", "/tmp/"}, strings.NewReader(""), &stdout, &stderr, nil, nil)

	s.Require().NoError(err)
	s.Require().Equal(0, exit)
	s.Require().Empty(stderr)
	return stdout.String()
}

// ExecuteNonInteractive Executes the passed executionRequest in the required environment without providing input.
func ExecuteNonInteractive(s *suite.Suite, environmentID dto.EnvironmentID, executionRequest *dto.ExecutionRequest,
	copyRequest *dto.UpdateFileSystemRequest) (stdout, stderr string, exitCode uint8) {
	connection, err := ProvideWebSocketConnection(s, environmentID, executionRequest, copyRequest)
	s.Require().NoError(err)

	startMessage, err := helpers.ReceiveNextWebSocketMessage(connection)
	s.Require().NoError(err)
	s.Equal(dto.WebSocketMetaStart, startMessage.Type)

	messages, err := helpers.ReceiveAllWebSocketMessages(connection)
	s.Require().Error(err)
	s.Equal(&websocket.CloseError{Code: websocket.CloseNormalClosure}, err)

	controlMessages := helpers.WebSocketControlMessages(messages)
	s.Require().Len(controlMessages, 1)
	exitMessage := controlMessages[0]
	s.Require().Equal(dto.WebSocketExit, exitMessage.Type)

	stdout, stderr, errors := helpers.WebSocketOutputMessages(messages)
	s.Empty(errors)
	return stdout, stderr, exitMessage.ExitCode
}

// ProvideWebSocketConnection establishes a client WebSocket connection to run the passed ExecutionRequest.
func ProvideWebSocketConnection(suite *suite.Suite, environmentID dto.EnvironmentID, executionRequest *dto.ExecutionRequest,
	copyRequest *dto.UpdateFileSystemRequest) (*websocket.Conn, error) {
	runnerID, err := ProvideRunner(&dto.RunnerRequest{ExecutionEnvironmentID: int(environmentID)})
	if err != nil {
		return nil, fmt.Errorf("error providing runner: %w", err)
	}
	if copyRequest != nil {
		resp, err := CopyFiles(runnerID, copyRequest)
		suite.Require().NoError(err)
		suite.Require().Equal(http.StatusNoContent, resp.StatusCode)
	}
	webSocketURL, err := ProvideWebSocketURL(runnerID, executionRequest)
	suite.Require().NoError(err)
	connection, err := ConnectToWebSocket(webSocketURL)
	if err != nil {
		return nil, fmt.Errorf("error connecting to WebSocket: %w", err)
	}
	return connection, nil
}

// ConnectToWebSocket establish an external WebSocket connection to the provided url.
// It requires a running Poseidon instance.
func ConnectToWebSocket(url string) (conn *websocket.Conn, err error) {
	conn, _, err = websocket.DefaultDialer.Dial(url, nil)
	return
}
