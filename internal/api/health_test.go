package api

import (
	"github.com/stretchr/testify/assert"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHealthRoute(t *testing.T) {
	request, err := http.NewRequest(http.MethodGet, "/health", http.NoBody)
	if err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	http.HandlerFunc(Health).ServeHTTP(recorder, request)
	assert.Equal(t, http.StatusNoContent, recorder.Code)
}
