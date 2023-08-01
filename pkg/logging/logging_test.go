package logging

import (
	"github.com/openHPI/poseidon/pkg/dto"
	"github.com/sirupsen/logrus"
	"github.com/sirupsen/logrus/hooks/test"
	"github.com/stretchr/testify/assert"
	"net/http"
	"net/http/httptest"
	"testing"
)

func mockHTTPStatusHandler(status int) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(status)
	})
}

func TestHTTPMiddlewareWarnsWhenInternalServerError(t *testing.T) {
	var hook *test.Hook
	log, hook = test.NewNullLogger()
	InitializeLogging(logrus.DebugLevel.String(), dto.FormatterText)

	request, err := http.NewRequest(http.MethodGet, "/", http.NoBody)
	if err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	HTTPLoggingMiddleware(mockHTTPStatusHandler(500)).ServeHTTP(recorder, request)

	assert.Equal(t, 1, len(hook.Entries))
	assert.Equal(t, logrus.ErrorLevel, hook.LastEntry().Level)
}

func TestHTTPMiddlewareDebugsWhenStatusOK(t *testing.T) {
	var hook *test.Hook
	log, hook = test.NewNullLogger()
	InitializeLogging(logrus.DebugLevel.String(), dto.FormatterText)

	request, err := http.NewRequest(http.MethodGet, "/", http.NoBody)
	if err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	HTTPLoggingMiddleware(mockHTTPStatusHandler(200)).ServeHTTP(recorder, request)

	assert.Equal(t, 1, len(hook.Entries))
	assert.Equal(t, logrus.DebugLevel, hook.LastEntry().Level)
}
