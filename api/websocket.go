package api

import (
	"errors"
	"github.com/gorilla/websocket"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/api/dto"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/runner"
	"net/http"
)

// connectToRunner is the endpoint for websocket connections.
func connectToRunner(writer http.ResponseWriter, request *http.Request) {
	r, _ := runner.FromContext(request.Context())
	executionId := runner.ExecutionId(request.URL.Query().Get(ExecutionIdKey))
	_, ok := r.Execution(executionId)
	if !ok {
		writeNotFound(writer, errors.New("executionId does not exist"))
		return
	}
	log.
		WithField("runnerId", r.Id()).
		WithField("executionId", executionId).
		Info("Running execution")
	connUpgrader := websocket.Upgrader{}
	connClient, err := connUpgrader.Upgrade(writer, request, nil)
	if err != nil {
		log.WithError(err).Warn("Connection upgrade failed")
		return
	}
	err = connClient.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
	if err != nil {
		writeInternalServerError(writer, err, dto.ErrorUnknown)
	}
}
