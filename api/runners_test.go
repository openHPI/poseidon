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

func (s *MiddlewareTestSuite) SetupTest() {
	s.manager = &runner.ManagerMock{}
	s.runner = runner.NewNomadJob(tests.DefaultRunnerID, nil)
	s.capturedRunner = nil
	s.runnerRequest = func(runnerId string) *http.Request {
		path, err := s.router.Get("test-runner-id").URL(RunnerIdKey, runnerId)
		s.Require().NoError(err)
		request, err := http.NewRequest(http.MethodPost, path.String(), nil)
		s.Require().NoError(err)
		return request
	}
	runnerRouteHandler := func(writer http.ResponseWriter, request *http.Request) {
		var ok bool
		s.capturedRunner, ok = runner.FromContext(request.Context())
		if ok {
			writer.WriteHeader(http.StatusOK)
		} else {
			writer.WriteHeader(http.StatusInternalServerError)
		}
	}
	s.router = mux.NewRouter()
	runnerController := &RunnerController{s.manager, s.router}
	s.router.Use(runnerController.findRunnerMiddleware)
	s.router.HandleFunc(fmt.Sprintf("/test/{%s}", RunnerIdKey), runnerRouteHandler).Name("test-runner-id")
}

func TestMiddlewareTestSuite(t *testing.T) {
	suite.Run(t, new(MiddlewareTestSuite))
}

func (s *MiddlewareTestSuite) TestFindRunnerMiddlewareIfRunnerExists() {
	s.manager.On("Get", s.runner.Id()).Return(s.runner, nil)

	recorder := httptest.NewRecorder()
	s.router.ServeHTTP(recorder, s.runnerRequest(s.runner.Id()))

	s.Equal(http.StatusOK, recorder.Code)
	s.Equal(s.runner, s.capturedRunner)
}

func (s *MiddlewareTestSuite) TestFindRunnerMiddlewareIfRunnerDoesNotExist() {
	invalidID := "some-invalid-runner-id"
	s.manager.On("Get", invalidID).Return(nil, runner.ErrRunnerNotFound)

	recorder := httptest.NewRecorder()
	s.router.ServeHTTP(recorder, s.runnerRequest(invalidID))

	s.Equal(http.StatusNotFound, recorder.Code)
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

func (s *RunnerRouteTestSuite) SetupTest() {
	s.runnerManager = &runner.ManagerMock{}
	s.router = NewRouter(s.runnerManager, nil)
	s.runner = runner.NewNomadJob("some-id", nil)
	s.executionId = "execution-id"
	s.runner.Add(s.executionId, &dto.ExecutionRequest{})
	s.runnerManager.On("Get", s.runner.Id()).Return(s.runner, nil)
}

func TestProvideRunnerTestSuite(t *testing.T) {
	suite.Run(t, new(ProvideRunnerTestSuite))
}

type ProvideRunnerTestSuite struct {
	RunnerRouteTestSuite
	defaultRequest *http.Request
	path           string
}

func (s *ProvideRunnerTestSuite) SetupTest() {
	s.RunnerRouteTestSuite.SetupTest()

	path, err := s.router.Get(ProvideRoute).URL()
	s.Require().NoError(err)
	s.path = path.String()

	runnerRequest := dto.RunnerRequest{ExecutionEnvironmentId: tests.DefaultEnvironmentIDAsInteger}
	body, err := json.Marshal(runnerRequest)
	s.Require().NoError(err)
	s.defaultRequest, err = http.NewRequest(http.MethodPost, s.path, bytes.NewReader(body))
	s.Require().NoError(err)
}

func (s *ProvideRunnerTestSuite) TestValidRequestReturnsRunner() {
	s.runnerManager.On("Claim", mock.AnythingOfType("runner.EnvironmentID")).Return(s.runner, nil)
	recorder := httptest.NewRecorder()

	s.router.ServeHTTP(recorder, s.defaultRequest)
	s.Equal(http.StatusOK, recorder.Code)

	s.Run("response contains runnerId", func() {
		var runnerResponse dto.RunnerResponse
		err := json.NewDecoder(recorder.Result().Body).Decode(&runnerResponse)
		s.Require().NoError(err)
		_ = recorder.Result().Body.Close()
		s.Equal(s.runner.Id(), runnerResponse.Id)
	})
}

func (s *ProvideRunnerTestSuite) TestInvalidRequestReturnsBadRequest() {
	badRequest, err := http.NewRequest(http.MethodPost, s.path, strings.NewReader(""))
	s.Require().NoError(err)
	recorder := httptest.NewRecorder()

	s.router.ServeHTTP(recorder, badRequest)
	s.Equal(http.StatusBadRequest, recorder.Code)
}

func (s *ProvideRunnerTestSuite) TestWhenExecutionEnvironmentDoesNotExistReturnsNotFound() {
	s.runnerManager.
		On("Claim", mock.AnythingOfType("runner.EnvironmentID")).
		Return(nil, runner.ErrUnknownExecutionEnvironment)
	recorder := httptest.NewRecorder()

	s.router.ServeHTTP(recorder, s.defaultRequest)
	s.Equal(http.StatusNotFound, recorder.Code)
}

func (s *ProvideRunnerTestSuite) TestWhenNoRunnerAvailableReturnsNomadOverload() {
	s.runnerManager.On("Claim", mock.AnythingOfType("runner.EnvironmentID")).Return(nil, runner.ErrNoRunnersAvailable)
	recorder := httptest.NewRecorder()

	s.router.ServeHTTP(recorder, s.defaultRequest)
	s.Equal(http.StatusInternalServerError, recorder.Code)

	var internalServerError dto.InternalServerError
	err := json.NewDecoder(recorder.Result().Body).Decode(&internalServerError)
	s.Require().NoError(err)
	_ = recorder.Result().Body.Close()
	s.Equal(dto.ErrorNomadOverload, internalServerError.ErrorCode)
}

func (s *RunnerRouteTestSuite) TestExecuteRoute() {
	path, err := s.router.Get(ExecutePath).URL(RunnerIdKey, s.runner.Id())
	s.Require().NoError(err)

	s.Run("valid request", func() {
		recorder := httptest.NewRecorder()
		executionRequest := dto.ExecutionRequest{
			Command:     "command",
			TimeLimit:   10,
			Environment: nil,
		}
		body, err := json.Marshal(executionRequest)
		s.Require().NoError(err)
		request, err := http.NewRequest(http.MethodPost, path.String(), bytes.NewReader(body))
		s.Require().NoError(err)

		s.router.ServeHTTP(recorder, request)

		var webSocketResponse dto.ExecutionResponse
		err = json.NewDecoder(recorder.Result().Body).Decode(&webSocketResponse)
		s.Require().NoError(err)

		s.Equal(http.StatusOK, recorder.Code)

		s.Run("creates an execution request for the runner", func() {
			webSocketUrl, err := url.Parse(webSocketResponse.WebSocketUrl)
			s.Require().NoError(err)
			executionId := webSocketUrl.Query().Get(ExecutionIdKey)
			storedExecutionRequest, ok := s.runner.Pop(runner.ExecutionId(executionId))

			s.True(ok, "No execution request with this id: ", executionId)
			s.Equal(&executionRequest, storedExecutionRequest)
		})
	})

	s.Run("invalid request", func() {
		recorder := httptest.NewRecorder()
		body := ""
		request, err := http.NewRequest(http.MethodPost, path.String(), strings.NewReader(body))
		s.Require().NoError(err)
		s.router.ServeHTTP(recorder, request)

		s.Equal(http.StatusBadRequest, recorder.Code)
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
	routeUrl, err := s.router.Get(UpdateFileSystemPath).URL(RunnerIdKey, tests.DefaultMockID)
	s.Require().NoError(err)
	s.path = routeUrl.String()
	s.runnerMock = &runner.RunnerMock{}
	s.runnerManager.On("Get", tests.DefaultMockID).Return(s.runnerMock, nil)
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

func (s *DeleteRunnerRouteTestSuite) SetupTest() {
	s.RunnerRouteTestSuite.SetupTest()
	deleteURL, err := s.router.Get(DeleteRoute).URL(RunnerIdKey, s.runner.Id())
	s.Require().NoError(err)
	s.path = deleteURL.String()
}

func (s *DeleteRunnerRouteTestSuite) TestValidRequestReturnsNoContent() {
	s.runnerManager.On("Return", s.runner).Return(nil)

	recorder := httptest.NewRecorder()
	request, err := http.NewRequest(http.MethodDelete, s.path, nil)
	s.Require().NoError(err)

	s.router.ServeHTTP(recorder, request)

	s.Equal(http.StatusNoContent, recorder.Code)

	s.Run("runner was returned to runner manager", func() {
		s.runnerManager.AssertCalled(s.T(), "Return", s.runner)
	})
}

func (s *DeleteRunnerRouteTestSuite) TestReturnInternalServerErrorWhenApiCallToNomadFailed() {
	s.runnerManager.On("Return", s.runner).Return(errors.New("API call failed"))

	recorder := httptest.NewRecorder()
	request, err := http.NewRequest(http.MethodDelete, s.path, nil)
	s.Require().NoError(err)

	s.router.ServeHTTP(recorder, request)

	s.Equal(http.StatusInternalServerError, recorder.Code)
}

func (s *DeleteRunnerRouteTestSuite) TestDeleteInvalidRunnerIdReturnsNotFound() {
	s.runnerManager.On("Get", mock.AnythingOfType("string")).Return(nil, errors.New("API call failed"))
	deleteURL, err := s.router.Get(DeleteRoute).URL(RunnerIdKey, "1nv4l1dID")
	s.Require().NoError(err)
	deletePath := deleteURL.String()

	recorder := httptest.NewRecorder()
	request, err := http.NewRequest(http.MethodDelete, deletePath, nil)
	s.Require().NoError(err)

	s.router.ServeHTTP(recorder, request)

	s.Equal(http.StatusNotFound, recorder.Code)
}
