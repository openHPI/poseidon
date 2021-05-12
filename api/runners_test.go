package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/api/dto"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/environment"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/runner"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

type MiddlewareTestSuite struct {
	suite.Suite
	manager          *runner.ManagerMock
	router           *mux.Router
	runnerController *RunnerController
	testRunner       runner.Runner
}

func (suite *MiddlewareTestSuite) SetupTest() {
	suite.manager = &runner.ManagerMock{}
	suite.router = mux.NewRouter()
	suite.runnerController = &RunnerController{suite.manager, suite.router}
	suite.testRunner = runner.NewRunner("runner")
}

func TestMiddlewareTestSuite(t *testing.T) {
	suite.Run(t, new(MiddlewareTestSuite))
}

func (suite *MiddlewareTestSuite) TestFindRunnerMiddleware() {
	var capturedRunner runner.Runner

	testRunnerIdRoute := func(writer http.ResponseWriter, request *http.Request) {
		var ok bool
		capturedRunner, ok = runner.FromContext(request.Context())
		if ok {
			writer.WriteHeader(http.StatusOK)
		} else {
			writer.WriteHeader(http.StatusInternalServerError)
		}
	}
	testRunnerRequest := func(t *testing.T, runnerId string) *http.Request {
		path, err := suite.router.Get("test-runner-id").URL(RunnerIdKey, runnerId)
		if err != nil {
			t.Fatal(err)
		}
		request, err := http.NewRequest(http.MethodPost, path.String(), nil)
		if err != nil {
			t.Fatal(err)
		}
		return request
	}

	suite.router.Use(suite.runnerController.findRunnerMiddleware)
	suite.router.HandleFunc(fmt.Sprintf("/test/{%s}", RunnerIdKey), testRunnerIdRoute).Name("test-runner-id")

	suite.manager.On("Get", suite.testRunner.Id()).Return(suite.testRunner, nil)
	suite.T().Run("sets runner in context if runner exists", func(t *testing.T) {
		capturedRunner = nil

		recorder := httptest.NewRecorder()
		suite.router.ServeHTTP(recorder, testRunnerRequest(t, suite.testRunner.Id()))

		assert.Equal(t, http.StatusOK, recorder.Code)
		assert.Equal(t, suite.testRunner, capturedRunner)
	})

	invalidID := "some-invalid-runner-id"
	suite.manager.On("Get", invalidID).Return(nil, runner.ErrRunnerNotFound)
	suite.T().Run("returns 404 if runner does not exist", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		suite.router.ServeHTTP(recorder, testRunnerRequest(t, invalidID))

		assert.Equal(t, http.StatusNotFound, recorder.Code)
	})
}

func TestRunnerRouteTestSuite(t *testing.T) {
	suite.Run(t, new(RunnerRouteTestSuite))
}

type RunnerRouteTestSuite struct {
	suite.Suite
	runnerManager      *runner.ManagerMock
	environmentManager *environment.ManagerMock
	router             *mux.Router
	runner             runner.Runner
}

func (suite *RunnerRouteTestSuite) SetupTest() {
	suite.runnerManager = &runner.ManagerMock{}
	suite.environmentManager = &environment.ManagerMock{}
	suite.router = NewRouter(suite.runnerManager, suite.environmentManager)
	suite.runner = runner.NewRunner("test_runner")
	suite.runnerManager.On("Get", suite.runner.Id()).Return(suite.runner, nil)
}

func (suite *RunnerRouteTestSuite) TestExecuteRoute() {
	path, err := suite.router.Get(ExecutePath).URL(RunnerIdKey, suite.runner.Id())
	if err != nil {
		suite.T().Fatal()
	}

	suite.Run("valid request", func() {
		recorder := httptest.NewRecorder()
		executionRequest := dto.ExecutionRequest{
			Command:     "command",
			TimeLimit:   10,
			Environment: nil,
		}
		body, err := json.Marshal(executionRequest)
		if err != nil {
			suite.T().Fatal(err)
		}
		request, err := http.NewRequest(http.MethodPost, path.String(), bytes.NewReader(body))
		if err != nil {
			suite.T().Fatal(err)
		}

		suite.router.ServeHTTP(recorder, request)

		var websocketResponse dto.WebsocketResponse
		err = json.NewDecoder(recorder.Result().Body).Decode(&websocketResponse)
		if err != nil {
			suite.T().Fatal(err)
		}

		suite.Equal(http.StatusOK, recorder.Code)

		suite.Run("creates an execution request for the runner", func() {
			url, err := url.Parse(websocketResponse.WebsocketUrl)
			if err != nil {
				suite.T().Fatal(err)
			}
			executionId := url.Query().Get(ExecutionIdKey)
			storedExecutionRequest, ok := suite.runner.Execution(runner.ExecutionId(executionId))

			suite.True(ok, "No execution request with this id: ", executionId)
			suite.Equal(executionRequest, storedExecutionRequest)
		})
	})

	suite.Run("invalid request", func() {
		recorder := httptest.NewRecorder()
		body := ""
		request, err := http.NewRequest(http.MethodPost, path.String(), strings.NewReader(body))
		if err != nil {
			suite.T().Fatal(err)
		}
		suite.router.ServeHTTP(recorder, request)

		suite.Equal(http.StatusBadRequest, recorder.Code)
	})
}

func (suite *RunnerRouteTestSuite) TestDeleteRoute() {
	deleteURL, err := suite.router.Get(DeleteRoute).URL(RunnerIdKey, suite.runner.Id())
	if err != nil {
		suite.T().Fatal(err)
	}
	deletePath := deleteURL.String()
	suite.runnerManager.On("Return", suite.runner).Return(nil)

	suite.Run("valid request", func() {
		recorder := httptest.NewRecorder()
		request, err := http.NewRequest(http.MethodDelete, deletePath, nil)
		if err != nil {
			suite.T().Fatal(err)
		}

		suite.router.ServeHTTP(recorder, request)

		suite.Equal(http.StatusNoContent, recorder.Code)

		suite.Run("runner was returned to runner manager", func() {
			suite.runnerManager.AssertCalled(suite.T(), "Return", suite.runner)
		})
	})
}
