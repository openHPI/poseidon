package dto

// RequestRunner is the expected json structure of the request body for the ProvideRunner function
type RequestRunner struct {
	ExecutionEnvironmentId int `json:"executionEnvironmentId"`
	InactivityTimeout      int `json:"inactivityTimeout"`
}

// ResponseRunner is the expected result from the api server
type ResponseRunner struct {
	Id string `json:"runnerId"`
}

// ClientError is the response interface if the request is not valid
type ClientError struct {
	Message string `json:"message"`
}

// InternalServerError is the response interface that is returned when an error occurs
type InternalServerError struct {
	Message   string    `json:"message"`
	ErrorCode ErrorCode `json:"errorCode"`
}

// ErrorCode is the type for error codes expected by CodeOcean
type ErrorCode string

const (
	ErrorNomadUnreachable         ErrorCode = "NOMAD_UNREACHABLE"
	ErrorNomadOverload            ErrorCode = "NOMAD_OVERLOAD"
	ErrorNomadInternalServerError ErrorCode = "NOMAD_INTERNAL_SERVER_ERROR"
	ErrorUnknown                  ErrorCode = "UNKNOWN"
)
