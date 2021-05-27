package api

import (
	"bytes"
	"encoding/json"
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/api/dto"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/environment"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/tests"
	"net/http"
	"net/http/httptest"
	"testing"
)

type EnvironmentControllerTestSuite struct {
	suite.Suite
	manager *environment.ManagerMock
	router  *mux.Router
}

func TestEnvironmentControllerTestSuite(t *testing.T) {
	suite.Run(t, new(EnvironmentControllerTestSuite))
}

func (s *EnvironmentControllerTestSuite) SetupTest() {
	s.manager = &environment.ManagerMock{}
	s.router = NewRouter(nil, s.manager)
}

type CreateOrUpdateEnvironmentTestSuite struct {
	EnvironmentControllerTestSuite
	path string
	id   string
	body []byte
}

func TestCreateOrUpdateEnvironmentTestSuite(t *testing.T) {
	suite.Run(t, new(CreateOrUpdateEnvironmentTestSuite))
}

func (s *CreateOrUpdateEnvironmentTestSuite) SetupTest() {
	s.EnvironmentControllerTestSuite.SetupTest()
	s.id = tests.DefaultEnvironmentIdAsString
	testURL, err := s.router.Get(createOrUpdateRouteName).URL(executionEnvironmentIDKey, s.id)
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
	testError := tests.DefaultError
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