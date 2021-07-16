package api

import (
	"bytes"
	"encoding/json"
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/internal/environment"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/internal/runner"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/pkg/dto"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/tests"
	"math"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
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
	id   runner.EnvironmentID
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
