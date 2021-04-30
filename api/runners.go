package api

import (
	"errors"
	"fmt"
	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/api/dto"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/config"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/environment"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/runner"
	"log"
	"net/http"
	"sync"
)

// ProvideRunner tries to respond with the id of a runner
// This runner is then reserved for future use
func provideRunner(writer http.ResponseWriter, request *http.Request) {
	runnerRequest := new(dto.RunnerRequest)
	if err := parseRequestBodyJSON(writer, request, runnerRequest); err != nil {
		return
	}
	environment, err := environment.GetExecutionEnvironment(runnerRequest.ExecutionEnvironmentId)
	if err != nil {
		writeNotFound(writer, err)
		return
	}
	runner, err := environment.NextRunner()
	if err != nil {
		writeInternalServerError(writer, err, dto.ErrorNomadOverload)
		return
	}
	executionsLock.Lock()
	executions[runner.Id] = make(map[string]dto.ExecutionRequest)
	executionsLock.Unlock()
	sendJson(writer, &dto.RunnerResponse{Id: runner.Id}, http.StatusOK)
}

var (
	executions     = make(map[string]map[string]dto.ExecutionRequest)
	executionsLock = sync.Mutex{}
)

func executeCommand(writer http.ResponseWriter, request *http.Request) {
	executionRequest := new(dto.ExecutionRequest)
	if err := parseRequestBodyJSON(writer, request, executionRequest); err != nil {
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
		log.Fatal("Expected runner in context! Something must be broken ...")
	}

	id, err := uuid.NewRandom()
	if err != nil {
		log.Printf("Error creating new execution id: %v", err)
		writeInternalServerError(writer, err, dto.ErrorUnknown)
		return
	}

	executionsLock.Lock()
	runnerExecutions, ok := executions[r.Id]
	if !ok {
		writeNotFound(writer, errors.New("runner has not been provided"))
		return
	}
	runnerExecutions[id.String()] = *executionRequest
	executionsLock.Unlock()

	path, err := router.Get("runner-websocket").URL("runnerId", r.Id)
	if err != nil {
		log.Printf("Error creating runner websocket URL %v", err)
		writeInternalServerError(writer, err, dto.ErrorUnknown)
		return
	}
	websocketUrl := fmt.Sprintf("%s://%s%s?executionId=%s", scheme, request.Host, path, id)
	sendJson(writer, &dto.WebsocketResponse{WebsocketUrl: websocketUrl}, http.StatusOK)
}

func connectToRunner(writer http.ResponseWriter, request *http.Request) {
	// Upgrade the connection to websocket
	executionId := request.URL.Query()["executionId"]
	log.Printf("Websocket for execution %s requested", executionId)
	writer.WriteHeader(http.StatusNotImplemented)
}

func findRunnerMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		// Find runner
		runnerId := mux.Vars(request)["runnerId"]
		// TODO: Get runner from runner store using runnerId
		env, err := execution_environment.GetExecutionEnvironment(1)
		if err != nil {
			writeNotFound(writer, err)
			return
		}
		r, ok := env.Runners[runnerId]
		if !ok {
			writer.WriteHeader(http.StatusNotFound)
			return
		}
		ctx := runner.NewContext(request.Context(), r)
		requestWithRunner := request.WithContext(ctx)
		next.ServeHTTP(writer, requestWithRunner)
	})
}

func registerRunnerRoutes(router *mux.Router) {
	router.HandleFunc("", provideRunner).Methods(http.MethodPost)
	runnerRouter := router.PathPrefix("/{runnerId}").Subrouter()
	runnerRouter.Use(findRunnerMiddleware)
	runnerRouter.HandleFunc("/execute", executeCommand).Methods(http.MethodPost).Name("runner-execute")
	runnerRouter.HandleFunc("/websocket", connectToRunner).Methods(http.MethodGet).Name("runner-websocket")
}
