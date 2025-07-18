package auth

import (
	"crypto/subtle"
	"net/http"

	"github.com/openHPI/poseidon/internal/config"
	"github.com/openHPI/poseidon/pkg/logging"
)

var log = logging.GetLogger("api/auth")

const TokenHeader = "Poseidon-Token"

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
			log.WithContext(r.Context()).
				WithField("token", logging.RemoveNewlineSymbol(token)).
				Warn("Incorrect token")
			w.WriteHeader(http.StatusUnauthorized)

			return
		}

		next.ServeHTTP(w, r)
	})
}
