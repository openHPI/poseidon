package auth

import (
	"crypto/subtle"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/config"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/logging"
	"net/http"
)

var log = logging.GetLogger("api/auth")

const TokenHeader = "X-Poseidon-Token"

var correctAuthenticationToken []byte

// InitializeAuthentication returns true iff the authentication is initialized successfully and can be used.
func InitializeAuthentication() bool {
	token := config.Config.Server.Token
	if token == "" {
		return false
	}
	correctAuthenticationToken = []byte(token)
	return true
}

func HTTPAuthenticationMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := r.Header.Get(TokenHeader)
		if subtle.ConstantTimeCompare([]byte(token), correctAuthenticationToken) == 0 {
			log.WithField("token", token).Warn("Incorrect token")
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}
