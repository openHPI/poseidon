package api

import (
	"encoding/json"
	"github.com/stretchr/testify/assert"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHealthRoute(t *testing.T) {
	req, err := http.NewRequest("GET", "/health", nil)
	if err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	http.HandlerFunc(Health).ServeHTTP(rec, req)
	res := &Message{}
	_ = json.NewDecoder(rec.Body).Decode(res)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "I'm alive!", res.Msg)
}
