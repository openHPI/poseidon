package dto

import (
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"strconv"
	"strings"
)

// RunnerRequest is the expected json structure of the request body for the ProvideRunner function.
type RunnerRequest struct {
	ExecutionEnvironmentID int `json:"executionEnvironmentId"`
	InactivityTimeout      int `json:"inactivityTimeout"`
}

// ExecutionRequest is the expected json structure of the request body for the ExecuteCommand function.
type ExecutionRequest struct {
	Command     string
	TimeLimit   int
	Environment map[string]string
}

func (er *ExecutionRequest) FullCommand() []string {
	command := make([]string, 0)
	command = append(command, "env", "-")
	for variable, value := range er.Environment {
		command = append(command, fmt.Sprintf("%s=%s", variable, value))
	}
	command = append(command, "sh", "-c", er.Command)
	return command
}

// EnvironmentID is an id of an environment.
type EnvironmentID int

// NewEnvironmentID parses a string into an EnvironmentID.
func NewEnvironmentID(id string) (EnvironmentID, error) {
	environment, err := strconv.Atoi(id)
	return EnvironmentID(environment), err
}

// ToString pareses an EnvironmentID back to a string.
func (e EnvironmentID) ToString() string {
	return strconv.Itoa(int(e))
}

// ExecutionEnvironmentData is the expected json structure of the response body
// for routes returning an execution environment.
type ExecutionEnvironmentData struct {
	ExecutionEnvironmentRequest
	ID int `json:"id"`
}

// ExecutionEnvironmentRequest is the expected json structure of the request body
// for the create execution environment function.
type ExecutionEnvironmentRequest struct {
	PrewarmingPoolSize uint     `json:"prewarmingPoolSize"`
	CPULimit           uint     `json:"cpuLimit"`
	MemoryLimit        uint     `json:"memoryLimit"`
	Image              string   `json:"image"`
	NetworkAccess      bool     `json:"networkAccess"`
	ExposedPorts       []uint16 `json:"exposedPorts"`
}

// MappedPort contains the mapping from exposed port inside the container to the host address
// outside the container.
type MappedPort struct {
	ExposedPort uint   `json:"exposedPort"`
	HostAddress string `json:"hostAddress"`
}

// RunnerResponse is the expected response when providing a runner.
type RunnerResponse struct {
	ID          string        `json:"runnerId"`
	MappedPorts []*MappedPort `json:"mappedPorts"`
}

// ExecutionResponse is the expected response when creating an execution for a runner.
type ExecutionResponse struct {
	WebSocketURL string `json:"websocketUrl"`
}

// UpdateFileSystemRequest is the expected json structure of the request body for the update file system route.
type UpdateFileSystemRequest struct {
	Delete []FilePath `json:"delete"`
	Copy   []File     `json:"copy"`
}

// FilePath specifies the path of a file and is part of the UpdateFileSystemRequest.
type FilePath string

// File is a DTO for transmitting file contents. It is part of the UpdateFileSystemRequest.
type File struct {
	Path    FilePath `json:"path"`
	Content []byte   `json:"content"`
}

// Cleaned returns the cleaned path of the FilePath.
func (f FilePath) Cleaned() string {
	return path.Clean(string(f))
}

// CleanedPath returns the cleaned path of the file.
func (f File) CleanedPath() string {
	return f.Path.Cleaned()
}

// IsDirectory returns true iff the path of the File ends with a /.
func (f File) IsDirectory() bool {
	return strings.HasSuffix(string(f.Path), "/")
}

// ByteContent returns the content of the File. If the File is a directory, the content will be empty.
func (f File) ByteContent() []byte {
	if f.IsDirectory() {
		return []byte("")
	} else {
		return f.Content
	}
}

// WebSocketMessageType is the type for the messages from Poseidon to the client.
type WebSocketMessageType string

const (
	WebSocketOutputStdout WebSocketMessageType = "stdout"
	WebSocketOutputStderr WebSocketMessageType = "stderr"
	WebSocketOutputError  WebSocketMessageType = "error"
	WebSocketMetaStart    WebSocketMessageType = "start"
	WebSocketMetaTimeout  WebSocketMessageType = "timeout"
	WebSocketExit         WebSocketMessageType = "exit"
)

var (
	ErrUnknownWebSocketMessageType = errors.New("unknown WebSocket message type")
	ErrMissingType                 = errors.New("type is missing")
	ErrMissingData                 = errors.New("data is missing")
	ErrInvalidType                 = errors.New("invalid type")
)

// WebSocketMessage is the type for all messages send in the WebSocket to the client.
// Depending on the MessageType the Data or ExitCode might not be included in the marshaled json message.
type WebSocketMessage struct {
	Type     WebSocketMessageType
	Data     string
	ExitCode uint8
}

// MarshalJSON implements the json.Marshaler interface.
// This converts the WebSocketMessage into the expected schema (see docs/websocket.schema.json).
func (m WebSocketMessage) MarshalJSON() (res []byte, err error) {
	switch m.Type {
	case WebSocketOutputStdout, WebSocketOutputStderr, WebSocketOutputError:
		res, err = json.Marshal(struct {
			MessageType WebSocketMessageType `json:"type"`
			Data        string               `json:"data"`
		}{m.Type, m.Data})
	case WebSocketMetaStart, WebSocketMetaTimeout:
		res, err = json.Marshal(struct {
			MessageType WebSocketMessageType `json:"type"`
		}{m.Type})
	case WebSocketExit:
		res, err = json.Marshal(struct {
			MessageType WebSocketMessageType `json:"type"`
			ExitCode    uint8                `json:"data"`
		}{m.Type, m.ExitCode})
	}
	if err != nil {
		return nil, fmt.Errorf("error marshaling WebSocketMessage: %w", err)
	} else if res == nil {
		return nil, ErrUnknownWebSocketMessageType
	}
	return res, nil
}

// UnmarshalJSON implements the json.Unmarshaler interface.
// It is used by tests in order to ReceiveNextWebSocketMessage.
func (m *WebSocketMessage) UnmarshalJSON(rawMessage []byte) error {
	messageMap := make(map[string]interface{})
	err := json.Unmarshal(rawMessage, &messageMap)
	if err != nil {
		return fmt.Errorf("error unmarshiling raw WebSocket message: %w", err)
	}
	messageType, ok := messageMap["type"]
	if !ok {
		return ErrMissingType
	}
	messageTypeString, ok := messageType.(string)
	if !ok {
		return fmt.Errorf("value of key type must be a string: %w", ErrInvalidType)
	}
	switch messageType := WebSocketMessageType(messageTypeString); messageType {
	case WebSocketExit:
		data, ok := messageMap["data"]
		if !ok {
			return ErrMissingData
		}
		// json.Unmarshal converts any number to a float64 in the massageMap, so we must first cast it to the float.
		exit, ok := data.(float64)
		if !ok {
			return fmt.Errorf("value of key data must be a number: %w", ErrInvalidType)
		}
		if exit != float64(uint8(exit)) {
			return fmt.Errorf("value of key data must be uint8: %w", ErrInvalidType)
		}
		m.Type = messageType
		m.ExitCode = uint8(exit)
	case WebSocketOutputStdout, WebSocketOutputStderr, WebSocketOutputError:
		data, ok := messageMap["data"]
		if !ok {
			return ErrMissingData
		}
		text, ok := data.(string)
		if !ok {
			return fmt.Errorf("value of key data must be a string: %w", ErrInvalidType)
		}
		m.Type = messageType
		m.Data = text
	case WebSocketMetaStart, WebSocketMetaTimeout:
		m.Type = messageType
	default:
		return ErrUnknownWebSocketMessageType
	}
	return nil
}

// ClientError is the response interface if the request is not valid.
type ClientError struct {
	Message string `json:"message"`
}

// InternalServerError is the response interface that is returned when an error occurs.
type InternalServerError struct {
	Message   string    `json:"message"`
	ErrorCode ErrorCode `json:"errorCode"`
}

// ErrorCode is the type for error codes expected by CodeOcean.
type ErrorCode string

const (
	ErrorNomadUnreachable         ErrorCode = "NOMAD_UNREACHABLE"
	ErrorNomadOverload            ErrorCode = "NOMAD_OVERLOAD"
	ErrorNomadInternalServerError ErrorCode = "NOMAD_INTERNAL_SERVER_ERROR"
	ErrorUnknown                  ErrorCode = "UNKNOWN"
)
