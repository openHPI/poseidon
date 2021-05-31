package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/api/dto"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/runner"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/tests"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

type MiddlewareTestSuite struct {
	suite.Suite
	manager        *runner.ManagerMock
	router         *mux.Router
	runner         runner.Runner
	capturedRunner runner.Runner
	runnerRequest  func(string) *http.Request
}

func (suite *MiddlewareTestSuite) SetupTest() {
	suite.manager = &runner.ManagerMock{}
	suite.runner = runner.NewNomadAllocation("runner", nil)
	suite.capturedRunner = nil
	suite.runnerRequest = func(runnerId string) *http.Request {
		path, err := suite.router.Get("test-runner-id").URL(RunnerIdKey, runnerId)
		if err != nil {
			suite.T().Fatal(err)
		}
		request, err := http.NewRequest(http.MethodPost, path.String(), nil)
		if err != nil {
			suite.T().Fatal(err)
		}
		return request
	}
	runnerRouteHandler := func(writer http.ResponseWriter, request *http.Request) {
		var ok bool
		suite.capturedRunner, ok = runner.FromContext(request.Context())
		if ok {
			writer.WriteHeader(http.StatusOK)
		} else {
			writer.WriteHeader(http.StatusInternalServerError)
		}
	}
	suite.router = mux.NewRouter()
	runnerController := &RunnerController{suite.manager, suite.router}
	suite.router.Use(runnerController.findRunnerMiddleware)
	suite.router.HandleFunc(fmt.Sprintf("/test/{%s}", RunnerIdKey), runnerRouteHandler).Name("test-runner-id")
}

func TestMiddlewareTestSuite(t *testing.T) {
	suite.Run(t, new(MiddlewareTestSuite))
}

func (suite *MiddlewareTestSuite) TestFindRunnerMiddlewareIfRunnerExists() {
	suite.manager.On("Get", suite.runner.Id()).Return(suite.runner, nil)

	recorder := httptest.NewRecorder()
	suite.router.ServeHTTP(recorder, suite.runnerRequest(suite.runner.Id()))

	suite.Equal(http.StatusOK, recorder.Code)
	suite.Equal(suite.runner, suite.capturedRunner)
}

func (suite *MiddlewareTestSuite) TestFindRunnerMiddlewareIfRunnerDoesNotExist() {
	invalidID := "some-invalid-runner-id"
	suite.manager.On("Get", invalidID).Return(nil, runner.ErrRunnerNotFound)

	recorder := httptest.NewRecorder()
	suite.router.ServeHTTP(recorder, suite.runnerRequest(invalidID))

	suite.Equal(http.StatusNotFound, recorder.Code)
}

func TestRunnerRouteTestSuite(t *testing.T) {
	suite.Run(t, new(RunnerRouteTestSuite))
}

type RunnerRouteTestSuite struct {
	suite.Suite
	runnerManager *runner.ManagerMock
	router        *mux.Router
	runner        runner.Runner
	executionId   runner.ExecutionId
}

func (suite *RunnerRouteTestSuite) SetupTest() {
	suite.runnerManager = &runner.ManagerMock{}
	suite.router = NewRouter(suite.runnerManager, nil)
	suite.runner = runner.NewNomadAllocation("some-id", nil)
	suite.executionId = "execution-id"
	suite.runner.Add(suite.executionId, &dto.ExecutionRequest{})
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

		var webSocketResponse dto.ExecutionResponse
		err = json.NewDecoder(recorder.Result().Body).Decode(&webSocketResponse)
		if err != nil {
			suite.T().Fatal(err)
		}

		suite.Equal(http.StatusOK, recorder.Code)

		suite.Run("creates an execution request for the runner", func() {
			webSocketUrl, err := url.Parse(webSocketResponse.WebSocketUrl)
			if err != nil {
				suite.T().Fatal(err)
			}
			executionId := webSocketUrl.Query().Get(ExecutionIdKey)
			storedExecutionRequest, ok := suite.runner.Pop(runner.ExecutionId(executionId))

			suite.True(ok, "No execution request with this id: ", executionId)
			suite.Equal(&executionRequest, storedExecutionRequest)
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

func TestUpdateFileSystemRouteTestSuite(t *testing.T) {
	suite.Run(t, new(UpdateFileSystemRouteTestSuite))
}

type UpdateFileSystemRouteTestSuite struct {
	RunnerRouteTestSuite
	path       string
	recorder   *httptest.ResponseRecorder
	runnerMock *runner.RunnerMock
}

func (s *UpdateFileSystemRouteTestSuite) SetupTest() {
	s.RunnerRouteTestSuite.SetupTest()
	routeUrl, err := s.router.Get(UpdateFileSystemPath).URL(RunnerIdKey, tests.DefaultMockId)
	if err != nil {
		s.T().Fatal(err)
	}
	s.path = routeUrl.String()
	s.runnerMock = &runner.RunnerMock{}
	s.runnerManager.On("Get", tests.DefaultMockId).Return(s.runnerMock, nil)
	s.recorder = httptest.NewRecorder()
}

func (s *UpdateFileSystemRouteTestSuite) TestUpdateFileSystemReturnsNoContentOnValidRequest() {
	s.runnerMock.On("UpdateFileSystem", mock.AnythingOfType("*dto.UpdateFileSystemRequest")).Return(nil)

	copyRequest := dto.UpdateFileSystemRequest{}
	body, _ := json.Marshal(copyRequest)
	request, _ := http.NewRequest(http.MethodPatch, s.path, bytes.NewReader(body))

	s.router.ServeHTTP(s.recorder, request)
	s.Equal(http.StatusNoContent, s.recorder.Code)
	s.runnerMock.AssertCalled(s.T(), "UpdateFileSystem", mock.AnythingOfType("*dto.UpdateFileSystemRequest"))
}

func (s *UpdateFileSystemRouteTestSuite) TestUpdateFileSystemReturnsBadRequestOnInvalidRequestBody() {
	request, _ := http.NewRequest(http.MethodPatch, s.path, strings.NewReader(""))

	s.router.ServeHTTP(s.recorder, request)
	s.Equal(http.StatusBadRequest, s.recorder.Code)
}

func (s *UpdateFileSystemRouteTestSuite) TestUpdateFileSystemToNonExistingRunnerReturnsNotFound() {
	invalidID := "some-invalid-runner-id"
	s.runnerManager.On("Get", invalidID).Return(nil, runner.ErrRunnerNotFound)
	path, _ := s.router.Get(UpdateFileSystemPath).URL(RunnerIdKey, invalidID)
	copyRequest := dto.UpdateFileSystemRequest{}
	body, _ := json.Marshal(copyRequest)
	request, _ := http.NewRequest(http.MethodPatch, path.String(), bytes.NewReader(body))

	s.router.ServeHTTP(s.recorder, request)
	s.Equal(http.StatusNotFound, s.recorder.Code)
}

func (s *UpdateFileSystemRouteTestSuite) TestUpdateFileSystemReturnsInternalServerErrorWhenCopyFailed() {
	s.runnerMock.
		On("UpdateFileSystem", mock.AnythingOfType("*dto.UpdateFileSystemRequest")).
		Return(runner.ErrorFileCopyFailed)

	copyRequest := dto.UpdateFileSystemRequest{}
	body, _ := json.Marshal(copyRequest)
	request, _ := http.NewRequest(http.MethodPatch, s.path, bytes.NewReader(body))

	s.router.ServeHTTP(s.recorder, request)
	s.Equal(http.StatusInternalServerError, s.recorder.Code)
}

func TestDeleteRunnerRouteTestSuite(t *testing.T) {
	suite.Run(t, new(DeleteRunnerRouteTestSuite))
}

type DeleteRunnerRouteTestSuite struct {
	RunnerRouteTestSuite
	path string
}

func (suite *DeleteRunnerRouteTestSuite) SetupTest() {
	suite.RunnerRouteTestSuite.SetupTest()
	deleteURL, err := suite.router.Get(DeleteRoute).URL(RunnerIdKey, suite.runner.Id())
	if err != nil {
		suite.T().Fatal(err)
	}
	suite.path = deleteURL.String()
}

func (suite *DeleteRunnerRouteTestSuite) TestValidRequestReturnsNoContent() {
	suite.runnerManager.On("Return", suite.runner).Return(nil)

	recorder := httptest.NewRecorder()
	request, err := http.NewRequest(http.MethodDelete, suite.path, nil)
	if err != nil {
		suite.T().Fatal(err)
	}

	suite.router.ServeHTTP(recorder, request)

	suite.Equal(http.StatusNoContent, recorder.Code)

	suite.Run("runner was returned to runner manager", func() {
		suite.runnerManager.AssertCalled(suite.T(), "Return", suite.runner)
	})
}

func (suite *DeleteRunnerRouteTestSuite) TestReturnInternalServerErrorWhenApiCallToNomadFailed() {
	suite.runnerManager.On("Return", suite.runner).Return(errors.New("API call failed"))

	recorder := httptest.NewRecorder()
	request, err := http.NewRequest(http.MethodDelete, suite.path, nil)
	if err != nil {
		suite.T().Fatal(err)
	}

	suite.router.ServeHTTP(recorder, request)

	suite.Equal(http.StatusInternalServerError, recorder.Code)
}

func (suite *DeleteRunnerRouteTestSuite) TestDeleteInvalidRunnerIdReturnsNotFound() {
	suite.runnerManager.On("Get", mock.AnythingOfType("string")).Return(nil, errors.New("API call failed"))
	deleteURL, err := suite.router.Get(DeleteRoute).URL(RunnerIdKey, "1nv4l1dID")
	if err != nil {
		suite.T().Fatal(err)
	}
	deletePath := deleteURL.String()

	recorder := httptest.NewRecorder()
	request, err := http.NewRequest(http.MethodDelete, deletePath, nil)
	if err != nil {
		suite.T().Fatal(err)
	}

	suite.router.ServeHTTP(recorder, request)

	suite.Equal(http.StatusNotFound, recorder.Code)
}
