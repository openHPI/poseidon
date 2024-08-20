package runner

import "github.com/openHPI/poseidon/pkg/dto"

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

	// DeleteEnvironment removes the specified execution environment in Poseidon's memory.
	// It does nothing if the specified environment can not be found.
	DeleteEnvironment(id dto.EnvironmentID)

	// EnvironmentStatistics returns statistical data for each execution environment.
	EnvironmentStatistics() map[dto.EnvironmentID]*dto.StatisticalExecutionEnvironmentData
}

// AccessorHandler is one handler in the chain of responsibility of runner accessors.
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
}
