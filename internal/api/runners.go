package api

import (
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

	nextRunner, err := r.manager.Claim(environmentID, runnerRequest.InactivityTimeout)
	if err != nil {
		switch {
		case errors.Is(err, runner.ErrUnknownExecutionEnvironment):
			writeClientError(writer, err, http.StatusNotFound)
		case errors.Is(err, runner.ErrNoRunnersAvailable):
			log.WithField("environment", logging.RemoveNewlineSymbol(strconv.Itoa(int(environmentID)))).
				Warn("No runners available")
			writeInternalServerError(writer, err, dto.ErrorNomadOverload)
		default:
			writeInternalServerError(writer, err, dto.ErrorUnknown)
		}
		return
	}
	monitoring.AddRunnerMonitoringData(request, nextRunner.ID(), nextRunner.Environment())
	sendJSON(writer, &dto.RunnerResponse{ID: nextRunner.ID(), MappedPorts: nextRunner.MappedPorts()}, http.StatusOK)
}

// listFileSystem handles the files API route with the method GET.
// It returns a listing of the file system of the provided runner.
func (r *RunnerController) listFileSystem(writer http.ResponseWriter, request *http.Request) {
	targetRunner, _ := runner.FromContext(request.Context())
	monitoring.AddRunnerMonitoringData(request, targetRunner.ID(), targetRunner.Environment())

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
	err = targetRunner.ListFileSystem(path, recursive, writer, privilegedExecution, request.Context())
	if errors.Is(err, runner.ErrFileNotFound) {
		writeClientError(writer, err, http.StatusFailedDependency)
		return
	} else if err != nil {
		log.WithError(err).Error("Could not perform the requested listFileSystem.")
		writeInternalServerError(writer, err, dto.ErrorUnknown)
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
	monitoring.AddRunnerMonitoringData(request, targetRunner.ID(), targetRunner.Environment())
	if err := targetRunner.UpdateFileSystem(fileCopyRequest); err != nil {
		log.WithError(err).Error("Could not perform the requested updateFileSystem.")
		writeInternalServerError(writer, err, dto.ErrorUnknown)
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
	err = targetRunner.GetFileContent(path, writer, privilegedExecution, request.Context())
	if errors.Is(err, runner.ErrFileNotFound) {
		writeClientError(writer, err, http.StatusFailedDependency)
		return
	} else if err != nil {
		log.WithError(err).Error("Could not retrieve the requested file.")
		writeInternalServerError(writer, err, dto.ErrorUnknown)
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

	var scheme string
	if config.Config.Server.TLS.Active {
		scheme = "wss"
	} else {
		scheme = "ws"
	}
	targetRunner, _ := runner.FromContext(request.Context())
	monitoring.AddRunnerMonitoringData(request, targetRunner.ID(), targetRunner.Environment())

	path, err := r.runnerRouter.Get(WebsocketPath).URL(RunnerIDKey, targetRunner.ID())
	if err != nil {
		log.WithError(err).Error("Could not create runner websocket URL.")
		writeInternalServerError(writer, err, dto.ErrorUnknown)
		return
	}
	newUUID, err := uuid.NewRandom()
	if err != nil {
		log.WithError(err).Error("Could not create execution id")
		writeInternalServerError(writer, err, dto.ErrorUnknown)
		return
	}
	id := newUUID.String()
	targetRunner.StoreExecution(id, executionRequest)
	webSocketURL := url.URL{
		Scheme:   scheme,
		Host:     request.Host,
		Path:     path.String(),
		RawQuery: fmt.Sprintf("%s=%s", ExecutionIDKey, id),
	}

	sendJSON(writer, &dto.ExecutionResponse{WebSocketURL: webSocketURL.String()}, http.StatusOK)
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
				log.WithError(readErr).Warn("Failed to discard the request body")
			}
			writeClientError(writer, err, http.StatusGone)
			return
		}
		ctx := runner.NewContext(request.Context(), targetRunner)
		requestWithRunner := request.WithContext(ctx)
		next.ServeHTTP(writer, requestWithRunner)
	})
}

// delete handles the delete runner API route.
// It destroys the given runner on the executor and removes it from the used runners list.
func (r *RunnerController) delete(writer http.ResponseWriter, request *http.Request) {
	targetRunner, _ := runner.FromContext(request.Context())
	monitoring.AddRunnerMonitoringData(request, targetRunner.ID(), targetRunner.Environment())

	err := r.manager.Return(targetRunner)
	if err != nil {
		writeInternalServerError(writer, err, dto.ErrorNomadInternalServerError)
		return
	}

	writer.WriteHeader(http.StatusNoContent)
}
