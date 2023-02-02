package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/getsentry/sentry-go"
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
	manager environment.ManagerHandler
}

type ExecutionEnvironmentsResponse struct {
	ExecutionEnvironments []runner.ExecutionEnvironment `json:"executionEnvironments"`
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
	fetch, err := parseFetchParameter(request)
	if err != nil {
		writeClientError(writer, err, http.StatusBadRequest)
		return
	}

	environments, err := e.manager.List(fetch)
	if err != nil {
		writeInternalServerError(writer, err, dto.ErrorUnknown)
		return
	}

	sendJSON(writer, ExecutionEnvironmentsResponse{environments}, http.StatusOK)
}

// get returns all information about the requested execution environment.
func (e *EnvironmentController) get(writer http.ResponseWriter, request *http.Request) {
	environmentID, err := parseEnvironmentID(request)
	if err != nil {
		// This case is never used as the router validates the id format
		writeClientError(writer, err, http.StatusBadRequest)
		return
	}
	fetch, err := parseFetchParameter(request)
	if err != nil {
		writeClientError(writer, err, http.StatusBadRequest)
		return
	}

	executionEnvironment, err := e.manager.Get(environmentID, fetch)
	if errors.Is(err, runner.ErrUnknownExecutionEnvironment) {
		writer.WriteHeader(http.StatusNotFound)
		return
	} else if err != nil {
		writeInternalServerError(writer, err, dto.ErrorUnknown)
		return
	}

	sendJSON(writer, executionEnvironment, http.StatusOK)
}

// delete removes the specified execution environment.
func (e *EnvironmentController) delete(writer http.ResponseWriter, request *http.Request) {
	environmentID, err := parseEnvironmentID(request)
	if err != nil {
		// This case is never used as the router validates the id format
		writeClientError(writer, err, http.StatusBadRequest)
		return
	}

	found, err := e.manager.Delete(environmentID)
	if err != nil {
		writeInternalServerError(writer, err, dto.ErrorUnknown)
		return
	} else if !found {
		writer.WriteHeader(http.StatusNotFound)
		return
	}

	writer.WriteHeader(http.StatusNoContent)
}

// createOrUpdate creates/updates an execution environment on the executor.
func (e *EnvironmentController) createOrUpdate(writer http.ResponseWriter, request *http.Request) {
	req := new(dto.ExecutionEnvironmentRequest)
	if err := json.NewDecoder(request.Body).Decode(req); err != nil {
		writeClientError(writer, err, http.StatusBadRequest)
		return
	}
	environmentID, err := parseEnvironmentID(request)
	if err != nil {
		writeClientError(writer, err, http.StatusBadRequest)
		return
	}

	span := sentry.StartSpan(request.Context(), "Create Environment")
	created, err := e.manager.CreateOrUpdate(environmentID, *req, request.Context())
	span.Finish()
	if err != nil {
		writeInternalServerError(writer, err, dto.ErrorUnknown)
	}

	if created {
		writer.WriteHeader(http.StatusCreated)
	} else {
		writer.WriteHeader(http.StatusNoContent)
	}
}

func parseEnvironmentID(request *http.Request) (dto.EnvironmentID, error) {
	id, ok := mux.Vars(request)[executionEnvironmentIDKey]
	if !ok {
		return 0, fmt.Errorf("could not find %s: %w", executionEnvironmentIDKey, ErrMissingURLParameter)
	}
	environmentID, err := dto.NewEnvironmentID(id)
	if err != nil {
		return 0, fmt.Errorf("could not update environment: %w", err)
	}
	return environmentID, nil
}

func parseFetchParameter(request *http.Request) (fetch bool, err error) {
	fetchString := request.FormValue(fetchEnvironmentKey)
	if len(fetchString) > 0 {
		fetch, err = strconv.ParseBool(fetchString)
		if err != nil {
			return false, fmt.Errorf("could not parse fetch parameter: %w", err)
		}
	}
	return fetch, nil
}
