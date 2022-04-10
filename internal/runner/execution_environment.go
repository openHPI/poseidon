package runner

import (
	"encoding/json"
	"github.com/openHPI/poseidon/pkg/dto"
)

// ExecutionEnvironment are groups of runner that share the configuration stored in the environment.
type ExecutionEnvironment interface {
	json.Marshaler

	// ID returns the id of the environment.
	ID() dto.EnvironmentID
	SetID(id dto.EnvironmentID)
	// PrewarmingPoolSize sets the number of idle runner of this environment that should be prewarmed.
	PrewarmingPoolSize() uint
	SetPrewarmingPoolSize(count uint)
	// ApplyPrewarmingPoolSize creates idle runners according to the PrewarmingPoolSize.
	ApplyPrewarmingPoolSize() error
	// CPULimit sets the share of cpu that a runner should receive at minimum.
	CPULimit() uint
	SetCPULimit(limit uint)
	// MemoryLimit sets the amount of memory that should be available for each runner.
	MemoryLimit() uint
	SetMemoryLimit(limit uint)
	// Image sets the image of the runner, e.g. Docker image.
	Image() string
	SetImage(image string)
	// NetworkAccess sets if a runner should have network access and if ports should be mapped.
	NetworkAccess() (bool, []uint16)
	SetNetworkAccess(allow bool, ports []uint16)
	// SetConfigFrom copies all above attributes from the passed environment to the object itself.
	SetConfigFrom(environment ExecutionEnvironment)

	// Register saves this environment at the executor.
	Register() error
	// Delete removes this environment and all it's runner from the executor and Poseidon itself.
	Delete() error

	// Sample returns and removes an arbitrary available runner.
	// ok is true iff a runner was returned.
	Sample() (r Runner, ok bool)
	// AddRunner adds an existing runner to the idle runners of the environment.
	AddRunner(r Runner)
	// DeleteRunner removes an idle runner from the environment.
	DeleteRunner(id string)
	// IdleRunnerCount returns the number of idle runners of the environment.
	IdleRunnerCount() int
}
