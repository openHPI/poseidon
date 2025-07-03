package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/getsentry/sentry-go"
	"github.com/gorilla/mux"
	"github.com/openHPI/poseidon/internal/environment"
	"github.com/openHPI/poseidon/internal/runner"
	"github.com/openHPI/poseidon/pkg/dto"
	"github.com/openHPI/poseidon/pkg/logging"
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
		writeClientError(request.Context(), writer, err, http.StatusBadRequest)
		return
	}

	environments, err := e.manager.List(request.Context(), fetch)
	if err != nil {
		writeInternalServerError(request.Context(), writer, err, dto.ErrorUnknown)
		return
	}

	sendJSON(request.Context(), writer, ExecutionEnvironmentsResponse{environments}, http.StatusOK)
}

// get returns all information about the requested execution environment.
func (e *EnvironmentController) get(writer http.ResponseWriter, request *http.Request) {
	environmentID, err := parseEnvironmentID(request)
	if err != nil {
		// This case is never used as the router validates the id format
		writeClientError(request.Context(), writer, err, http.StatusBadRequest)
		return
	}

	fetch, err := parseFetchParameter(request)
	if err != nil {
		writeClientError(request.Context(), writer, err, http.StatusBadRequest)
		return
	}

	executionEnvironment, err := e.manager.Get(request.Context(), environmentID, fetch)
	if errors.Is(err, runner.ErrUnknownExecutionEnvironment) {
		writer.WriteHeader(http.StatusNotFound)
		return
	} else if err != nil {
		writeInternalServerError(request.Context(), writer, err, dto.ErrorUnknown)
		return
	}

	sendJSON(request.Context(), writer, executionEnvironment, http.StatusOK)
}

// delete removes the specified execution environment.
func (e *EnvironmentController) delete(writer http.ResponseWriter, request *http.Request) {
	environmentID, err := parseEnvironmentID(request)
	if err != nil {
		// This case is never used as the router validates the id format
		writeClientError(request.Context(), writer, err, http.StatusBadRequest)
		return
	}

	found, err := e.manager.Delete(environmentID)
	if err != nil {
		writeInternalServerError(request.Context(), writer, err, dto.ErrorUnknown)
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

	err := json.NewDecoder(request.Body).Decode(req)
	if err != nil {
		writeClientError(request.Context(), writer, err, http.StatusBadRequest)
		return
	}

	environmentID, err := parseEnvironmentID(request)
	if err != nil {
		writeClientError(request.Context(), writer, err, http.StatusBadRequest)
		return
	}

	var created bool

	logging.StartSpan(request.Context(), "api.env.update", "Create Environment", func(ctx context.Context, _ *sentry.Span) {
		created, err = e.manager.CreateOrUpdate(ctx, environmentID, *req)
	})

	if err != nil {
		writeInternalServerError(request.Context(), writer, err, dto.ErrorUnknown)
		return
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
	if fetchString != "" {
		fetch, err = strconv.ParseBool(fetchString)
		if err != nil {
			return false, fmt.Errorf("could not parse fetch parameter: %w", err)
		}
	}

	return fetch, nil
}
