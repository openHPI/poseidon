package api

import (
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/config"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/environment"
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
	v1 := newRouterV1(router, nil, environment.NewLocalRunnerPool())

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
		v1.HandleFunc("/test", mockHTTPHandler)
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
	v1 := newRouterV1(router, nil, environment.NewLocalRunnerPool())

	t.Run("health route is accessible", func(t *testing.T) {
		request, err := http.NewRequest(http.MethodGet, "/api/v1/health", nil)
		if err != nil {
			t.Fatal(err)
		}
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, request)
		assert.Equal(t, http.StatusNoContent, recorder.Code)
	})

	t.Run("added route is not accessible", func(t *testing.T) {
		v1.HandleFunc("/test", mockHTTPHandler)
		request, err := http.NewRequest(http.MethodGet, "/api/v1/test", nil)
		if err != nil {
			t.Fatal(err)
		}
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, request)
		assert.Equal(t, http.StatusUnauthorized, recorder.Code)
	})
	config.Config.Server.Token = ""
}
