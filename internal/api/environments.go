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
	"strconv"
)

const (
	executionEnvironmentIDKey = "executionEnvironmentId"
	fetchEnvironmentKey       = "fetch"
	listRouteName             = "list"
	getRouteName              = "get"
	createOrUpdateRouteName   = "createOrUpdate"
	deleteRouteName           = "delete"
)

var ErrMissingURLParameter = errors.New("url parameter missing")

type EnvironmentController struct {
	manager environment.Manager
}

func (e *EnvironmentController) ConfigureRoutes(router *mux.Router) {
	environmentRouter := router.PathPrefix(EnvironmentsPath).Subrouter()
	environmentRouter.HandleFunc("", e.list).Methods(http.MethodGet).Name(listRouteName)

	specificEnvironmentRouter := environmentRouter.Path(fmt.Sprintf("/{%s:[0-9]+}", executionEnvironmentIDKey)).Subrouter()
	specificEnvironmentRouter.HandleFunc("", e.get).Methods(http.MethodGet).Name(getRouteName)
	specificEnvironmentRouter.HandleFunc("", e.createOrUpdate).Methods(http.MethodPut).Name(createOrUpdateRouteName)
	specificEnvironmentRouter.HandleFunc("", e.delete).Methods(http.MethodDelete).Name(deleteRouteName)
}

// list returns all information about available execution environments.
func (e *EnvironmentController) list(writer http.ResponseWriter, request *http.Request) {
	fetchString := request.FormValue(fetchEnvironmentKey)
	var fetch bool
	if len(fetchString) > 0 {
		var err error
		fetch, err = strconv.ParseBool(fetchString)
		if err != nil {
			writeBadRequest(writer, err)
			return
		}
	}

	environments, err := e.manager.List(fetch)
	if err != nil {
		writeInternalServerError(writer, err, dto.ErrorUnknown)
		return
	}

	sendJSON(writer, struct {
		ExecutionEnvironments []runner.ExecutionEnvironment `json:"executionEnvironments"`
	}{environments}, http.StatusOK)
}

// get returns all information about the requested execution environment.
func (e *EnvironmentController) get(writer http.ResponseWriter, request *http.Request) {
	id, ok := mux.Vars(request)[executionEnvironmentIDKey]
	if !ok {
		writeBadRequest(writer, fmt.Errorf("could not find %s: %w", executionEnvironmentIDKey, ErrMissingURLParameter))
		return
	}
	environmentID, err := dto.NewEnvironmentID(id)
	if err != nil {
		writeBadRequest(writer, fmt.Errorf("could parse environment id: %w", err))
		return
	}
	fetchString := request.FormValue(fetchEnvironmentKey)
	var fetch bool
	if len(fetchString) > 0 {
		var err error
		fetch, err = strconv.ParseBool(fetchString)
		if err != nil {
			writeBadRequest(writer, err)
			return
		}
	}

	executionEnvironment, err := e.manager.Get(environmentID, fetch)
	if err != nil {
		writeInternalServerError(writer, err, dto.ErrorUnknown)
		return
	}

	sendJSON(writer, executionEnvironment, http.StatusOK)
}

// delete removes the specified execution environment.
func (e *EnvironmentController) delete(writer http.ResponseWriter, request *http.Request) {
	id, ok := mux.Vars(request)[executionEnvironmentIDKey]
	if !ok {
		writeBadRequest(writer, fmt.Errorf("could not find %s: %w", executionEnvironmentIDKey, ErrMissingURLParameter))
		return
	}
	environmentID, err := dto.NewEnvironmentID(id)
	if err != nil {
		writeBadRequest(writer, fmt.Errorf("could parse environment id: %w", err))
		return
	}

	found, err := e.manager.Delete(environmentID)
	if err != nil {
		writeInternalServerError(writer, err, dto.ErrorUnknown)
		return
	} else if !found {
		writer.WriteHeader(http.StatusNotFound)
	}

	writer.WriteHeader(http.StatusNoContent)
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
	environmentID, err := dto.NewEnvironmentID(id)
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
