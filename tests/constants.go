package tests

import (
	"errors"
	nomadApi "github.com/hashicorp/nomad/api"
	"github.com/openHPI/poseidon/pkg/dto"
	"time"
)

const (
	NonExistingIntegerID          = 9999
	NonExistingStringID           = "n0n-3x1st1ng-1d"
	DefaultFileName               = "test.txt"
	DefaultFileContent            = "Hello, Codemoon!"
	DefaultDirectoryName          = "test/"
	FileNameWithAbsolutePath      = "/test.txt"
	DefaultEnvironmentIDAsInteger = 0
	DefaultEnvironmentIDAsString  = "0"
	AnotherEnvironmentIDAsInteger = 42
	AnotherEnvironmentIDAsString  = "42"
	DefaultUUID                   = "MY-DEFAULT-RANDOM-UUID"
	AnotherUUID                   = "another-uuid-43"
	DefaultTemplateJobID          = "template-" + DefaultEnvironmentIDAsString
	DefaultRunnerID               = DefaultEnvironmentIDAsString + "-" + DefaultUUID
	AnotherRunnerID               = AnotherEnvironmentIDAsString + "-" + AnotherUUID
	DefaultExecutionID            = "s0m3-3x3cu710n-1d"
	DefaultMockID                 = "m0ck-1d"
	ShortTimeout                  = 100 * time.Millisecond
	DefaultTestTimeout            = 10 * time.Minute
)

var (
	ErrDefault              = errors.New("an error occurred")
	ErrCleanupDestroyReason = errors.New("destruction required for cleanup")

	DefaultPortMappings = []nomadApi.PortMapping{{To: 42, Value: 1337, Label: "lit", HostIP: "127.0.0.1"}}
	DefaultMappedPorts  = []*dto.MappedPort{{ExposedPort: 42, HostAddress: "127.0.0.1:1337"}}
)
