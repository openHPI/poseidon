package dto

import (
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"strings"
)

// RunnerRequest is the expected json structure of the request body for the ProvideRunner function.
type RunnerRequest struct {
	ExecutionEnvironmentId int `json:"executionEnvironmentId"`
	InactivityTimeout      int `json:"inactivityTimeout"`
}

// ExecutionRequest is the expected json structure of the request body for the ExecuteCommand function.
type ExecutionRequest struct {
	Command     string
	TimeLimit   int
	Environment map[string]string
}

func (er *ExecutionRequest) FullCommand() []string {
	var command []string
	command = append(command, "env", "-")
	for variable, value := range er.Environment {
		command = append(command, fmt.Sprintf("%s=%s", variable, value))
	}
	command = append(command, "sh", "-c", er.Command)
	return command
}

// ExecutionEnvironmentRequest is the expected json structure of the request body for the create execution environment function.
type ExecutionEnvironmentRequest struct {
	PrewarmingPoolSize uint     `json:"prewarmingPoolSize"`
	CPULimit           uint     `json:"cpuLimit"`
	MemoryLimit        uint     `json:"memoryLimit"`
	Image              string   `json:"image"`
	NetworkAccess      bool     `json:"networkAccess"`
	ExposedPorts       []uint16 `json:"exposedPorts"`
}

// RunnerResponse is the expected response when providing a runner.
type RunnerResponse struct {
	Id string `json:"runnerId"`
}

// ExecutionResponse is the expected response when creating an execution for a runner.
type ExecutionResponse struct {
	WebSocketUrl string `json:"websocketUrl"`
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

// ToAbsolute returns the absolute path of the FilePath with respect to the given basePath. If the FilePath already is absolute, basePath will be ignored.
func (f FilePath) ToAbsolute(basePath string) string {
	filePathString := string(f)
	if path.IsAbs(filePathString) {
		return path.Clean(filePathString)
	}
	return path.Clean(path.Join(basePath, filePathString))
}

// AbsolutePath returns the absolute path of the file. See FilePath.ToAbsolute for details.
func (f File) AbsolutePath(basePath string) string {
	return f.Path.ToAbsolute(basePath)
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

// WebSocketMessage is the type for all messages send in the WebSocket to the client.
// Depending on the MessageType the Data or ExitCode might not be included in the marshaled json message.
type WebSocketMessage struct {
	Type     WebSocketMessageType
	Data     string
	ExitCode uint8
}

// MarshalJSON implements the json.Marshaler interface.
// This converts the WebSocketMessage into the expected schema (see docs/websocket.schema.json).
func (m WebSocketMessage) MarshalJSON() ([]byte, error) {
	switch m.Type {
	case WebSocketOutputStdout, WebSocketOutputStderr, WebSocketOutputError:
		return json.Marshal(struct {
			MessageType WebSocketMessageType `json:"type"`
			Data        string               `json:"data"`
		}{m.Type, m.Data})
	case WebSocketMetaStart, WebSocketMetaTimeout:
		return json.Marshal(struct {
			MessageType WebSocketMessageType `json:"type"`
		}{m.Type})
	case WebSocketExit:
		return json.Marshal(struct {
			MessageType WebSocketMessageType `json:"type"`
			ExitCode    uint8                `json:"data"`
		}{m.Type, m.ExitCode})
	}
	return nil, errors.New("unhandled WebSocket message type")
}

// UnmarshalJSON implements the json.Unmarshaler interface.
// It is used by tests in order to ReceiveNextWebSocketMessage.
func (m *WebSocketMessage) UnmarshalJSON(rawMessage []byte) error {
	messageMap := make(map[string]interface{})
	err := json.Unmarshal(rawMessage, &messageMap)
	if err != nil {
		return err
	}
	messageType, ok := messageMap["type"]
	if !ok {
		return errors.New("missing key type")
	}
	messageTypeString, ok := messageType.(string)
	if !ok {
		return errors.New("value of key type must be a string")
	}
	switch messageType := WebSocketMessageType(messageTypeString); messageType {
	case WebSocketExit:
		data, ok := messageMap["data"]
		if !ok {
			return errors.New("missing key data")
		}
		// json.Unmarshal converts any number to a float64 in the massageMap, so we must first cast it to the float.
		exit, ok := data.(float64)
		if !ok {
			return errors.New("value of key data must be a number")
		}
		if exit != float64(uint8(exit)) {
			return errors.New("value of key data must be uint8")
		}
		m.Type = messageType
		m.ExitCode = uint8(exit)
	case WebSocketOutputStdout, WebSocketOutputStderr, WebSocketOutputError:
		data, ok := messageMap["data"]
		if !ok {
			return errors.New("missing key data")
		}
		text, ok := data.(string)
		if !ok {
			return errors.New("value of key data must be a string")
		}
		m.Type = messageType
		m.Data = text
	case WebSocketMetaStart, WebSocketMetaTimeout:
		m.Type = messageType
	default:
		return errors.New("unknown WebSocket message type")
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
