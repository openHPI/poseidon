package api

import (
	"context"
	"errors"
	"fmt"
	"github.com/gorilla/websocket"
	"github.com/openHPI/poseidon/internal/api/ws"
	"github.com/openHPI/poseidon/internal/runner"
	"github.com/openHPI/poseidon/pkg/dto"
	"github.com/openHPI/poseidon/pkg/logging"
	"github.com/openHPI/poseidon/pkg/monitoring"
	"net/http"
)

var ErrUnknownExecutionID = errors.New("execution id unknown")

// webSocketProxy is an encapsulation of logic for forwarding between Runners and CodeOcean.
type webSocketProxy struct {
	ctx    context.Context
	cancel context.CancelFunc
	Input  ws.WebSocketReader
	Output ws.WebSocketWriter
}

// upgradeConnection upgrades a connection to a websocket and returns a webSocketProxy for this connection.
func upgradeConnection(writer http.ResponseWriter, request *http.Request) (ws.Connection, error) {
	connUpgrader := websocket.Upgrader{}
	connection, err := connUpgrader.Upgrade(writer, request, nil)
	if err != nil {
		log.WithError(err).Warn("Connection upgrade failed")
		return nil, fmt.Errorf("error upgrading the connection: %w", err)
	}
	return connection, nil
}

// newWebSocketProxy returns an initiated and started webSocketProxy.
// As this proxy is already started, a start message is send to the client.
func newWebSocketProxy(connection ws.Connection, proxyCtx context.Context) *webSocketProxy {
	wsCtx, cancelWsCommunication := context.WithCancel(proxyCtx)
	proxy := &webSocketProxy{
		ctx:    wsCtx,
		cancel: cancelWsCommunication,
		Input:  ws.NewCodeOceanToRawReader(connection, wsCtx, proxyCtx),
		Output: ws.NewCodeOceanOutputWriter(connection, wsCtx),
	}

	connection.SetCloseHandler(func(code int, text string) error {
		log.WithField("code", code).WithField("text", text).Debug("The client closed the connection.")
		cancelWsCommunication()
		return nil
	})
	return proxy
}

// waitForExit waits for an exit of either the runner (when the command terminates) or the client closing the WebSocket
// and handles WebSocket exit messages.
func (wp *webSocketProxy) waitForExit(exit <-chan runner.ExitInfo, cancelExecution context.CancelFunc) {
	wp.Input.Start()

	var exitInfo runner.ExitInfo
	select {
	case <-wp.ctx.Done():
		log.Info("Client closed the connection")
		wp.Input.Stop()
		cancelExecution()
		<-exit // /internal/runner/runner.go handleExitOrContextDone does not require client connection anymore.
		<-exit // The goroutine closes this channel indicating that it does not use the connection to the executor anymore.
	case exitInfo = <-exit:
		log.Info("Execution returned")
		wp.Input.Stop()
		wp.Output.SendExitInfo(&exitInfo)
	}
}

// connectToRunner is the endpoint for websocket connections.
func (r *RunnerController) connectToRunner(writer http.ResponseWriter, request *http.Request) {
	targetRunner, _ := runner.FromContext(request.Context())
	monitoring.AddRunnerMonitoringData(request, targetRunner.ID(), targetRunner.Environment())

	executionID := request.URL.Query().Get(ExecutionIDKey)
	if !targetRunner.ExecutionExists(executionID) {
		writeClientError(writer, ErrUnknownExecutionID, http.StatusNotFound)
		return
	}

	connection, err := upgradeConnection(writer, request)
	if err != nil {
		writeInternalServerError(writer, err, dto.ErrorUnknown)
		return
	}
	proxyCtx, cancelProxy := context.WithCancel(context.Background())
	defer cancelProxy()
	proxy := newWebSocketProxy(connection, proxyCtx)

	log.WithField("runnerId", targetRunner.ID()).
		WithField("executionID", logging.RemoveNewlineSymbol(executionID)).
		Info("Running execution")
	logging.StartSpan("api.runner.connect", "Execute Interactively", request.Context(), func(_ context.Context) {
		exit, cancel, err := targetRunner.ExecuteInteractively(executionID,
			proxy.Input, proxy.Output.StdOut(), proxy.Output.StdErr())
		if err != nil {
			log.WithError(err).Warn("Cannot execute request.")
			return // The proxy is stopped by the deferred cancel.
		}

		proxy.waitForExit(exit, cancel)
	})
}
