package auth

import (
	"github.com/sirupsen/logrus"
	"github.com/sirupsen/logrus/hooks/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/config"
	"net/http"
	"net/http/httptest"
	"testing"
)

const testToken = "C0rr3ctT0k3n"

type AuthenticationMiddlewareTestSuite struct {
	suite.Suite
	request                      *http.Request
	recorder                     *httptest.ResponseRecorder
	httpAuthenticationMiddleware http.Handler
}

func (suite *AuthenticationMiddlewareTestSuite) SetupTest() {
	config.Config.Server.Token = testToken
	InitializeAuthentication()
	suite.recorder = httptest.NewRecorder()
	request, err := http.NewRequest(http.MethodGet, "/api/v1/test", nil)
	if err != nil {
		suite.T().Fatal(err)
	}
	suite.request = request
	suite.httpAuthenticationMiddleware = HTTPAuthenticationMiddleware(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
}

func (suite *AuthenticationMiddlewareTestSuite) TestReturns401WhenHeaderUnset() {
	suite.httpAuthenticationMiddleware.ServeHTTP(suite.recorder, suite.request)
	assert.Equal(suite.T(), http.StatusUnauthorized, suite.recorder.Code)
}

func (suite *AuthenticationMiddlewareTestSuite) TestReturns401WhenTokenWrong() {
	suite.request.Header.Set(TokenHeader, "Wr0ngT0k3n")
	suite.httpAuthenticationMiddleware.ServeHTTP(suite.recorder, suite.request)
	assert.Equal(suite.T(), http.StatusUnauthorized, suite.recorder.Code)
}

func (suite *AuthenticationMiddlewareTestSuite) TestWarnsWhenUnauthorized() {
	var hook *test.Hook
	logger, hook := test.NewNullLogger()
	log = logger.WithField("pkg", "api/auth")

	suite.request.Header.Set(TokenHeader, "Wr0ngT0k3n")
	suite.httpAuthenticationMiddleware.ServeHTTP(suite.recorder, suite.request)

	assert.Equal(suite.T(), http.StatusUnauthorized, suite.recorder.Code)
	assert.Equal(suite.T(), logrus.WarnLevel, hook.LastEntry().Level)
	assert.Equal(suite.T(), hook.LastEntry().Data["token"], "Wr0ngT0k3n")
}

func (suite *AuthenticationMiddlewareTestSuite) TestPassesWhenTokenCorrect() {
	suite.request.Header.Set(TokenHeader, testToken)
	suite.httpAuthenticationMiddleware.ServeHTTP(suite.recorder, suite.request)

	assert.Equal(suite.T(), http.StatusOK, suite.recorder.Code)
}

func TestHTTPAuthenticationMiddleware(t *testing.T) {
	suite.Run(t, new(AuthenticationMiddlewareTestSuite))
}
