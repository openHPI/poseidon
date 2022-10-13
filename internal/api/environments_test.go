package api

import (
	"bytes"
	"encoding/json"
	"github.com/gorilla/mux"
	"github.com/openHPI/poseidon/internal/environment"
	"github.com/openHPI/poseidon/internal/nomad"
	"github.com/openHPI/poseidon/internal/runner"
	"github.com/openHPI/poseidon/pkg/dto"
	"github.com/openHPI/poseidon/tests"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
	"math"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

type EnvironmentControllerTestSuite struct {
	suite.Suite
	manager *environment.ManagerHandlerMock
	router  *mux.Router
}

func TestEnvironmentControllerTestSuite(t *testing.T) {
	suite.Run(t, new(EnvironmentControllerTestSuite))
}

func (s *EnvironmentControllerTestSuite) SetupTest() {
	s.manager = &environment.ManagerHandlerMock{}
	s.router = NewRouter(nil, s.manager)
}

func (s *EnvironmentControllerTestSuite) TestList() {
	call := s.manager.On("List", mock.AnythingOfType("bool"))
	call.Run(func(args mock.Arguments) {
		call.ReturnArguments = mock.Arguments{[]runner.ExecutionEnvironment{}, nil}
	})
	path, err := s.router.Get(listRouteName).URL()
	s.Require().NoError(err)
	request, err := http.NewRequest(http.MethodGet, path.String(), http.NoBody)
	s.Require().NoError(err)

	s.Run("with no Environments", func() {
		recorder := httptest.NewRecorder()
		s.router.ServeHTTP(recorder, request)
		s.Equal(http.StatusOK, recorder.Code)

		var environmentsResponse ExecutionEnvironmentsResponse
		err = json.NewDecoder(recorder.Result().Body).Decode(&environmentsResponse)
		s.Require().NoError(err)
		_ = recorder.Result().Body.Close()

		s.Empty(environmentsResponse.ExecutionEnvironments)
	})
	s.manager.Calls = []mock.Call{}

	s.Run("with fetch", func() {
		recorder := httptest.NewRecorder()
		query := path.Query()
		query.Set("fetch", "true")
		path.RawQuery = query.Encode()
		request, err := http.NewRequest(http.MethodGet, path.String(), http.NoBody)
		s.Require().NoError(err)

		s.router.ServeHTTP(recorder, request)
		s.Equal(http.StatusOK, recorder.Code)
		s.manager.AssertCalled(s.T(), "List", true)
	})
	s.manager.Calls = []mock.Call{}

	s.Run("with bad fetch", func() {
		recorder := httptest.NewRecorder()
		query := path.Query()
		query.Set("fetch", "YouDecide")
		path.RawQuery = query.Encode()
		request, err := http.NewRequest(http.MethodGet, path.String(), http.NoBody)
		s.Require().NoError(err)

		s.router.ServeHTTP(recorder, request)
		s.Equal(http.StatusBadRequest, recorder.Code)
		s.manager.AssertNotCalled(s.T(), "List")
	})

	s.Run("returns multiple environments", func() {
		call.Run(func(args mock.Arguments) {
			firstEnvironment, err := environment.NewNomadEnvironment(tests.DefaultEnvironmentIDAsInteger, nil,
				"job \""+nomad.TemplateJobID(tests.DefaultEnvironmentIDAsInteger)+"\" {}")
			s.Require().NoError(err)
			secondEnvironment, err := environment.NewNomadEnvironment(tests.DefaultEnvironmentIDAsInteger, nil,
				"job \""+nomad.TemplateJobID(tests.AnotherEnvironmentIDAsInteger)+"\" {}")
			s.Require().NoError(err)
			call.ReturnArguments = mock.Arguments{[]runner.ExecutionEnvironment{firstEnvironment, secondEnvironment}, nil}
		})
		recorder := httptest.NewRecorder()
		s.router.ServeHTTP(recorder, request)
		s.Equal(http.StatusOK, recorder.Code)

		paramMap := make(map[string]interface{})
		err := json.NewDecoder(recorder.Result().Body).Decode(&paramMap)
		s.Require().NoError(err)
		environmentsInterface, ok := paramMap["executionEnvironments"]
		s.Require().True(ok)
		environments, ok := environmentsInterface.([]interface{})
		s.Require().True(ok)
		s.Equal(2, len(environments))
	})
}

func (s *EnvironmentControllerTestSuite) TestGet() {
	call := s.manager.On("Get", mock.AnythingOfType("dto.EnvironmentID"), mock.AnythingOfType("bool"))
	path, err := s.router.Get(getRouteName).URL(executionEnvironmentIDKey, tests.DefaultEnvironmentIDAsString)
	s.Require().NoError(err)
	request, err := http.NewRequest(http.MethodGet, path.String(), http.NoBody)
	s.Require().NoError(err)

	s.Run("with unknown environment", func() {
		call.Run(func(args mock.Arguments) {
			call.ReturnArguments = mock.Arguments{nil, runner.ErrUnknownExecutionEnvironment}
		})

		recorder := httptest.NewRecorder()
		s.router.ServeHTTP(recorder, request)
		s.Equal(http.StatusNotFound, recorder.Code)
		s.manager.AssertCalled(s.T(), "Get", dto.EnvironmentID(0), false)
	})
	s.manager.Calls = []mock.Call{}

	s.Run("not found with fetch", func() {
		recorder := httptest.NewRecorder()
		query := path.Query()
		query.Set("fetch", "true")
		path.RawQuery = query.Encode()
		request, err := http.NewRequest(http.MethodGet, path.String(), http.NoBody)
		s.Require().NoError(err)

		call.Run(func(args mock.Arguments) {
			call.ReturnArguments = mock.Arguments{nil, runner.ErrUnknownExecutionEnvironment}
		})

		s.router.ServeHTTP(recorder, request)
		s.Equal(http.StatusNotFound, recorder.Code)
		s.manager.AssertCalled(s.T(), "Get", dto.EnvironmentID(0), true)
	})
	s.manager.Calls = []mock.Call{}

	s.Run("returns environment", func() {
		call.Run(func(args mock.Arguments) {
			testEnvironment, err := environment.NewNomadEnvironment(tests.DefaultEnvironmentIDAsInteger, nil,
				"job \""+nomad.TemplateJobID(tests.DefaultEnvironmentIDAsInteger)+"\" {}")
			s.Require().NoError(err)
			call.ReturnArguments = mock.Arguments{testEnvironment, nil}
		})

		recorder := httptest.NewRecorder()
		s.router.ServeHTTP(recorder, request)
		s.Equal(http.StatusOK, recorder.Code)

		var environmentParams map[string]interface{}
		err := json.NewDecoder(recorder.Result().Body).Decode(&environmentParams)
		s.Require().NoError(err)
		idInterface, ok := environmentParams["id"]
		s.Require().True(ok)
		idFloat, ok := idInterface.(float64)
		s.Require().True(ok)
		s.Equal(tests.DefaultEnvironmentIDAsInteger, int(idFloat))
	})
}

func (s *EnvironmentControllerTestSuite) TestDelete() {
	call := s.manager.On("Delete", mock.AnythingOfType("dto.EnvironmentID"))
	path, err := s.router.Get(deleteRouteName).URL(executionEnvironmentIDKey, tests.DefaultEnvironmentIDAsString)
	s.Require().NoError(err)
	request, err := http.NewRequest(http.MethodDelete, path.String(), http.NoBody)
	s.Require().NoError(err)

	s.Run("environment not found", func() {
		call.Run(func(args mock.Arguments) {
			call.ReturnArguments = mock.Arguments{false, nil}
		})
		recorder := httptest.NewRecorder()
		s.router.ServeHTTP(recorder, request)
		s.Equal(http.StatusNotFound, recorder.Code)
	})

	s.Run("environment deleted", func() {
		call.Run(func(args mock.Arguments) {
			call.ReturnArguments = mock.Arguments{true, nil}
		})
		recorder := httptest.NewRecorder()
		s.router.ServeHTTP(recorder, request)
		s.Equal(http.StatusNoContent, recorder.Code)
	})

	s.manager.Calls = []mock.Call{}
	s.Run("with bad environment id", func() {
		_, err := s.router.Get(deleteRouteName).URL(executionEnvironmentIDKey, "MagicNonNumberID")
		s.Error(err)
	})
}

type CreateOrUpdateEnvironmentTestSuite struct {
	EnvironmentControllerTestSuite
	path string
	id   dto.EnvironmentID
	body []byte
}

func TestCreateOrUpdateEnvironmentTestSuite(t *testing.T) {
	suite.Run(t, new(CreateOrUpdateEnvironmentTestSuite))
}

func (s *CreateOrUpdateEnvironmentTestSuite) SetupTest() {
	s.EnvironmentControllerTestSuite.SetupTest()
	s.id = tests.DefaultEnvironmentIDAsInteger
	testURL, err := s.router.Get(createOrUpdateRouteName).URL(executionEnvironmentIDKey, strconv.Itoa(int(s.id)))
	if err != nil {
		s.T().Fatal(err)
	}
	s.path = testURL.String()
	s.body, err = json.Marshal(dto.ExecutionEnvironmentRequest{})
	if err != nil {
		s.T().Fatal(err)
	}
}

func (s *CreateOrUpdateEnvironmentTestSuite) recordRequest() *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest(http.MethodPut, s.path, bytes.NewReader(s.body))
	if err != nil {
		s.T().Fatal(err)
	}
	s.router.ServeHTTP(recorder, request)
	return recorder
}

func (s *CreateOrUpdateEnvironmentTestSuite) TestReturnsBadRequestWhenBadBody() {
	s.body = []byte{}
	recorder := s.recordRequest()
	s.Equal(http.StatusBadRequest, recorder.Code)
}

func (s *CreateOrUpdateEnvironmentTestSuite) TestReturnsInternalServerErrorWhenManagerReturnsError() {
	testError := tests.ErrDefault
	s.manager.
		On("CreateOrUpdate", s.id, mock.AnythingOfType("dto.ExecutionEnvironmentRequest")).
		Return(false, testError)

	recorder := s.recordRequest()
	s.Equal(http.StatusInternalServerError, recorder.Code)
	s.Contains(recorder.Body.String(), testError.Error())
}

func (s *CreateOrUpdateEnvironmentTestSuite) TestReturnsCreatedIfNewEnvironment() {
	s.manager.
		On("CreateOrUpdate", s.id, mock.AnythingOfType("dto.ExecutionEnvironmentRequest")).
		Return(true, nil)

	recorder := s.recordRequest()
	s.Equal(http.StatusCreated, recorder.Code)
}

func (s *CreateOrUpdateEnvironmentTestSuite) TestReturnsNoContentIfNotNewEnvironment() {
	s.manager.
		On("CreateOrUpdate", s.id, mock.AnythingOfType("dto.ExecutionEnvironmentRequest")).
		Return(false, nil)

	recorder := s.recordRequest()
	s.Equal(http.StatusNoContent, recorder.Code)
}

func (s *CreateOrUpdateEnvironmentTestSuite) TestReturnsNotFoundOnNonIntegerID() {
	s.path = strings.Join([]string{BasePath, EnvironmentsPath, "/", "invalid-id"}, "")
	recorder := s.recordRequest()
	s.Equal(http.StatusNotFound, recorder.Code)
}

func (s *CreateOrUpdateEnvironmentTestSuite) TestFailsOnTooLargeID() {
	tooLargeIntStr := strconv.Itoa(math.MaxInt64) + "0"
	s.path = strings.Join([]string{BasePath, EnvironmentsPath, "/", tooLargeIntStr}, "")
	recorder := s.recordRequest()
	s.Equal(http.StatusBadRequest, recorder.Code)
}
