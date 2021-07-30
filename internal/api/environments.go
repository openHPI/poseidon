package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/gorilla/mux"
	"github.com/openHPI/poseidon/internal/environment"
	"github.com/openHPI/poseidon/internal/runner"
	"github.com/openHPI/poseidon/pkg/dto"
	"net/http"
)

const (
	executionEnvironmentIDKey = "executionEnvironmentId"
	createOrUpdateRouteName   = "createOrUpdate"
)

var ErrMissingURLParameter = errors.New("url parameter missing")

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
		writeBadRequest(writer, fmt.Errorf("could not find %s: %w", executionEnvironmentIDKey, ErrMissingURLParameter))
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
