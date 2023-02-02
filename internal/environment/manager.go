package environment

import (
	"context"
	"github.com/openHPI/poseidon/internal/runner"
	"github.com/openHPI/poseidon/pkg/dto"
)

// ManagerHandler is one handler in the chain of responsibility of environment managers.
// Each manager can handle different requests.
type ManagerHandler interface {
	Manager
	SetNextHandler(next ManagerHandler)
	NextHandler() ManagerHandler
	HasNextHandler() bool
}

// Manager encapsulates API calls to the executor API for creation and deletion of execution environments.
type Manager interface {
	// List returns all environments known by Poseidon.
	// When `fetch` is set the environments are fetched from the executor before returning.
	List(fetch bool) ([]runner.ExecutionEnvironment, error)

	// Get returns the details of the requested environment.
	// When `fetch` is set the requested environment is fetched from the executor before returning.
	Get(id dto.EnvironmentID, fetch bool) (runner.ExecutionEnvironment, error)

	// CreateOrUpdate creates/updates an execution environment on the executor.
	// If the job was created, the returned boolean is true, if it was updated, it is false.
	// If err is not nil, that means the environment was neither created nor updated.
	CreateOrUpdate(
		id dto.EnvironmentID,
		request dto.ExecutionEnvironmentRequest,
		ctx context.Context,
	) (bool, error)

	// Delete removes the specified execution environment.
	// Iff the specified environment could not be found Delete returns false.
	Delete(id dto.EnvironmentID) (bool, error)

	// Statistics returns statistical data for each execution environment.
	Statistics() map[dto.EnvironmentID]*dto.StatisticalExecutionEnvironmentData
}
