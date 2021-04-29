package logging

import (
	"github.com/sirupsen/logrus"
	"github.com/sirupsen/logrus/hooks/test"
	"github.com/stretchr/testify/assert"
	"net/http"
	"net/http/httptest"
	"testing"
)

func mockHttpStatusHandler(status int) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(status)
	})
}

func TestHTTPMiddlewareWarnsWhenInternalServerError(t *testing.T) {
	var hook *test.Hook
	log, hook = test.NewNullLogger()
	InitializeLogging(logrus.DebugLevel.String())

	request, err := http.NewRequest(http.MethodGet, "/", nil)
	if err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	HTTPLoggingMiddleware(mockHttpStatusHandler(500)).ServeHTTP(recorder, request)

	assert.Equal(t, 1, len(hook.Entries))
	assert.Equal(t, logrus.WarnLevel, hook.LastEntry().Level)
}

func TestHTTPMiddlewareDebugsWhenStatusOK(t *testing.T) {
	var hook *test.Hook
	log, hook = test.NewNullLogger()
	InitializeLogging(logrus.DebugLevel.String())

	request, err := http.NewRequest(http.MethodGet, "/", nil)
	if err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	HTTPLoggingMiddleware(mockHttpStatusHandler(200)).ServeHTTP(recorder, request)

	assert.Equal(t, 1, len(hook.Entries))
	assert.Equal(t, logrus.DebugLevel, hook.LastEntry().Level)
}
