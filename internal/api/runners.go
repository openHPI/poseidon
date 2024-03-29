package api

import (
	"context"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/openHPI/poseidon/internal/config"
	"github.com/openHPI/poseidon/internal/runner"
	"github.com/openHPI/poseidon/pkg/dto"
	"github.com/openHPI/poseidon/pkg/logging"
	"github.com/openHPI/poseidon/pkg/monitoring"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

const (
	ExecutePath             = "/execute"
	WebsocketPath           = "/websocket"
	UpdateFileSystemPath    = "/files"
	ListFileSystemRouteName = UpdateFileSystemPath + "_list"
	FileContentRawPath      = UpdateFileSystemPath + "/raw"
	ProvideRoute            = "provideRunner"
	DeleteRoute             = "deleteRunner"
	RunnerIDKey             = "runnerId"
	ExecutionIDKey          = "executionID"
	PathKey                 = "path"
	RecursiveKey            = "recursive"
	PrivilegedExecutionKey  = "privilegedExecution"
)

var ErrForbiddenCharacter = errors.New("use of forbidden character")

type RunnerController struct {
	manager      runner.Accessor
	runnerRouter *mux.Router
}

// ConfigureRoutes configures a given router with the runner routes of our API.
func (r *RunnerController) ConfigureRoutes(router *mux.Router) {
	runnersRouter := router.PathPrefix(RunnersPath).Subrouter()
	runnersRouter.HandleFunc("", r.provide).Methods(http.MethodPost).Name(ProvideRoute)
	r.runnerRouter = runnersRouter.PathPrefix(fmt.Sprintf("/{%s}", RunnerIDKey)).Subrouter()
	r.runnerRouter.Use(r.findRunnerMiddleware)
	r.runnerRouter.HandleFunc(UpdateFileSystemPath, r.listFileSystem).Methods(http.MethodGet).
		Name(ListFileSystemRouteName)
	r.runnerRouter.HandleFunc(UpdateFileSystemPath, r.updateFileSystem).Methods(http.MethodPatch).
		Name(UpdateFileSystemPath)
	r.runnerRouter.HandleFunc(FileContentRawPath, r.fileContent).Methods(http.MethodGet).Name(FileContentRawPath)
	r.runnerRouter.HandleFunc(ExecutePath, r.execute).Methods(http.MethodPost).Name(ExecutePath)
	r.runnerRouter.HandleFunc(WebsocketPath, r.connectToRunner).Methods(http.MethodGet).Name(WebsocketPath)
	r.runnerRouter.HandleFunc("", r.delete).Methods(http.MethodDelete).Name(DeleteRoute)
}

// provide handles the provide runners API route.
// It tries to respond with the id of a unused runner.
// This runner is then reserved for future use.
func (r *RunnerController) provide(writer http.ResponseWriter, request *http.Request) {
	runnerRequest := new(dto.RunnerRequest)
	if err := parseJSONRequestBody(writer, request, runnerRequest); err != nil {
		return
	}
	environmentID := dto.EnvironmentID(runnerRequest.ExecutionEnvironmentID)

	var nextRunner runner.Runner
	var err error
	logging.StartSpan("api.runner.claim", "Claim Runner", request.Context(), func(_ context.Context) {
		nextRunner, err = r.manager.Claim(environmentID, runnerRequest.InactivityTimeout)
	})
	if err != nil {
		switch {
		case errors.Is(err, runner.ErrUnknownExecutionEnvironment):
			writeClientError(writer, err, http.StatusNotFound, request.Context())
		case errors.Is(err, runner.ErrNoRunnersAvailable):
			log.WithContext(request.Context()).Warn("No runners available")
			writeInternalServerError(writer, err, dto.ErrorNomadOverload, request.Context())
		default:
			writeInternalServerError(writer, err, dto.ErrorUnknown, request.Context())
		}
		return
	}
	monitoring.AddRunnerMonitoringData(request, nextRunner.ID(), nextRunner.Environment())
	sendJSON(writer, &dto.RunnerResponse{ID: nextRunner.ID(), MappedPorts: nextRunner.MappedPorts()},
		http.StatusOK, request.Context())
}

// listFileSystem handles the files API route with the method GET.
// It returns a listing of the file system of the provided runner.
func (r *RunnerController) listFileSystem(writer http.ResponseWriter, request *http.Request) {
	targetRunner, _ := runner.FromContext(request.Context())

	recursiveRaw := request.URL.Query().Get(RecursiveKey)
	recursive, err := strconv.ParseBool(recursiveRaw)
	recursive = err != nil || recursive

	path := request.URL.Query().Get(PathKey)
	if path == "" {
		path = "./"
	}
	privilegedExecution, err := strconv.ParseBool(request.URL.Query().Get(PrivilegedExecutionKey))
	if err != nil {
		privilegedExecution = false
	}

	writer.Header().Set("Content-Type", "application/json")
	logging.StartSpan("api.fs.list", "List File System", request.Context(), func(ctx context.Context) {
		err = targetRunner.ListFileSystem(path, recursive, writer, privilegedExecution, ctx)
	})
	if errors.Is(err, runner.ErrFileNotFound) {
		writeClientError(writer, err, http.StatusFailedDependency, request.Context())
		return
	} else if err != nil {
		log.WithContext(request.Context()).WithError(err).Error("Could not perform the requested listFileSystem.")
		writeInternalServerError(writer, err, dto.ErrorUnknown, request.Context())
		return
	}
}

// updateFileSystem handles the files API route.
// It takes an dto.UpdateFileSystemRequest and sends it to the runner for processing.
func (r *RunnerController) updateFileSystem(writer http.ResponseWriter, request *http.Request) {
	monitoring.AddRequestSize(request)
	fileCopyRequest := new(dto.UpdateFileSystemRequest)
	if err := parseJSONRequestBody(writer, request, fileCopyRequest); err != nil {
		return
	}

	targetRunner, _ := runner.FromContext(request.Context())

	var err error
	logging.StartSpan("api.fs.update", "Update File System", request.Context(), func(ctx context.Context) {
		err = targetRunner.UpdateFileSystem(fileCopyRequest, ctx)
	})
	if err != nil {
		log.WithContext(request.Context()).WithError(err).Error("Could not perform the requested updateFileSystem.")
		writeInternalServerError(writer, err, dto.ErrorUnknown, request.Context())
		return
	}

	writer.WriteHeader(http.StatusNoContent)
}

func (r *RunnerController) fileContent(writer http.ResponseWriter, request *http.Request) {
	targetRunner, _ := runner.FromContext(request.Context())
	path := request.URL.Query().Get(PathKey)
	privilegedExecution, err := strconv.ParseBool(request.URL.Query().Get(PrivilegedExecutionKey))
	if err != nil {
		privilegedExecution = false
	}

	writer.Header().Set("Content-Disposition", "attachment; filename=\""+path+"\"")
	logging.StartSpan("api.fs.read", "File Content", request.Context(), func(ctx context.Context) {
		err = targetRunner.GetFileContent(path, writer, privilegedExecution, ctx)
	})
	if errors.Is(err, runner.ErrFileNotFound) {
		writeClientError(writer, err, http.StatusFailedDependency, request.Context())
		return
	} else if err != nil {
		log.WithContext(request.Context()).WithError(err).Error("Could not retrieve the requested file.")
		writeInternalServerError(writer, err, dto.ErrorUnknown, request.Context())
		return
	}
}

// execute handles the execute API route.
// It takes an ExecutionRequest and stores it for a runner.
// It returns a url to connect to for a websocket connection to this execution in the corresponding runner.
func (r *RunnerController) execute(writer http.ResponseWriter, request *http.Request) {
	executionRequest := new(dto.ExecutionRequest)
	if err := parseJSONRequestBody(writer, request, executionRequest); err != nil {
		return
	}
	forbiddenCharacters := "'"
	if strings.ContainsAny(executionRequest.Command, forbiddenCharacters) {
		writeClientError(writer, ErrForbiddenCharacter, http.StatusBadRequest, request.Context())
		return
	}

	var scheme string
	if config.Config.Server.TLS.Active {
		scheme = "wss"
	} else {
		scheme = "ws"
	}
	targetRunner, _ := runner.FromContext(request.Context())

	path, err := r.runnerRouter.Get(WebsocketPath).URL(RunnerIDKey, targetRunner.ID())
	if err != nil {
		log.WithContext(request.Context()).WithError(err).Error("Could not create runner websocket URL.")
		writeInternalServerError(writer, err, dto.ErrorUnknown, request.Context())
		return
	}
	newUUID, err := uuid.NewRandom()
	if err != nil {
		log.WithContext(request.Context()).WithError(err).Error("Could not create execution id")
		writeInternalServerError(writer, err, dto.ErrorUnknown, request.Context())
		return
	}
	id := newUUID.String()

	logging.StartSpan("api.runner.exec", "Store Execution", request.Context(), func(ctx context.Context) {
		targetRunner.StoreExecution(id, executionRequest)
	})
	webSocketURL := url.URL{
		Scheme:   scheme,
		Host:     request.Host,
		Path:     path.String(),
		RawQuery: fmt.Sprintf("%s=%s", ExecutionIDKey, id),
	}

	sendJSON(writer, &dto.ExecutionResponse{WebSocketURL: webSocketURL.String()}, http.StatusOK, request.Context())
}

// The findRunnerMiddleware looks up the runnerId for routes containing it
// and adds the runner to the context of the request.
func (r *RunnerController) findRunnerMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		runnerID := mux.Vars(request)[RunnerIDKey]
		targetRunner, err := r.manager.Get(runnerID)
		if err != nil {
			// We discard the request body because an early write causes errors for some clients.
			// See https://github.com/openHPI/poseidon/issues/54
			_, readErr := io.ReadAll(request.Body)
			if readErr != nil {
				log.WithContext(request.Context()).WithError(readErr).Debug("Failed to discard the request body")
			}
			writeClientError(writer, err, http.StatusGone, request.Context())
			return
		}
		ctx := runner.NewContext(request.Context(), targetRunner)
		ctx = context.WithValue(ctx, dto.ContextKey(dto.KeyRunnerID), targetRunner.ID())
		ctx = context.WithValue(ctx, dto.ContextKey(dto.KeyEnvironmentID), targetRunner.Environment().ToString())
		requestWithRunner := request.WithContext(ctx)
		monitoring.AddRunnerMonitoringData(requestWithRunner, targetRunner.ID(), targetRunner.Environment())

		next.ServeHTTP(writer, requestWithRunner)
	})
}

// delete handles the delete runner API route.
// It destroys the given runner on the executor and removes it from the used runners list.
func (r *RunnerController) delete(writer http.ResponseWriter, request *http.Request) {
	targetRunner, _ := runner.FromContext(request.Context())

	var err error
	logging.StartSpan("api.runner.delete", "Return Runner", request.Context(), func(ctx context.Context) {
		err = r.manager.Return(targetRunner)
	})
	if err != nil {
		writeInternalServerError(writer, err, dto.ErrorNomadInternalServerError, request.Context())
		return
	}

	writer.WriteHeader(http.StatusNoContent)
}
