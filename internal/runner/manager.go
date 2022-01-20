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

// Manager keeps track of the used and unused runners of all execution environments in order to provide unused
// runners to new clients and ensure no runner is used twice.
type Manager interface {
	EnvironmentAccessor
	AccessorHandler
}

// EnvironmentAccessor provides access to the stored environments.
type EnvironmentAccessor interface {
	// ListEnvironments returns all execution environments known by Poseidon.
	ListEnvironments() []ExecutionEnvironment

	// GetEnvironment returns the details of the requested environment.
	// Iff the requested environment is not stored it returns false.
	GetEnvironment(id dto.EnvironmentID) (ExecutionEnvironment, bool)

	// StoreEnvironment stores the environment in Poseidons memory.
	StoreEnvironment(environment ExecutionEnvironment)

	// DeleteEnvironment removes the specified execution environment in Poseidons memory.
	// It does nothing if the specified environment can not be found.
	DeleteEnvironment(id dto.EnvironmentID)

	// EnvironmentStatistics returns statistical data for each execution environment.
	EnvironmentStatistics() map[dto.EnvironmentID]*dto.StatisticalExecutionEnvironmentData
}

// AccessorHandler is one handler in te chain of responsibility of runner accessors.
// Each runner accessor can handle different requests.
type AccessorHandler interface {
	Accessor
	SetNextHandler(m AccessorHandler)
	NextHandler() AccessorHandler
	HasNextHandler() bool
}

// Accessor manages the lifecycle of Runner.
type Accessor interface {
	// Claim returns a new runner. The runner is deleted after duration seconds if duration is not 0.
	// It makes sure that the runner is not in use yet and returns an error if no runner could be provided.
	Claim(id dto.EnvironmentID, duration int) (Runner, error)

	// Get returns the used runner with the given runnerId.
	// If no runner with the given runnerId is currently used, it returns an error.
	Get(runnerID string) (Runner, error)

	// Return signals that the runner is no longer used by the caller and can be claimed by someone else.
	// The runner is deleted or cleaned up for reuse depending on the used executor.
	Return(r Runner) error

	// Load fetches all already created runners from the executor and registers them.
	// It should be called during the startup process (e.g. on creation of the Manager).
	Load()
}
