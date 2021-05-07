package api

import (
	"errors"
	"github.com/gorilla/websocket"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/api/dto"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/runner"
	"net/http"
)

var connUpgrade = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

// connectToRunner is a placeholder for now and will become the endpoint for websocket connections.
func connectToRunner(writer http.ResponseWriter, request *http.Request) {
	r, ok := runner.FromContext(request.Context())
	if !ok {
		log.Error("Runner not set in request context.")
		writeInternalServerError(writer, errors.New("findRunnerMiddleware failure"), dto.ErrorUnknown)
		return
	}
	executionId := request.URL.Query().Get(ExecutionIdKey)
	connClient, err := connUpgrade.Upgrade(writer, request, nil)
	if err != nil {
		writeInternalServerError(writer, err, dto.ErrorUnknown)
		return
	}
	defer func(connClient *websocket.Conn) {
		err := connClient.Close()
		if err != nil {
			writeInternalServerError(writer, err, dto.ErrorUnknown)
		}
	}(connClient)

	// ToDo: Implement communication forwarding
	err, ok = r.Execute(runner.ExecutionId(executionId))
	if !ok {
		writeBadRequest(writer, errors.New("invalid Execution Id"))
		return
	} else if err != nil {
		writeInternalServerError(writer, err, dto.ErrorUnknown)
		return
	}
}
