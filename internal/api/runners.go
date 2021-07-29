package api

import (
	"errors"
	"fmt"
	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/internal/config"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/internal/runner"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/pkg/dto"
	"net/http"
	"net/url"
)

const (
	ExecutePath          = "/execute"
	WebsocketPath        = "/websocket"
	UpdateFileSystemPath = "/files"
	DeleteRoute          = "deleteRunner"
	RunnerIDKey          = "runnerId"
	ExecutionIDKey       = "executionID"
	ProvideRoute         = "provideRunner"
)

type RunnerController struct {
	manager      runner.Manager
	runnerRouter *mux.Router
}

// ConfigureRoutes configures a given router with the runner routes of our API.
func (r *RunnerController) ConfigureRoutes(router *mux.Router) {
	runnersRouter := router.PathPrefix(RunnersPath).Subrouter()
	runnersRouter.HandleFunc("", r.provide).Methods(http.MethodPost).Name(ProvideRoute)
	r.runnerRouter = runnersRouter.PathPrefix(fmt.Sprintf("/{%s}", RunnerIDKey)).Subrouter()
	r.runnerRouter.Use(r.findRunnerMiddleware)
	r.runnerRouter.HandleFunc(UpdateFileSystemPath, r.updateFileSystem).Methods(http.MethodPatch).
		Name(UpdateFileSystemPath)
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
	environmentID := runner.EnvironmentID(runnerRequest.ExecutionEnvironmentID)
	nextRunner, err := r.manager.Claim(environmentID, runnerRequest.InactivityTimeout)
	if err != nil {
		switch err {
		case runner.ErrUnknownExecutionEnvironment:
			writeNotFound(writer, err)
		case runner.ErrNoRunnersAvailable:
			log.WithField("environment", environmentID).Warn("No runners available")
			writeInternalServerError(writer, err, dto.ErrorNomadOverload)
		default:
			writeInternalServerError(writer, err, dto.ErrorUnknown)
		}
		return
	}
	sendJSON(writer, &dto.RunnerResponse{ID: nextRunner.ID(), MappedPorts: nextRunner.MappedPorts()}, http.StatusOK)
}

// updateFileSystem handles the files API route.
// It takes an dto.UpdateFileSystemRequest and sends it to the runner for processing.
func (r *RunnerController) updateFileSystem(writer http.ResponseWriter, request *http.Request) {
	fileCopyRequest := new(dto.UpdateFileSystemRequest)
	if err := parseJSONRequestBody(writer, request, fileCopyRequest); err != nil {
		return
	}

	targetRunner, _ := runner.FromContext(request.Context())
	if err := targetRunner.UpdateFileSystem(fileCopyRequest); err != nil {
		log.WithError(err).Error("Could not perform the requested updateFileSystem.")
		writeInternalServerError(writer, err, dto.ErrorUnknown)
		return
	}

	writer.WriteHeader(http.StatusNoContent)
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
			writeNotFound(writer, err)
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

	err := r.manager.Return(targetRunner)
	if err != nil {
		if errors.Is(err, runner.ErrUnknownExecutionEnvironment) {
			writeNotFound(writer, err)
		} else {
			writeInternalServerError(writer, err, dto.ErrorNomadInternalServerError)
		}
		return
	}

	writer.WriteHeader(http.StatusNoContent)
}
