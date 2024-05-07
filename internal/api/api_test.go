package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"
	"github.com/openHPI/poseidon/internal/config"
	"github.com/openHPI/poseidon/internal/environment"
	"github.com/openHPI/poseidon/pkg/dto"
	"github.com/openHPI/poseidon/tests"
	"github.com/stretchr/testify/suite"
)

func mockHTTPHandler(writer http.ResponseWriter, _ *http.Request) {
	writer.WriteHeader(http.StatusOK)
}

type MainTestSuite struct {
	tests.MemoryLeakTestSuite
}

func TestMainTestSuite(t *testing.T) {
	suite.Run(t, new(MainTestSuite))
}

func (s *MainTestSuite) TestNewRouterV1WithAuthenticationDisabled() {
	config.Config.Server.Token = ""
	router := mux.NewRouter()
	m := &environment.ManagerHandlerMock{}
	m.On("Statistics").Return(make(map[dto.EnvironmentID]*dto.StatisticalExecutionEnvironmentData))
	configureV1Router(router, nil, m)

	s.Run("health route is accessible", func() {
		request, err := http.NewRequest(http.MethodGet, "/api/v1/health", http.NoBody)
		if err != nil {
			s.T().Fatal(err)
		}
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, request)
		s.Equal(http.StatusNoContent, recorder.Code)
	})

	s.Run("added route is accessible", func() {
		router.HandleFunc("/api/v1/test", mockHTTPHandler)
		request, err := http.NewRequest(http.MethodGet, "/api/v1/test", http.NoBody)
		if err != nil {
			s.T().Fatal(err)
		}
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, request)
		s.Equal(http.StatusOK, recorder.Code)
	})
}

func (s *MainTestSuite) TestNewRouterV1WithAuthenticationEnabled() {
	config.Config.Server.Token = "TestToken"
	router := mux.NewRouter()
	m := &environment.ManagerHandlerMock{}
	m.On("Statistics").Return(make(map[dto.EnvironmentID]*dto.StatisticalExecutionEnvironmentData))
	configureV1Router(router, nil, m)

	s.Run("health route is accessible", func() {
		request, err := http.NewRequest(http.MethodGet, "/api/v1/health", http.NoBody)
		if err != nil {
			s.T().Fatal(err)
		}
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, request)
		s.Equal(http.StatusNoContent, recorder.Code)
	})

	s.Run("protected route is not accessible", func() {
		// request an available API route that should be guarded by authentication.
		// (which one, in particular, does not matter here)
		request, err := http.NewRequest(http.MethodPost, "/api/v1/runners", http.NoBody)
		if err != nil {
			s.T().Fatal(err)
		}
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, request)
		s.Equal(http.StatusUnauthorized, recorder.Code)
	})
	config.Config.Server.Token = ""
}
