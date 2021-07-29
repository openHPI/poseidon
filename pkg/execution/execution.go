package execution

import (
	"gitlab.hpi.de/codeocean/codemoon/poseidon/pkg/dto"
)

// ID is an identifier for an execution.
type ID string

// Storer stores executions.
type Storer interface {
	// Add adds a runner to the storage.
	// It overwrites the existing execution if an execution with the same id already exists.
	Add(id ID, executionRequest *dto.ExecutionRequest)

	// Pop deletes the execution with the given id from the storage and returns it.
	// If no such execution exists, ok is false and true otherwise.
	Pop(id ID) (request *dto.ExecutionRequest, ok bool)
}
