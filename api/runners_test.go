package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/api/dto"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/environment"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/mocks"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/runner"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestFindRunnerMiddleware(t *testing.T) {
	runnerPool := environment.NewLocalRunnerPool()
	var capturedRunner runner.Runner
	testRunner := runner.NewExerciseRunner("testRunner")
	runnerPool.Add(testRunner)

	testRunnerIdRoute := func(writer http.ResponseWriter, request *http.Request) {
		var ok bool
		capturedRunner, ok = runner.FromContext(request.Context())
		if ok {
			writer.WriteHeader(http.StatusOK)
		} else {
			writer.WriteHeader(http.StatusInternalServerError)
		}
	}
	router := mux.NewRouter()
	router.Use(findRunnerMiddleware(runnerPool))
	router.HandleFunc(fmt.Sprintf("/test/{%s}", RunnerIdKey), testRunnerIdRoute).Name("test-runner-id")

	testRunnerRequest := func(t *testing.T, runnerId string) *http.Request {
		path, err := router.Get("test-runner-id").URL(RunnerIdKey, runnerId)
		if err != nil {
			t.Fatal(err)
		}
		request, err := http.NewRequest(http.MethodPost, path.String(), nil)
		if err != nil {
			t.Fatal(err)
		}
		return request
	}

	t.Run("sets runner in context if runner exists", func(t *testing.T) {
		capturedRunner = nil

		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, testRunnerRequest(t, testRunner.Id()))

		assert.Equal(t, http.StatusOK, recorder.Code)
		assert.Equal(t, testRunner, capturedRunner)
	})

	t.Run("returns 404 if runner does not exist", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, testRunnerRequest(t, "some-invalid-runner-id"))

		assert.Equal(t, http.StatusNotFound, recorder.Code)
	})
}

func TestExecuteRoute(t *testing.T) {
	runnerPool := environment.NewLocalRunnerPool()
	router := NewRouter(nil, runnerPool)
	testRunner := runner.NewExerciseRunner("testRunner")
	runnerPool.Add(testRunner)

	path, err := router.Get(ExecutePath).URL(RunnerIdKey, testRunner.Id())
	if err != nil {
		t.Fatal(err)
	}

	t.Run("valid request", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		executionRequest := dto.ExecutionRequest{
			Command:     "command",
			TimeLimit:   10,
			Environment: nil,
		}
		body, err := json.Marshal(executionRequest)
		if err != nil {
			t.Fatal(err)
		}
		request, err := http.NewRequest(http.MethodPost, path.String(), bytes.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}

		router.ServeHTTP(recorder, request)

		var websocketResponse dto.WebsocketResponse
		err = json.NewDecoder(recorder.Result().Body).Decode(&websocketResponse)
		if err != nil {
			t.Fatal(err)
		}

		assert.Equal(t, http.StatusOK, recorder.Code)

		t.Run("creates an execution request for the runner", func(t *testing.T) {
			url, err := url.Parse(websocketResponse.WebsocketUrl)
			if err != nil {
				t.Fatal(err)
			}
			executionId := url.Query().Get(ExecutionIdKey)
			storedExecutionRequest, ok := testRunner.Execution(runner.ExecutionId(executionId))

			assert.True(t, ok, "No execution request with this id: ", executionId)
			assert.Equal(t, executionRequest, storedExecutionRequest)
		})
	})

	t.Run("invalid request", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		body := ""
		request, err := http.NewRequest(http.MethodPost, path.String(), strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		router.ServeHTTP(recorder, request)

		assert.Equal(t, http.StatusBadRequest, recorder.Code)
	})
}

func TestDeleteRunnerRouteTestSuite(t *testing.T) {
	suite.Run(t, new(DeleteRunnerRouteTestSuite))
}

type DeleteRunnerRouteTestSuite struct {
	suite.Suite
	runnerPool environment.RunnerPool
	apiClient  *mocks.ExecutorApi
	router     *mux.Router
	testRunner runner.Runner
	path       string
}

func (suite *DeleteRunnerRouteTestSuite) SetupTest() {
	suite.runnerPool = environment.NewLocalRunnerPool()
	suite.apiClient = &mocks.ExecutorApi{}
	suite.router = NewRouter(suite.apiClient, suite.runnerPool)

	suite.testRunner = runner.NewExerciseRunner("testRunner")
	suite.runnerPool.Add(suite.testRunner)

	var err error
	runnerUrl, err := suite.router.Get(DeleteRoute).URL(RunnerIdKey, suite.testRunner.Id())
	if err != nil {
		suite.T().Fatal(err)
	}
	suite.path = runnerUrl.String()
}

func (suite *DeleteRunnerRouteTestSuite) TestValidRequestReturnsNoContent() {
	suite.apiClient.On("DeleteRunner", mock.AnythingOfType("string")).Return(nil)

	recorder := httptest.NewRecorder()
	request, err := http.NewRequest(http.MethodDelete, suite.path, nil)
	if err != nil {
		suite.T().Fatal(err)
	}

	suite.router.ServeHTTP(recorder, request)

	suite.Equal(http.StatusNoContent, recorder.Code)

	suite.Run("runner is deleted on nomad", func() {
		suite.apiClient.AssertCalled(suite.T(), "DeleteRunner", suite.testRunner.Id())
	})

	suite.Run("runner is deleted from runnerPool", func() {
		returnedRunner, ok := suite.runnerPool.Get(suite.testRunner.Id())
		suite.Nil(returnedRunner)
		suite.False(ok)
	})
}

func (suite *DeleteRunnerRouteTestSuite) TestReturnInternalServerErrorWhenApiCallToNomadFailed() {
	suite.apiClient.On("DeleteRunner", mock.AnythingOfType("string")).Return(errors.New("API call failed"))

	recorder := httptest.NewRecorder()
	request, err := http.NewRequest(http.MethodDelete, suite.path, nil)
	if err != nil {
		suite.T().Fatal(err)
	}

	suite.router.ServeHTTP(recorder, request)

	suite.Equal(http.StatusInternalServerError, recorder.Code)
}

func (suite *DeleteRunnerRouteTestSuite) TestDeleteInvalidRunnerIdReturnsNotFound() {
	var err error
	runnersUrl, err := suite.router.Get(DeleteRoute).URL(RunnerIdKey, "1nv4l1dID")
	if err != nil {
		suite.T().Fatal(err)
	}
	suite.path = runnersUrl.String()

	recorder := httptest.NewRecorder()
	request, err := http.NewRequest(http.MethodDelete, suite.path, nil)
	if err != nil {
		suite.T().Fatal(err)
	}

	suite.router.ServeHTTP(recorder, request)

	suite.Equal(http.StatusNotFound, recorder.Code)
}
