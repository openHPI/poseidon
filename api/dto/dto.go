package dto

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

// ExecutionEnvironmentRequest is the expected json structure of the request body for the create execution environment function.
// nolint:unused,structcheck
type ExecutionEnvironmentRequest struct {
	prewarmingPoolSize uint
	cpuLimit           uint
	memoryLimit        uint
	image              string
	networkAccess      bool
	exposedPorts       []uint16
}

// RunnerResponse is the expected response when providing a runner.
type RunnerResponse struct {
	Id string `json:"runnerId"`
}

// FileCreation is the expected json structure of the request body for the copy files route.
// TODO: specify content of the struct
type FileCreation struct{}

// WebsocketResponse is the expected response when creating an execution for a runner.
type WebsocketResponse struct {
	WebsocketUrl string `json:"websocketUrl"`
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
