package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/gorilla/mux"
	"github.com/openHPI/poseidon/internal/nomad"
	"github.com/openHPI/poseidon/internal/runner"
	"github.com/openHPI/poseidon/pkg/dto"
	"github.com/openHPI/poseidon/pkg/monitoring"
	"github.com/openHPI/poseidon/tests"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
)

const invalidID = "some-invalid-runner-id"

type MiddlewareTestSuite struct {
	tests.MemoryLeakTestSuite
	manager        *runner.ManagerMock
	router         *mux.Router
	runner         runner.Runner
	capturedRunner runner.Runner
	runnerRequest  func(string) *http.Request
}

func (s *MiddlewareTestSuite) SetupTest() {
	s.MemoryLeakTestSuite.SetupTest()
	s.manager = &runner.ManagerMock{}
	apiMock := &nomad.ExecutorAPIMock{}
	apiMock.On("DeleteJob", mock.AnythingOfType("string")).Return(nil)
	s.runner = runner.NewNomadJob(s.TestCtx, tests.DefaultRunnerID, nil, apiMock, nil)
	s.capturedRunner = nil
	s.runnerRequest = func(runnerId string) *http.Request {
		path, err := s.router.Get("test-runner-id").URL(RunnerIDKey, runnerId)
		s.Require().NoError(err)
		request, err := http.NewRequest(http.MethodPost, path.String(), http.NoBody)
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
	s.router.Use(monitoring.InfluxDB2Middleware)
	s.router.Use(runnerController.findRunnerMiddleware)
	s.router.HandleFunc(fmt.Sprintf("/test/{%s}", RunnerIDKey), runnerRouteHandler).Name("test-runner-id")
}

func (s *MiddlewareTestSuite) TearDownTest() {
	defer s.MemoryLeakTestSuite.TearDownTest()
	err := s.runner.Destroy(nil)
	s.Require().NoError(err)
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
	s.manager.On("Get", invalidID).Return(nil, runner.ErrRunnerNotFound)

	recorder := httptest.NewRecorder()
	s.router.ServeHTTP(recorder, s.runnerRequest(invalidID))

	s.Equal(http.StatusGone, recorder.Code)
}

func (s *MiddlewareTestSuite) TestFindRunnerMiddlewareDoesNotEarlyRespond() {
	body := strings.NewReader(strings.Repeat("A", 798968))

	path, err := s.router.Get("test-runner-id").URL(RunnerIDKey, invalidID)
	s.Require().NoError(err)
	request, err := http.NewRequest(http.MethodPost, path.String(), body)
	s.Require().NoError(err)

	s.manager.On("Get", mock.AnythingOfType("string")).Return(nil, runner.ErrRunnerNotFound)
	recorder := httptest.NewRecorder()
	s.router.ServeHTTP(recorder, request)

	s.Equal(http.StatusGone, recorder.Code)
	s.Equal(0, body.Len()) // No data should be unread
}

func TestRunnerRouteTestSuite(t *testing.T) {
	suite.Run(t, new(RunnerRouteTestSuite))
}

type RunnerRouteTestSuite struct {
	tests.MemoryLeakTestSuite
	runnerManager *runner.ManagerMock
	router        *mux.Router
	runner        runner.Runner
	executionID   string
}

func (s *RunnerRouteTestSuite) SetupTest() {
	s.MemoryLeakTestSuite.SetupTest()
	s.runnerManager = &runner.ManagerMock{}
	s.router = NewRouter(s.runnerManager, nil)
	apiMock := &nomad.ExecutorAPIMock{}
	apiMock.On("DeleteJob", mock.AnythingOfType("string")).Return(nil)
	s.runner = runner.NewNomadJob(s.TestCtx, "some-id", nil, apiMock, func(_ runner.Runner) error { return nil })
	s.executionID = "execution"
	s.runner.StoreExecution(s.executionID, &dto.ExecutionRequest{})
	s.runnerManager.On("Get", s.runner.ID()).Return(s.runner, nil)
}

func (s *RunnerRouteTestSuite) TearDownTest() {
	defer s.MemoryLeakTestSuite.TearDownTest()
	s.Require().NoError(s.runner.Destroy(nil))
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
	s.runnerManager.
		On("Claim", mock.AnythingOfType("dto.EnvironmentID"), mock.AnythingOfType("int")).
		Return(s.runner, nil)
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
		On("Claim", mock.AnythingOfType("dto.EnvironmentID"), mock.AnythingOfType("int")).
		Return(nil, runner.ErrUnknownExecutionEnvironment)
	recorder := httptest.NewRecorder()

	s.router.ServeHTTP(recorder, s.defaultRequest)
	s.Equal(http.StatusNotFound, recorder.Code)
}

func (s *ProvideRunnerTestSuite) TestWhenNoRunnerAvailableReturnsNomadOverload() {
	s.runnerManager.
		On("Claim", mock.AnythingOfType("dto.EnvironmentID"), mock.AnythingOfType("int")).
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
			ok := s.runner.ExecutionExists(executionID)

			s.True(ok, "No execution request with this id: ", executionID)
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

	s.Run("forbidden characters in command", func() {
		recorder := httptest.NewRecorder()
		executionRequest := dto.ExecutionRequest{
			Command:   "echo 'forbidden'",
			TimeLimit: 10,
		}
		body, err := json.Marshal(executionRequest)
		s.Require().NoError(err)
		request, err := http.NewRequest(http.MethodPost, path.String(), bytes.NewReader(body))
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
	s.runnerMock.On("ID").Return(tests.DefaultMockID)
	s.runnerMock.On("Environment").Return(dto.EnvironmentID(tests.DefaultEnvironmentIDAsInteger))
	s.runnerManager.On("Get", tests.DefaultMockID).Return(s.runnerMock, nil)
	s.recorder = httptest.NewRecorder()
}

func (s *UpdateFileSystemRouteTestSuite) TestUpdateFileSystemReturnsNoContentOnValidRequest() {
	s.runnerMock.On("UpdateFileSystem", mock.Anything, mock.AnythingOfType("*dto.UpdateFileSystemRequest")).
		Return(nil)

	copyRequest := dto.UpdateFileSystemRequest{}
	body, err := json.Marshal(copyRequest)
	s.Require().NoError(err)
	request, err := http.NewRequest(http.MethodPatch, s.path, bytes.NewReader(body))
	s.Require().NoError(err)

	s.router.ServeHTTP(s.recorder, request)
	s.Equal(http.StatusNoContent, s.recorder.Code)
	s.runnerMock.AssertCalled(s.T(), "UpdateFileSystem", mock.Anything,
		mock.AnythingOfType("*dto.UpdateFileSystemRequest"))
}

func (s *UpdateFileSystemRouteTestSuite) TestUpdateFileSystemReturnsBadRequestOnInvalidRequestBody() {
	request, err := http.NewRequest(http.MethodPatch, s.path, strings.NewReader(""))
	s.Require().NoError(err)

	s.router.ServeHTTP(s.recorder, request)
	s.Equal(http.StatusBadRequest, s.recorder.Code)
}

func (s *UpdateFileSystemRouteTestSuite) TestUpdateFileSystemToNonExistingRunnerReturnsGone() {
	s.runnerManager.On("Get", invalidID).Return(nil, runner.ErrRunnerNotFound)
	path, err := s.router.Get(UpdateFileSystemPath).URL(RunnerIDKey, invalidID)
	s.Require().NoError(err)
	copyRequest := dto.UpdateFileSystemRequest{}
	body, err := json.Marshal(copyRequest)
	s.Require().NoError(err)
	request, err := http.NewRequest(http.MethodPatch, path.String(), bytes.NewReader(body))
	s.Require().NoError(err)

	s.router.ServeHTTP(s.recorder, request)
	s.Equal(http.StatusGone, s.recorder.Code)
}

func (s *UpdateFileSystemRouteTestSuite) TestUpdateFileSystemReturnsInternalServerErrorWhenCopyFailed() {
	s.runnerMock.
		On("UpdateFileSystem", mock.Anything, mock.AnythingOfType("*dto.UpdateFileSystemRequest")).
		Return(runner.ErrFileCopyFailed)

	copyRequest := dto.UpdateFileSystemRequest{}
	body, err := json.Marshal(copyRequest)
	s.Require().NoError(err)
	request, err := http.NewRequest(http.MethodPatch, s.path, bytes.NewReader(body))
	s.Require().NoError(err)

	s.router.ServeHTTP(s.recorder, request)
	s.Equal(http.StatusInternalServerError, s.recorder.Code)
}

func (s *UpdateFileSystemRouteTestSuite) TestListFileSystem() {
	routeURL, err := s.router.Get(UpdateFileSystemPath).URL(RunnerIDKey, tests.DefaultMockID)
	s.Require().NoError(err)
	mockCall := s.runnerMock.On("ListFileSystem", mock.Anything, mock.AnythingOfType("string"),
		mock.AnythingOfType("bool"), mock.Anything, mock.AnythingOfType("bool"))

	s.Run("default parameters", func() {
		mockCall.Run(func(args mock.Arguments) {
			path, ok := args.Get(1).(string)
			s.True(ok)
			s.Equal("./", path)
			recursive, ok := args.Get(2).(bool)
			s.True(ok)
			s.True(recursive)
			mockCall.ReturnArguments = mock.Arguments{nil}
		})
		request, err := http.NewRequest(http.MethodGet, routeURL.String(), strings.NewReader(""))
		s.Require().NoError(err)
		s.router.ServeHTTP(s.recorder, request)
		s.Equal(http.StatusOK, s.recorder.Code)
	})

	s.recorder = httptest.NewRecorder()
	s.Run("passed parameters", func() {
		expectedPath := "/flag"

		mockCall.Run(func(args mock.Arguments) {
			path, ok := args.Get(1).(string)
			s.True(ok)
			s.Equal(expectedPath, path)
			recursive, ok := args.Get(2).(bool)
			s.True(ok)
			s.False(recursive)
			mockCall.ReturnArguments = mock.Arguments{nil}
		})

		query := routeURL.Query()
		query.Set(PathKey, expectedPath)
		query.Set(RecursiveKey, strconv.FormatBool(false))
		routeURL.RawQuery = query.Encode()

		request, err := http.NewRequest(http.MethodGet, routeURL.String(), strings.NewReader(""))
		s.Require().NoError(err)
		s.router.ServeHTTP(s.recorder, request)
		s.Equal(http.StatusOK, s.recorder.Code)
	})

	s.recorder = httptest.NewRecorder()
	s.Run("Internal Server Error on failure", func() {
		mockCall.Run(func(_ mock.Arguments) {
			mockCall.ReturnArguments = mock.Arguments{runner.ErrRunnerNotFound}
		})

		request, err := http.NewRequest(http.MethodGet, routeURL.String(), strings.NewReader(""))
		s.Require().NoError(err)
		s.router.ServeHTTP(s.recorder, request)
		s.Equal(http.StatusInternalServerError, s.recorder.Code)
	})
}

func (s *UpdateFileSystemRouteTestSuite) TestFileContent() {
	routeURL, err := s.router.Get(FileContentRawPath).URL(RunnerIDKey, tests.DefaultMockID)
	s.Require().NoError(err)
	mockCall := s.runnerMock.On("GetFileContent",
		mock.Anything, mock.AnythingOfType("string"), mock.Anything, mock.AnythingOfType("bool"))

	s.Run("Not Found", func() {
		mockCall.Return(runner.ErrFileNotFound)
		request, err := http.NewRequest(http.MethodGet, routeURL.String(), strings.NewReader(""))
		s.Require().NoError(err)
		s.router.ServeHTTP(s.recorder, request)
		s.Equal(http.StatusFailedDependency, s.recorder.Code)
	})

	s.recorder = httptest.NewRecorder()
	s.Run("Unknown Error", func() {
		mockCall.Return(nomad.ErrExecutorCommunicationFailed)
		request, err := http.NewRequest(http.MethodGet, routeURL.String(), strings.NewReader(""))
		s.Require().NoError(err)
		s.router.ServeHTTP(s.recorder, request)
		s.Equal(http.StatusInternalServerError, s.recorder.Code)
	})

	s.recorder = httptest.NewRecorder()
	s.Run("No Error", func() {
		mockCall.Return(nil)
		request, err := http.NewRequest(http.MethodGet, routeURL.String(), strings.NewReader(""))
		s.Require().NoError(err)
		s.router.ServeHTTP(s.recorder, request)
		s.Equal(http.StatusOK, s.recorder.Code)
	})
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
	request, err := http.NewRequest(http.MethodDelete, s.path, http.NoBody)
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
	request, err := http.NewRequest(http.MethodDelete, s.path, http.NoBody)
	s.Require().NoError(err)

	s.router.ServeHTTP(recorder, request)

	s.Equal(http.StatusInternalServerError, recorder.Code)
}

func (s *DeleteRunnerRouteTestSuite) TestDeleteInvalidRunnerIdReturnsGone() {
	s.runnerManager.On("Get", mock.AnythingOfType("string")).Return(nil, tests.ErrDefault)
	deleteURL, err := s.router.Get(DeleteRoute).URL(RunnerIDKey, "1nv4l1dID")
	s.Require().NoError(err)
	deletePath := deleteURL.String()

	recorder := httptest.NewRecorder()
	request, err := http.NewRequest(http.MethodDelete, deletePath, http.NoBody)
	s.Require().NoError(err)

	s.router.ServeHTTP(recorder, request)

	s.Equal(http.StatusGone, recorder.Code)
}
