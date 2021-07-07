package api

import (
	"bytes"
	"encoding/json"
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
	s.runner = runner.NewNomadJob(tests.DefaultRunnerID, nil, nil, nil)
	s.capturedRunner = nil
	s.runnerRequest = func(runnerId string) *http.Request {
		path, err := s.router.Get("test-runner-id").URL(RunnerIDKey, runnerId)
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
	s.router.HandleFunc(fmt.Sprintf("/test/{%s}", RunnerIDKey), runnerRouteHandler).Name("test-runner-id")
}

func TestMiddlewareTestSuite(t *testing.T) {
	suite.Run(t, new(MiddlewareTestSuite))
}

func (s *MiddlewareTestSuite) TestFindRunnerMiddlewareIfRunnerExists() {
	s.manager.On("Get", s.runner.ID()).Return(s.runner, nil)

	recorder := httptest.NewRecorder()
	s.router.ServeHTTP(recorder, s.runnerRequest(s.runner.ID()))

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
	executionID   runner.ExecutionID
}

func (s *RunnerRouteTestSuite) SetupTest() {
	s.runnerManager = &runner.ManagerMock{}
	s.router = NewRouter(s.runnerManager, nil)
	s.runner = runner.NewNomadJob("some-id", nil, nil, nil)
	s.executionID = "execution-id"
	s.runner.Add(s.executionID, &dto.ExecutionRequest{})
	s.runnerManager.On("Get", s.runner.ID()).Return(s.runner, nil)
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

	runnerRequest := dto.RunnerRequest{ExecutionEnvironmentID: tests.DefaultEnvironmentIDAsInteger}
	body, err := json.Marshal(runnerRequest)
	s.Require().NoError(err)
	s.defaultRequest, err = http.NewRequest(http.MethodPost, s.path, bytes.NewReader(body))
	s.Require().NoError(err)
}

func (s *ProvideRunnerTestSuite) TestValidRequestReturnsRunner() {
	s.runnerManager.On("Claim", mock.AnythingOfType("runner.EnvironmentID"),
		mock.AnythingOfType("int")).Return(s.runner, nil)
	recorder := httptest.NewRecorder()

	s.router.ServeHTTP(recorder, s.defaultRequest)
	s.Equal(http.StatusOK, recorder.Code)

	s.Run("response contains runnerId", func() {
		var runnerResponse dto.RunnerResponse
		err := json.NewDecoder(recorder.Result().Body).Decode(&runnerResponse)
		s.Require().NoError(err)
		_ = recorder.Result().Body.Close()
		s.Equal(s.runner.ID(), runnerResponse.ID)
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
		On("Claim", mock.AnythingOfType("runner.EnvironmentID"), mock.AnythingOfType("int")).
		Return(nil, runner.ErrUnknownExecutionEnvironment)
	recorder := httptest.NewRecorder()

	s.router.ServeHTTP(recorder, s.defaultRequest)
	s.Equal(http.StatusNotFound, recorder.Code)
}

func (s *ProvideRunnerTestSuite) TestWhenNoRunnerAvailableReturnsNomadOverload() {
	s.runnerManager.On("Claim", mock.AnythingOfType("runner.EnvironmentID"), mock.AnythingOfType("int")).
		Return(nil, runner.ErrNoRunnersAvailable)
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
	path, err := s.router.Get(ExecutePath).URL(RunnerIDKey, s.runner.ID())
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
			webSocketURL, err := url.Parse(webSocketResponse.WebSocketURL)
			s.Require().NoError(err)
			executionID := webSocketURL.Query().Get(ExecutionIDKey)
			storedExecutionRequest, ok := s.runner.Pop(runner.ExecutionID(executionID))

			s.True(ok, "No execution request with this id: ", executionID)
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
	routeURL, err := s.router.Get(UpdateFileSystemPath).URL(RunnerIDKey, tests.DefaultMockID)
	s.Require().NoError(err)
	s.path = routeURL.String()
	s.runnerMock = &runner.RunnerMock{}
	s.runnerManager.On("Get", tests.DefaultMockID).Return(s.runnerMock, nil)
	s.recorder = httptest.NewRecorder()
}

func (s *UpdateFileSystemRouteTestSuite) TestUpdateFileSystemReturnsNoContentOnValidRequest() {
	s.runnerMock.On("UpdateFileSystem", mock.AnythingOfType("*dto.UpdateFileSystemRequest")).Return(nil)

	copyRequest := dto.UpdateFileSystemRequest{}
	body, err := json.Marshal(copyRequest)
	s.Require().NoError(err)
	request, err := http.NewRequest(http.MethodPatch, s.path, bytes.NewReader(body))
	s.Require().NoError(err)

	s.router.ServeHTTP(s.recorder, request)
	s.Equal(http.StatusNoContent, s.recorder.Code)
	s.runnerMock.AssertCalled(s.T(), "UpdateFileSystem", mock.AnythingOfType("*dto.UpdateFileSystemRequest"))
}

func (s *UpdateFileSystemRouteTestSuite) TestUpdateFileSystemReturnsBadRequestOnInvalidRequestBody() {
	request, err := http.NewRequest(http.MethodPatch, s.path, strings.NewReader(""))
	s.Require().NoError(err)

	s.router.ServeHTTP(s.recorder, request)
	s.Equal(http.StatusBadRequest, s.recorder.Code)
}

func (s *UpdateFileSystemRouteTestSuite) TestUpdateFileSystemToNonExistingRunnerReturnsNotFound() {
	invalidID := "some-invalid-runner-id"
	s.runnerManager.On("Get", invalidID).Return(nil, runner.ErrRunnerNotFound)
	path, err := s.router.Get(UpdateFileSystemPath).URL(RunnerIDKey, invalidID)
	s.Require().NoError(err)
	copyRequest := dto.UpdateFileSystemRequest{}
	body, err := json.Marshal(copyRequest)
	s.Require().NoError(err)
	request, err := http.NewRequest(http.MethodPatch, path.String(), bytes.NewReader(body))
	s.Require().NoError(err)

	s.router.ServeHTTP(s.recorder, request)
	s.Equal(http.StatusNotFound, s.recorder.Code)
}

func (s *UpdateFileSystemRouteTestSuite) TestUpdateFileSystemReturnsInternalServerErrorWhenCopyFailed() {
	s.runnerMock.
		On("UpdateFileSystem", mock.AnythingOfType("*dto.UpdateFileSystemRequest")).
		Return(runner.ErrorFileCopyFailed)

	copyRequest := dto.UpdateFileSystemRequest{}
	body, err := json.Marshal(copyRequest)
	s.Require().NoError(err)
	request, err := http.NewRequest(http.MethodPatch, s.path, bytes.NewReader(body))
	s.Require().NoError(err)

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
	deleteURL, err := s.router.Get(DeleteRoute).URL(RunnerIDKey, s.runner.ID())
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
	s.runnerManager.On("Return", s.runner).Return(tests.ErrDefault)

	recorder := httptest.NewRecorder()
	request, err := http.NewRequest(http.MethodDelete, s.path, nil)
	s.Require().NoError(err)

	s.router.ServeHTTP(recorder, request)

	s.Equal(http.StatusInternalServerError, recorder.Code)
}

func (s *DeleteRunnerRouteTestSuite) TestDeleteInvalidRunnerIdReturnsNotFound() {
	s.runnerManager.On("Get", mock.AnythingOfType("string")).Return(nil, tests.ErrDefault)
	deleteURL, err := s.router.Get(DeleteRoute).URL(RunnerIDKey, "1nv4l1dID")
	s.Require().NoError(err)
	deletePath := deleteURL.String()

	recorder := httptest.NewRecorder()
	request, err := http.NewRequest(http.MethodDelete, deletePath, nil)
	s.Require().NoError(err)

	s.router.ServeHTTP(recorder, request)

	s.Equal(http.StatusNotFound, recorder.Code)
}
