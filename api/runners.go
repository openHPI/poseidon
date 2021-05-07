package api

import (
	"fmt"
	"github.com/gorilla/mux"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/api/dto"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/config"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/environment"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/nomad"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/runner"
	"net/http"
	"net/url"
)

const (
	ExecutePath    = "/execute"
	WebsocketPath  = "/websocket"
	DeleteRoute    = "deleteRunner"
	RunnerIdKey    = "runnerId"
	ExecutionIdKey = "executionId"
)

// provideRunner tries to respond with the id of a runner
// This runner is then reserved for future use
func provideRunner(writer http.ResponseWriter, request *http.Request) {
	runnerRequest := new(dto.RunnerRequest)
	if err := parseJSONRequestBody(writer, request, runnerRequest); err != nil {
		return
	}
	env, err := environment.GetExecutionEnvironment(runnerRequest.ExecutionEnvironmentId)
	if err != nil {
		writeNotFound(writer, err)
		return
	}
	nextRunner, err := env.NextRunner()
	if err != nil {
		writeInternalServerError(writer, err, dto.ErrorNomadOverload)
		return
	}
	sendJson(writer, &dto.RunnerResponse{Id: nextRunner.Id()}, http.StatusOK)
}

// executeCommand takes an ExecutionRequest and stores it for a runner.
// It returns a url to connect to for a websocket connection to this execution in the corresponding runner.
func executeCommand(router *mux.Router) func(w http.ResponseWriter, r *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		executionRequest := new(dto.ExecutionRequest)
		if err := parseJSONRequestBody(writer, request, executionRequest); err != nil {
			return
		}

		var scheme string
		if config.Config.Server.TLS {
			scheme = "wss"
		} else {
			scheme = "ws"
		}
		r, _ := runner.FromContext(request.Context())

		path, err := router.Get(WebsocketPath).URL(RunnerIdKey, r.Id())
		if err != nil {
			log.WithError(err).Error("Could not create runner websocket URL.")
			writeInternalServerError(writer, err, dto.ErrorUnknown)
			return
		}
		id, err := r.AddExecution(*executionRequest)
		if err != nil {
			log.WithError(err).Error("Could not store execution.")
			writeInternalServerError(writer, err, dto.ErrorUnknown)
			return
		}
		websocketUrl := url.URL{
			Scheme:   scheme,
			Host:     request.Host,
			Path:     path.String(),
			RawQuery: fmt.Sprintf("%s=%s", ExecutionIdKey, id),
		}

		sendJson(writer, &dto.WebsocketResponse{WebsocketUrl: websocketUrl.String()}, http.StatusOK)
	}
}

// The findRunnerMiddleware looks up the runnerId for routes containing it
// and adds the runner to the context of the request.
func findRunnerMiddleware(runnerPool environment.RunnerPool) func(handler http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			// Find runner
			runnerId := mux.Vars(request)[RunnerIdKey]
			r, ok := runnerPool.Get(runnerId)
			if !ok {
				writer.WriteHeader(http.StatusNotFound)
				return
			}
			ctx := runner.NewContext(request.Context(), r.(runner.Runner))
			requestWithRunner := request.WithContext(ctx)
			next.ServeHTTP(writer, requestWithRunner)
		})
	}
}

func deleteRunner(apiClient nomad.ExecutorApi, runnerPool environment.RunnerPool) func(writer http.ResponseWriter, request *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		targetRunner, _ := runner.FromContext(request.Context())

		err := apiClient.DeleteRunner(targetRunner.Id())
		if err != nil {
			writeInternalServerError(writer, err, dto.ErrorNomadInternalServerError)
			return
		}

		runnerPool.Delete(targetRunner.Id())

		writer.WriteHeader(http.StatusNoContent)
	}
}

func registerRunnerRoutes(router *mux.Router, apiClient nomad.ExecutorApi, runnerPool environment.RunnerPool) {
	router.HandleFunc("", provideRunner).Methods(http.MethodPost)
	runnerRouter := router.PathPrefix(fmt.Sprintf("/{%s}", RunnerIdKey)).Subrouter()
	runnerRouter.Use(findRunnerMiddleware(runnerPool))
	runnerRouter.HandleFunc(ExecutePath, executeCommand(runnerRouter)).Methods(http.MethodPost).Name(ExecutePath)
	runnerRouter.HandleFunc(WebsocketPath, connectToRunner).Methods(http.MethodGet).Name(WebsocketPath)
	runnerRouter.HandleFunc("", deleteRunner(apiClient, runnerPool)).Methods(http.MethodDelete).Name(DeleteRoute)
}
