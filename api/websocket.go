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
	r, _ := runner.FromContext(request.Context())
	executionId := request.URL.Query().Get(ExecutionIdKey)
	executionRequest, ok := r.Execution(runner.ExecutionId(executionId))
	if !ok {
		writeNotFound(writer, errors.New("executionId does not exist"))
		return
	}
	log.
		WithField("executionId", executionId).
		WithField("command", executionRequest.Command).
		Info("Running execution")
	connClient, err := connUpgrade.Upgrade(writer, request, nil)
	if err != nil {
		writeInternalServerError(writer, err, dto.ErrorUnknown)
		return
	}
	defer func(connClient *websocket.Conn) {
		err := connClient.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
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
