package api

import (
	"net/http"
	"net/http/httptest"
)

func (s *MainTestSuite) TestHealthRoute() {
	request, err := http.NewRequest(http.MethodGet, "/health", http.NoBody)
	if err != nil {
		s.T().Fatal(err)
	}
	recorder := httptest.NewRecorder()
	http.HandlerFunc(Health).ServeHTTP(recorder, request)
	s.Equal(http.StatusNoContent, recorder.Code)
}
