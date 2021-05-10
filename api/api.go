package api

import (
	"github.com/gorilla/mux"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/api/auth"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/environment"
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
func NewRouter(runnerPool environment.RunnerPool) *mux.Router {
	router := mux.NewRouter()
	// this can later be restricted to a specific host with
	// `router.Host(...)` and to HTTPS with `router.Schemes("https")`
	router = newRouterV1(router, runnerPool)
	router.Use(logging.HTTPLoggingMiddleware)
	return router
}

// newRouterV1 returns a sub-router containing the routes of version
// 1 of our API.
func newRouterV1(router *mux.Router, runnerPool environment.RunnerPool) *mux.Router {
	v1 := router.PathPrefix(RouteBase).Subrouter()
	v1.HandleFunc(RouteHealth, Health).Methods(http.MethodGet)

	if auth.InitializeAuthentication() {
		// Create new authenticated subrouter.
		// All routes added to v1 after this require authentication.
		v1 = v1.PathPrefix("").Subrouter()
		v1.Use(auth.HTTPAuthenticationMiddleware)
	}
	registerRunnerRoutes(v1.PathPrefix(RouteRunners).Subrouter(), runnerPool)

	return v1
}
