package api

import (
	"errors"
	"fmt"
	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/suite"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/api/dto"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/environment/pool"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/runner"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

type WebsocketTestSuite struct {
	suite.Suite
	runner      runner.Runner
	server      *httptest.Server
	router      *mux.Router
	executionId runner.ExecutionId
}

func (suite *WebsocketTestSuite) SetupSuite() {
	runnerPool := pool.NewLocalRunnerPool()
	suite.runner = runner.NewExerciseRunner("testRunner")
	runnerPool.AddRunner(suite.runner)
	var err error
	suite.executionId, err = suite.runner.AddExecution(dto.ExecutionRequest{
		Command:     "command",
		TimeLimit:   10,
		Environment: nil,
	})
	if !suite.NoError(err) {
		return
	}

	router := mux.NewRouter()
	router.Use(findRunnerMiddleware(runnerPool))
	router.HandleFunc(fmt.Sprintf("%s/{%s}%s", RouteRunners, RunnerIdKey, WebsocketPath), connectToRunner).Methods(http.MethodGet).Name(WebsocketPath)
	suite.server = httptest.NewServer(router)
	suite.router = router
}

func (suite *WebsocketTestSuite) url(scheme, runnerId string, executionId runner.ExecutionId) (*url.URL, error) {
	websocketUrl, err := url.Parse(suite.server.URL)
	if !suite.NoError(err, "Error: parsing test server url") {
		return nil, errors.New("could not parse server url")
	}
	path, err := suite.router.Get(WebsocketPath).URL(RunnerIdKey, runnerId)
	if !suite.NoError(err) {
		return nil, errors.New("could not set runnerId")
	}
	websocketUrl.Scheme = scheme
	websocketUrl.Path = path.Path
	websocketUrl.RawQuery = fmt.Sprintf("executionId=%s", executionId)
	return websocketUrl, nil
}

func (suite *WebsocketTestSuite) TearDownSuite() {
	suite.server.Close()
}

func TestWebsocketTestSuite(t *testing.T) {
	suite.Run(t, new(WebsocketTestSuite))
}

func (suite *WebsocketTestSuite) TestEstablishWebsocketConnection() {
	path, err := suite.url("ws", suite.runner.Id(), suite.executionId)
	if !suite.NoError(err) {
		return
	}
	_, _, err = websocket.DefaultDialer.Dial(path.String(), nil)
	if !suite.NoError(err) {
		return
	}
}

func (suite *WebsocketTestSuite) TestWebsocketReturns404IfExecutionDoesNotExist() {
	wsUrl, err := suite.url("http", suite.runner.Id(), "invalid-execution-id")
	if !suite.NoError(err) {
		return
	}
	response, err := http.Get(wsUrl.String())
	if !suite.NoError(err) {
		return
	}
	suite.Equal(http.StatusNotFound, response.StatusCode)
}
