package api

import (
	"github.com/gorilla/mux"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/api/auth"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/environment"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/logging"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/runner"
	"net/http"
)

var log = logging.GetLogger("api")

const (
	RouteBase    = "/api/v1"
	RouteHealth  = "/health"
	RouteRunners = "/runners"
)

// NewRouter returns a *mux.Router which can be
// used by the net/http package to serve the routes of our API. It
// always returns a router for the newest version of our API. We
// use gorilla/mux because it is more convenient than net/http, e.g.
// when extracting path parameters.
func NewRouter(runnerManager runner.Manager, environmentManager environment.Manager) *mux.Router {
	router := mux.NewRouter()
	// this can later be restricted to a specific host with
	// `router.Host(...)` and to HTTPS with `router.Schemes("https")`
	configureV1Router(router, runnerManager, environmentManager)
	router.Use(logging.HTTPLoggingMiddleware)
	return router
}

// configureV1Router configures a given router with the routes of version 1 of the Poseidon API.
func configureV1Router(router *mux.Router, runnerManager runner.Manager, environmentManager environment.Manager) {
	v1 := router.PathPrefix(RouteBase).Subrouter()
	v1.HandleFunc(RouteHealth, Health).Methods(http.MethodGet)

	runnerController := &RunnerController{manager: runnerManager}

	if auth.InitializeAuthentication() {
		// Create new authenticated subrouter.
		// All routes added to v1 after this require authentication.
		authenticatedV1Router := v1.PathPrefix("").Subrouter()
		authenticatedV1Router.Use(auth.HTTPAuthenticationMiddleware)
		runnerController.ConfigureRoutes(authenticatedV1Router)
	} else {
		runnerController.ConfigureRoutes(v1)
	}
}
