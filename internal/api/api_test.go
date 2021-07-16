package api

import (
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/internal/config"
	"net/http"
	"net/http/httptest"
	"testing"
)

func mockHTTPHandler(writer http.ResponseWriter, _ *http.Request) {
	writer.WriteHeader(http.StatusOK)
}

func TestNewRouterV1WithAuthenticationDisabled(t *testing.T) {
	config.Config.Server.Token = ""
	router := mux.NewRouter()
	configureV1Router(router, nil, nil)

	t.Run("health route is accessible", func(t *testing.T) {
		request, err := http.NewRequest(http.MethodGet, "/api/v1/health", nil)
		if err != nil {
			t.Fatal(err)
		}
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, request)
		assert.Equal(t, http.StatusNoContent, recorder.Code)
	})

	t.Run("added route is accessible", func(t *testing.T) {
		router.HandleFunc("/api/v1/test", mockHTTPHandler)
		request, err := http.NewRequest(http.MethodGet, "/api/v1/test", nil)
		if err != nil {
			t.Fatal(err)
		}
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, request)
		assert.Equal(t, http.StatusOK, recorder.Code)
	})
}

func TestNewRouterV1WithAuthenticationEnabled(t *testing.T) {
	config.Config.Server.Token = "TestToken"
	router := mux.NewRouter()
	configureV1Router(router, nil, nil)

	t.Run("health route is accessible", func(t *testing.T) {
		request, err := http.NewRequest(http.MethodGet, "/api/v1/health", nil)
		if err != nil {
			t.Fatal(err)
		}
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, request)
		assert.Equal(t, http.StatusNoContent, recorder.Code)
	})

	t.Run("protected route is not accessible", func(t *testing.T) {
		// request an available API route that should be guarded by authentication.
		// (which one, in particular, does not matter here)
		request, err := http.NewRequest(http.MethodPost, "/api/v1/runners", nil)
		if err != nil {
			t.Fatal(err)
		}
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, request)
		assert.Equal(t, http.StatusUnauthorized, recorder.Code)
	})
	config.Config.Server.Token = ""
}
