package api

import (
	"encoding/json"
	"fmt"
	"github.com/gorilla/mux"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/api/dto"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/environment"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/runner"
	"net/http"
)

const (
	executionEnvironmentIDKey = "executionEnvironmentId"
	createOrUpdateRouteName   = "createOrUpdate"
)

type EnvironmentController struct {
	manager environment.Manager
}

func (e *EnvironmentController) ConfigureRoutes(router *mux.Router) {
	environmentRouter := router.PathPrefix(EnvironmentsPath).Subrouter()
	specificEnvironmentRouter := environmentRouter.Path(fmt.Sprintf("/{%s:[0-9]+}", executionEnvironmentIDKey)).Subrouter()
	specificEnvironmentRouter.HandleFunc("", e.createOrUpdate).Methods(http.MethodPut).Name(createOrUpdateRouteName)
}

// createOrUpdate creates/updates an execution environment on the executor.
func (e *EnvironmentController) createOrUpdate(writer http.ResponseWriter, request *http.Request) {
	req := new(dto.ExecutionEnvironmentRequest)
	if err := json.NewDecoder(request.Body).Decode(req); err != nil {
		writeBadRequest(writer, err)
		return
	}

	id, ok := mux.Vars(request)[executionEnvironmentIDKey]
	if !ok {
		writeBadRequest(writer, fmt.Errorf("could not find %s", executionEnvironmentIDKey))
		return
	}
	environmentID, err := runner.NewEnvironmentID(id)
	if err != nil {
		writeBadRequest(writer, fmt.Errorf("could not update environment: %w", err))
		return
	}
	created, err := e.manager.CreateOrUpdate(environmentID, *req)
	if err != nil {
		writeInternalServerError(writer, err, dto.ErrorUnknown)
	}

	if created {
		writer.WriteHeader(http.StatusCreated)
	} else {
		writer.WriteHeader(http.StatusNoContent)
	}
}

// delete removes an execution environment from the executor
func (e *EnvironmentController) delete(writer http.ResponseWriter, request *http.Request) { // nolint:unused ToDo

}
