package api

import (
	"encoding/json"
	"github.com/gorilla/mux"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/api/auth"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/api/dto"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/logging"
	"net/http"
)

var log = logging.GetLogger("api")

const (
	RouteBase    = "/api/v1"
	RouteHealth  = "/health"
	RouteRunners = "/runners"
)

// NewRouter returns an HTTP handler (http.Handler) which can be
// used by the net/http package to serve the routes of our API. It
// always returns a router for the newest version of our API. We
// use gorilla/mux because it is more convenient than net/http, e.g.
// when extracting path parameters.
func NewRouter() http.Handler {
	router := mux.NewRouter()
	// this can later be restricted to a specific host with
	// `router.Host(...)` and to HTTPS with `router.Schemes("https")`
	router = newRouterV1(router)
	router.Use(logging.HTTPLoggingMiddleware)
	if auth.InitializeAuthentication() {
		router.Use(auth.HTTPAuthenticationMiddleware)
	}
	return router
}

// newRouterV1 returns a sub-router containing the routes of version
// 1 of our API.
func newRouterV1(router *mux.Router) *mux.Router {
	v1 := router.PathPrefix(RouteBase).Subrouter()
	v1.HandleFunc(RouteHealth, Health).Methods(http.MethodGet)
	registerRunnerRoutes(v1.PathPrefix(RouteRunners).Subrouter())
	return v1
}

func writeInternalServerError(writer http.ResponseWriter, err error, errorCode dto.ErrorCode) {
	sendJson(writer, &dto.InternalServerError{Message: err.Error(), ErrorCode: errorCode}, http.StatusInternalServerError)
}

func writeBadRequest(writer http.ResponseWriter, err error) {
	sendJson(writer, &dto.ClientError{Message: err.Error()}, http.StatusBadRequest)
}

func writeNotFound(writer http.ResponseWriter, err error) {
	sendJson(writer, &dto.ClientError{Message: err.Error()}, http.StatusNotFound)
}

func sendJson(writer http.ResponseWriter, content interface{}, httpStatusCode int) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(httpStatusCode)
	response, err := json.Marshal(content)
	if err != nil {
		// cannot produce infinite recursive loop, since json.Marshal of dto.InternalServerError won't return an error
		writeInternalServerError(writer, err, dto.ErrorUnknown)
		return
	}
	if _, err := writer.Write(response); err != nil {
		log.WithError(err).Warn("Error writing JSON to response")
		http.Error(writer, err.Error(), http.StatusInternalServerError)
	}
}
