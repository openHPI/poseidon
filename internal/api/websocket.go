package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/getsentry/sentry-go"
	"github.com/gorilla/websocket"
	"github.com/openHPI/poseidon/internal/api/ws"
	"github.com/openHPI/poseidon/internal/runner"
	"github.com/openHPI/poseidon/pkg/dto"
	"github.com/openHPI/poseidon/pkg/logging"
)

var ErrUnknownExecutionID = errors.New("execution id unknown")

// webSocketProxy is an encapsulation of logic for forwarding between Runners and CodeOcean.
type webSocketProxy struct {
	//nolint:containedctx // See #630.
	ctx    context.Context
	Input  ws.WebSocketReader
	Output ws.WebSocketWriter
}

// upgradeConnection upgrades a connection to a websocket and returns a webSocketProxy for this connection.
func upgradeConnection(writer http.ResponseWriter, request *http.Request) (ws.Connection, error) {
	connUpgrader := websocket.Upgrader{}

	connection, err := connUpgrader.Upgrade(writer, request, nil)
	if err != nil {
		log.WithContext(request.Context()).WithError(err).Warn("Connection upgrade failed")
		return nil, fmt.Errorf("error upgrading the connection: %w", err)
	}

	return connection, nil
}

// newWebSocketProxy returns an initiated and started webSocketProxy.
// As this proxy is already started, a start message is send to the client.
func newWebSocketProxy(proxyCtx context.Context, connection ws.Connection) *webSocketProxy {
	// wsCtx is detached from the proxyCtx
	// as it should send all messages in the buffer even shortly after the execution/proxy is done.
	wsCtx := context.WithoutCancel(proxyCtx)
	wsCtx, cancelWsCommunication := context.WithCancel(wsCtx)

	proxy := &webSocketProxy{
		ctx:    wsCtx,
		Input:  ws.NewCodeOceanToRawReader(wsCtx, proxyCtx, connection),
		Output: ws.NewCodeOceanOutputWriter(wsCtx, connection, cancelWsCommunication),
	}

	connection.SetCloseHandler(func(code int, text string) error {
		log.WithContext(wsCtx).WithField("code", code).WithField("text", text).Debug("The client closed the connection.")
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
		log.WithContext(wp.ctx).Info("Client closed the connection")
		wp.Input.Stop()
		wp.Output.Close(nil)
		cancelExecution()
		<-exit // /internal/runner/runner.go handleExitOrContextDone does not require client connection anymore.
		<-exit // The goroutine closes this channel indicating that it does not use the connection to the executor anymore.
	case exitInfo = <-exit:
		log.WithContext(wp.ctx).Info("Execution returned")
		wp.Input.Stop()
		wp.Output.Close(&exitInfo)
	}
}

// connectToRunner is the endpoint for websocket connections.
func (r *RunnerController) connectToRunner(writer http.ResponseWriter, request *http.Request) {
	targetRunner, _ := runner.FromContext(request.Context())

	executionID := request.URL.Query().Get(ExecutionIDKey)
	if !targetRunner.ExecutionExists(executionID) {
		writeClientError(request.Context(), writer, ErrUnknownExecutionID, http.StatusNotFound)
		return
	}

	connection, err := upgradeConnection(writer, request)
	if err != nil {
		writeInternalServerError(request.Context(), writer, err, dto.ErrorUnknown)
		return
	}

	// We do not inherit from the request.Context() here because we rely on the WebSocket Close Handler.
	proxyCtx := context.WithoutCancel(request.Context())

	proxyCtx, cancelProxy := context.WithCancel(proxyCtx)
	defer cancelProxy()

	proxy := newWebSocketProxy(proxyCtx, connection)

	log.WithContext(proxyCtx).
		WithField("executionID", logging.RemoveNewlineSymbol(executionID)).
		Info("Running execution")
	logging.StartSpan(request.Context(), "api.runner.connect", "Execute Interactively", func(ctx context.Context, _ *sentry.Span) {
		exit, cancel, err := targetRunner.ExecuteInteractively(ctx, executionID,
			proxy.Input, proxy.Output.StdOut(), proxy.Output.StdErr())
		if err != nil {
			log.WithContext(ctx).WithError(err).Warn("Cannot execute request.")
			return // The proxy is stopped by the deferred cancel.
		}

		proxy.waitForExit(exit, cancel)
	})
}
