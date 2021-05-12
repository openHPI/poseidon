package api

import (
	"gitlab.hpi.de/codeocean/codemoon/poseidon/environment"
	"net/http"
)

type EnvironmentController struct {
	manager environment.Manager // nolint:unused,structcheck
}

// create creates a new execution environment on the executor.
func (e *EnvironmentController) create(writer http.ResponseWriter, request *http.Request) { // nolint:unused

}

// delete removes an execution environment from the executor
func (e *EnvironmentController) delete(writer http.ResponseWriter, request *http.Request) { // nolint:unused

}
