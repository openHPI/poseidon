package api

import (
	"encoding/json"
	"github.com/stretchr/testify/assert"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHealthRoute(t *testing.T) {
	request, err := http.NewRequest(http.MethodGet, "/health", nil)
	if err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	http.HandlerFunc(Health).ServeHTTP(recorder, request)
	result := Message{}
	_ = json.Unmarshal(recorder.Body.Bytes(), &result)

	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Equal(t, "application/json", recorder.Header().Get("Content-Type"))
	assert.Equal(t, "I'm alive!", result.Message)
}
