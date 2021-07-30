package execution

import (
	"github.com/openHPI/poseidon/pkg/dto"
)

// ID is an identifier for an execution.
type ID string

// Storer stores executions.
type Storer interface {
	// Add adds a runner to the storage.
	// It overwrites the existing execution if an execution with the same id already exists.
	Add(id ID, executionRequest *dto.ExecutionRequest)

	// Exists returns whether the execution with the given id exists in the store.
	Exists(id ID) bool

	// Pop deletes the execution with the given id from the storage and returns it.
	// If no such execution exists, ok is false and true otherwise.
	Pop(id ID) (request *dto.ExecutionRequest, ok bool)
}
