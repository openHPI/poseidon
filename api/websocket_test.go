package api

import (
	"fmt"
	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/suite"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/api/dto"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/environment"
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

func TestWebsocketTestSuite(t *testing.T) {
	suite.Run(t, new(WebsocketTestSuite))
}

func (suite *WebsocketTestSuite) SetupSuite() {
	runnerPool := environment.NewLocalRunnerPool()
	suite.runner = runner.NewExerciseRunner("testRunner")
	runnerPool.Add(suite.runner)
	var err error
	suite.executionId, err = suite.runner.AddExecution(dto.ExecutionRequest{
		Command:     "command",
		TimeLimit:   10,
		Environment: nil,
	})
	suite.Require().NoError(err)

	router := mux.NewRouter()
	router.Use(findRunnerMiddleware(runnerPool))
	router.HandleFunc(fmt.Sprintf("%s/{%s}%s", RouteRunners, RunnerIdKey, WebsocketPath), connectToRunner).Methods(http.MethodGet).Name(WebsocketPath)
	suite.server = httptest.NewServer(router)
	suite.router = router
}

func (suite *WebsocketTestSuite) websocketUrl(scheme, runnerId string, executionId runner.ExecutionId) (*url.URL, error) {
	websocketUrl, err := url.Parse(suite.server.URL)
	suite.Require().NoError(err, "Error: parsing test server url")
	path, err := suite.router.Get(WebsocketPath).URL(RunnerIdKey, runnerId)
	suite.Require().NoError(err, "could not set runnerId")
	websocketUrl.Scheme = scheme
	websocketUrl.Path = path.Path
	websocketUrl.RawQuery = fmt.Sprintf("executionId=%s", executionId)
	return websocketUrl, nil
}

func (suite *WebsocketTestSuite) TearDownSuite() {
	suite.server.Close()
}

func (suite *WebsocketTestSuite) TestWebsocketConnectionCanBeEstablished() {
	path, err := suite.websocketUrl("ws", suite.runner.Id(), suite.executionId)
	suite.Require().NoError(err)
	_, _, err = websocket.DefaultDialer.Dial(path.String(), nil)
	suite.Require().NoError(err)
}

func (suite *WebsocketTestSuite) TestWebsocketReturns404IfExecutionDoesNotExist() {
	wsUrl, err := suite.websocketUrl("ws", suite.runner.Id(), "invalid-execution-id")
	suite.Require().NoError(err)
	_, response, _ := websocket.DefaultDialer.Dial(wsUrl.String(), nil)
	suite.Equal(http.StatusNotFound, response.StatusCode)
}

func (suite *WebsocketTestSuite) TestWebsocketReturns400IfRequestedViaHttp() {
	wsUrl, err := suite.websocketUrl("http", suite.runner.Id(), suite.executionId)
	suite.Require().NoError(err)
	response, err := http.Get(wsUrl.String())
	suite.Require().NoError(err)
	suite.Equal(http.StatusBadRequest, response.StatusCode)
}
