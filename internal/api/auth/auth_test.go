package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/openHPI/poseidon/internal/config"
	"github.com/openHPI/poseidon/tests"
	"github.com/sirupsen/logrus"
	"github.com/sirupsen/logrus/hooks/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

const testToken = "C0rr3ctT0k3n"

type AuthenticationMiddlewareTestSuite struct {
	tests.MemoryLeakTestSuite

	request                      *http.Request
	recorder                     *httptest.ResponseRecorder
	httpAuthenticationMiddleware http.Handler
}

func (s *AuthenticationMiddlewareTestSuite) SetupTest() {
	s.MemoryLeakTestSuite.SetupTest()

	correctAuthenticationToken = []byte(testToken)
	s.recorder = httptest.NewRecorder()

	request, err := http.NewRequest(http.MethodGet, "/api/v1/test", http.NoBody)
	if err != nil {
		s.T().Fatal(err)
	}

	s.request = request
	s.httpAuthenticationMiddleware = HTTPAuthenticationMiddleware(
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
}

func (s *AuthenticationMiddlewareTestSuite) TearDownTest() {
	defer s.MemoryLeakTestSuite.TearDownTest()

	correctAuthenticationToken = []byte(nil)
}

func (s *AuthenticationMiddlewareTestSuite) TestReturns401WhenHeaderUnset() {
	s.httpAuthenticationMiddleware.ServeHTTP(s.recorder, s.request)
	s.Equal(http.StatusUnauthorized, s.recorder.Code)
}

func (s *AuthenticationMiddlewareTestSuite) TestReturns401WhenTokenWrong() {
	s.request.Header.Set(TokenHeader, "Wr0ngT0k3n")
	s.httpAuthenticationMiddleware.ServeHTTP(s.recorder, s.request)
	s.Equal(http.StatusUnauthorized, s.recorder.Code)
}

func (s *AuthenticationMiddlewareTestSuite) TestWarnsWhenUnauthorized() {
	var hook *test.Hook

	logger, hook := test.NewNullLogger()
	log = logger.WithField("pkg", "api/auth")

	s.request.Header.Set(TokenHeader, "Wr0ngT0k3n")
	s.httpAuthenticationMiddleware.ServeHTTP(s.recorder, s.request)

	s.Equal(http.StatusUnauthorized, s.recorder.Code)
	s.Equal(logrus.WarnLevel, hook.LastEntry().Level)
	s.Equal("Wr0ngT0k3n", hook.LastEntry().Data["token"])
}

func (s *AuthenticationMiddlewareTestSuite) TestPassesWhenTokenCorrect() {
	s.request.Header.Set(TokenHeader, testToken)
	s.httpAuthenticationMiddleware.ServeHTTP(s.recorder, s.request)

	s.Equal(http.StatusOK, s.recorder.Code)
}

func TestHTTPAuthenticationMiddleware(t *testing.T) {
	suite.Run(t, new(AuthenticationMiddlewareTestSuite))
}

func TestInitializeAuthentication(t *testing.T) {
	t.Run("if token unset", func(t *testing.T) {
		config.Config.Server.Token = ""
		initialized := InitializeAuthentication()
		assert.False(t, initialized)
		assert.Equal(t, []byte(nil), correctAuthenticationToken, "it should not set correctAuthenticationToken")
	})
	t.Run("if token set", func(t *testing.T) {
		config.Config.Server.Token = testToken
		initialized := InitializeAuthentication()
		assert.True(t, initialized)
		assert.Equal(t, []byte(testToken), correctAuthenticationToken, "it should set correctAuthenticationToken")

		config.Config.Server.Token = ""
		correctAuthenticationToken = []byte(nil)
	})
}
