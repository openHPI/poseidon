package runner

import (
	"encoding/json"
	"strconv"

	"github.com/influxdata/influxdb-client-go/v2/api/write"
	"github.com/openHPI/poseidon/pkg/dto"
	"github.com/openHPI/poseidon/pkg/storage"
)

// ReadableExecutionEnvironment defines structs that can return the attributes of an Execution Environment.
type ReadableExecutionEnvironment interface {
	// ID returns the id of the environment.
	ID() dto.EnvironmentID

	// PrewarmingPoolSize sets the number of idle runner of this environment that should be prewarmed.
	PrewarmingPoolSize() uint

	// CPULimit sets the share of cpu that a runner should receive at minimum.
	CPULimit() uint

	// MemoryLimit sets the amount of memory that should be available for each runner.
	MemoryLimit() uint

	// Image sets the image of the runner, e.g. Docker image.
	Image() string

	// NetworkAccess sets if a runner should have network access and if ports should be mapped.
	NetworkAccess() (bool, []uint16)

	// IdleRunnerCount returns the number of idle runners of the environment.
	IdleRunnerCount() uint
}

// WriteableExecutionEnvironment defines structs that can update the attributes of an Execution Environment.
type WriteableExecutionEnvironment interface {
	SetID(id dto.EnvironmentID)
	SetPrewarmingPoolSize(count uint)
	SetCPULimit(limit uint) error
	SetMemoryLimit(limit uint) error
	SetImage(image string)
	SetNetworkAccess(allow bool, ports []uint16)

	// SetConfigFrom copies all above attributes from the passed environment to the object itself.
	SetConfigFrom(environment ExecutionEnvironment)

	// AddRunner adds an existing runner to the idle runners of the environment.
	AddRunner(r Runner)
}

// ExternalExecutionEnvironment defines the functionalities with impact on external executors.
type ExternalExecutionEnvironment interface {
	// Register saves this environment at the executor.
	Register() error
	// ApplyPrewarmingPoolSize creates idle runners according to the PrewarmingPoolSize.
	ApplyPrewarmingPoolSize() error
	// Delete removes this environment and all it's runner from the executor and Poseidon itself.
	// Iff local the environment is just removed from Poseidon without external escalation.
	Delete(reason DestroyReason) error

	// Sample returns and removes an arbitrary available runner.
	// ok is true iff a runner was returned.
	Sample() (r Runner, ok bool)
	// DeleteRunner removes an idle runner from the environment and returns it.
	// This function handles only the environment. The runner has to be destroyed separately.
	// ok is true iff the runner was found (and deleted).
	DeleteRunner(id string) (r Runner, ok bool)
}

// ExecutionEnvironment are groups of runner that share the configuration stored in the environment.
type ExecutionEnvironment interface {
	json.Marshaler

	ReadableExecutionEnvironment
	WriteableExecutionEnvironment
	ExternalExecutionEnvironment
}

// monitorEnvironmentData passes the configuration of the environment e into the monitoring Point p.
func monitorEnvironmentData(dataPoint *write.Point, e ExecutionEnvironment, eventType storage.EventType) {
	if eventType == storage.Creation && e != nil {
		dataPoint.AddTag("image", e.Image())
		dataPoint.AddTag("cpu_limit", strconv.FormatUint(uint64(e.CPULimit()), 10))
		dataPoint.AddTag("memory_limit", strconv.FormatUint(uint64(e.MemoryLimit()), 10))
		hasNetworkAccess, _ := e.NetworkAccess()
		dataPoint.AddTag("network_access", strconv.FormatBool(hasNetworkAccess))
	}
}
