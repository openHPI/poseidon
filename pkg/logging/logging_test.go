package logging

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/openHPI/poseidon/pkg/dto"
	"github.com/openHPI/poseidon/tests"
	"github.com/sirupsen/logrus"
	"github.com/sirupsen/logrus/hooks/test"
	"github.com/stretchr/testify/suite"
)

func mockHTTPStatusHandler(status int) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(status)
	})
}

type MainTestSuite struct {
	tests.MemoryLeakTestSuite
}

func TestMainTestSuite(t *testing.T) {
	suite.Run(t, new(MainTestSuite))
}

func (s *MainTestSuite) TestHTTPMiddlewareDebugsWhenStatusOK() {
	var hook *test.Hook
	log, hook = test.NewNullLogger()
	InitializeLogging(logrus.DebugLevel.String(), dto.FormatterText)

	request, err := http.NewRequest(http.MethodGet, "/", http.NoBody)
	if err != nil {
		s.Fail(err.Error())
	}
	recorder := httptest.NewRecorder()
	HTTPLoggingMiddleware(mockHTTPStatusHandler(200)).ServeHTTP(recorder, request)

	s.Len(hook.Entries, 1)
	s.Equal(logrus.DebugLevel, hook.LastEntry().Level)
}
