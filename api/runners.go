package api

import (
	"encoding/json"
	"github.com/gorilla/mux"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/api/dto"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/environment"
	"net/http"
)

// ProvideRunner tries to respond with the id of a runner.
// This runner is then reserved for future use.
func ProvideRunner(writer http.ResponseWriter, request *http.Request) {
	runnerRequest := new(dto.RequestRunner)
	if err := json.NewDecoder(request.Body).Decode(runnerRequest); err != nil {
		writeBadRequest(writer, err)
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

	sendJson(writer, &dto.ResponseRunner{Id: runner.Id()}, http.StatusOK)
}

func registerRunnerRoutes(router *mux.Router) {
	router.HandleFunc("", ProvideRunner).Methods(http.MethodPost)
}
