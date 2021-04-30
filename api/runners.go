package api

import (
	"errors"
	"fmt"
	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/api/dto"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/config"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/environment"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/environment/pool"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/runner"
	"net/http"
	"sync"
)

var (
	executions     = make(map[string]map[string]dto.ExecutionRequest)
	executionsLock = sync.Mutex{}
)

func allocateExecutionMap(runner runner.Runner) {
	executionsLock.Lock()
	executions[runner.Id()] = make(map[string]dto.ExecutionRequest)
	executionsLock.Unlock()
}

// provideRunner tries to respond with the id of a runner
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
	allocateExecutionMap(runner)
	sendJson(writer, &dto.RunnerResponse{Id: runner.Id()}, http.StatusOK)
}

// executeCommand takes an ExecutionRequest and stores it for a runner.
// It returns a url to connect to for a websocket connection to this execution in the corresponding runner.
func executeCommand(router *mux.Router) func(w http.ResponseWriter, r *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
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
		runnerExecutions, ok := executions[r.Id()]
		if !ok {
			writeNotFound(writer, errors.New("runner has not been provided"))
			return
		}
		runnerExecutions[id.String()] = *executionRequest
		executionsLock.Unlock()

		path, err := router.Get("runner-websocket").URL("runnerId", r.Id())
		if err != nil {
			log.Printf("Error creating runner websocket URL %v", err)
			writeInternalServerError(writer, err, dto.ErrorUnknown)
			return
		}
		websocketUrl := fmt.Sprintf("%s://%s%s?executionId=%s", scheme, request.Host, path, id)
		sendJson(writer, &dto.WebsocketResponse{WebsocketUrl: websocketUrl}, http.StatusOK)
	}
}

func connectToRunner(writer http.ResponseWriter, request *http.Request) {
	// Todo: Execute the command, upgrade the connection to websocket and handle forwarding
	executionId := request.URL.Query()["executionId"]
	log.Printf("Websocket for execution %s requested", executionId)
	writer.WriteHeader(http.StatusNotImplemented)
}

// The findRunnerMiddleware looks up the runnerId for routes containing it
// and adds the runner to the context of the request.
func findRunnerMiddleware(runnerPool pool.RunnerPool) func(handler http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			// Find runner
			runnerId := mux.Vars(request)["runnerId"]
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
	runnerRouter := router.PathPrefix("/{runnerId}").Subrouter()
	runnerRouter.Use(findRunnerMiddleware(runnerPool))
	runnerRouter.HandleFunc("/execute", executeCommand(runnerRouter)).Methods(http.MethodPost).Name("runner-execute")
	runnerRouter.HandleFunc("/websocket", connectToRunner).Methods(http.MethodGet).Name("runner-websocket")
}
