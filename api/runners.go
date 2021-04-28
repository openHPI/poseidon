package api

import (
	"errors"
	"fmt"
	"github.com/gorilla/mux"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/api/dto"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/config"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/environment"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/environment/pool"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/runner"
	"net/http"
	"net/url"
)

const (
	ExecutePath    = "/execute"
	WebsocketPath  = "/websocket"
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
	runner, err := env.NextRunner()
	if err != nil {
		writeInternalServerError(writer, err, dto.ErrorNomadOverload)
		return
	}
	sendJson(writer, &dto.RunnerResponse{Id: runner.Id()}, http.StatusOK)
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
		r, ok := runner.FromContext(request.Context())
		if !ok {
			log.Error("Runner not set in request context.")
			writeInternalServerError(writer, errors.New("findRunnerMiddleware failure"), dto.ErrorUnknown)
			return
		}

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

// connectToRunner is a placeholder for now and will become the endpoint for websocket connections.
func connectToRunner(writer http.ResponseWriter, request *http.Request) {
	// Todo: Execute the command, upgrade the connection to websocket and handle forwarding
	executionId := request.URL.Query()[ExecutionIdKey]
	log.WithField("executionId", executionId).Info("Websocket for execution requested.")
	writer.WriteHeader(http.StatusNotImplemented)
}

// The findRunnerMiddleware looks up the runnerId for routes containing it
// and adds the runner to the context of the request.
func findRunnerMiddleware(runnerPool pool.RunnerPool) func(handler http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			// Find runner
			runnerId := mux.Vars(request)[RunnerIdKey]
			r, ok := runnerPool.GetRunner(runnerId)
			if !ok {
				writer.WriteHeader(http.StatusNotFound)
				return
			}
			ctx := runner.NewContext(request.Context(), r)
			requestWithRunner := request.WithContext(ctx)
			next.ServeHTTP(writer, requestWithRunner)
		})
	}
}

func registerRunnerRoutes(router *mux.Router, runnerPool pool.RunnerPool) {
	router.HandleFunc("", provideRunner).Methods(http.MethodPost)
	runnerRouter := router.PathPrefix(fmt.Sprintf("/{%s}", RunnerIdKey)).Subrouter()
	runnerRouter.Use(findRunnerMiddleware(runnerPool))
	runnerRouter.HandleFunc(ExecutePath, executeCommand(runnerRouter)).Methods(http.MethodPost).Name(ExecutePath)
	runnerRouter.HandleFunc(WebsocketPath, connectToRunner).Methods(http.MethodGet).Name(WebsocketPath)
}
