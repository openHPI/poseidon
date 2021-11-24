package api

import (
	"github.com/gorilla/mux"
	"github.com/openHPI/poseidon/internal/api/auth"
	"github.com/openHPI/poseidon/internal/config"
	"github.com/openHPI/poseidon/internal/environment"
	"github.com/openHPI/poseidon/internal/runner"
	"github.com/openHPI/poseidon/pkg/logging"
	"net/http"
)

var log = logging.GetLogger("api")

const (
	BasePath         = "/api/v1"
	HealthPath       = "/health"
	VersionPath      = "/version"
	RunnersPath      = "/runners"
	EnvironmentsPath = "/execution-environments"
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
	router.NotFoundHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.WithField("request", r).Debug("Not Found Handler")
		w.WriteHeader(http.StatusNotFound)
	})
	v1 := router.PathPrefix(BasePath).Subrouter()
	v1.HandleFunc(HealthPath, Health).Methods(http.MethodGet)
	v1.HandleFunc(VersionPath, Version).Methods(http.MethodGet)

	runnerController := &RunnerController{manager: runnerManager}
	environmentController := &EnvironmentController{manager: environmentManager}

	configureRoutes := func(router *mux.Router) {
		runnerController.ConfigureRoutes(router)
		environmentController.ConfigureRoutes(router)
	}

	if auth.InitializeAuthentication() {
		// Create new authenticated subrouter.
		// All routes added to v1 after this require authentication.
		authenticatedV1Router := v1.PathPrefix("").Subrouter()
		authenticatedV1Router.Use(auth.HTTPAuthenticationMiddleware)
		configureRoutes(authenticatedV1Router)
	} else {
		configureRoutes(v1)
	}
}

// Version handles the version route.
// It responds the release information stored in the configuration.
func Version(writer http.ResponseWriter, _ *http.Request) {
	release := config.Config.Sentry.Release
	if len(release) > 0 {
		sendJSON(writer, release, http.StatusOK)
	} else {
		writer.WriteHeader(http.StatusNotFound)
	}
}
