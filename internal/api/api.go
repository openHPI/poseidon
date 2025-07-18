package api

import (
	"net/http"

	"github.com/gorilla/mux"
	"github.com/openHPI/poseidon/internal/api/auth"
	"github.com/openHPI/poseidon/internal/config"
	"github.com/openHPI/poseidon/internal/environment"
	"github.com/openHPI/poseidon/internal/runner"
	"github.com/openHPI/poseidon/pkg/dto"
	"github.com/openHPI/poseidon/pkg/logging"
	"github.com/openHPI/poseidon/pkg/monitoring"
)

var log = logging.GetLogger("api")

const (
	BasePath         = "/api/v1"
	HealthPath       = "/health"
	VersionPath      = "/version"
	RunnersPath      = "/runners"
	EnvironmentsPath = "/execution-environments"
	StatisticsPath   = "/statistics"
)

// NewRouter returns a *mux.Router which can be
// used by the net/http package to serve the routes of our API. It
// always returns a router for the newest version of our API. We
// use gorilla/mux because it is more convenient than net/http, e.g.
// when extracting path parameters.
func NewRouter(runnerManager runner.Manager, environmentManager environment.ManagerHandler) *mux.Router {
	router := mux.NewRouter()
	// this can later be restricted to a specific host with
	// `router.Host(...)` and to HTTPS with `router.Schemes("https")`
	configureV1Router(router, runnerManager, environmentManager)
	router.Use(logging.HTTPLoggingMiddleware)
	router.Use(monitoring.InfluxDB2Middleware)

	return router
}

// configureV1Router configures a given router with the routes of version 1 of the Poseidon API.
func configureV1Router(router *mux.Router,
	runnerManager runner.Manager, environmentManager environment.ManagerHandler,
) {
	router.NotFoundHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.WithContext(r.Context()).WithField("request", r).Debug("Not Found Handler")
		w.WriteHeader(http.StatusNotFound)
	})
	v1SubRouter := router.PathPrefix(BasePath).Subrouter()
	v1SubRouter.HandleFunc(HealthPath, Health(environmentManager)).Methods(http.MethodGet).Name(HealthPath)
	v1SubRouter.HandleFunc(VersionPath, Version).Methods(http.MethodGet).Name(VersionPath)

	runnerController := &RunnerController{manager: runnerManager}
	environmentController := &EnvironmentController{manager: environmentManager}
	configureRoutes := func(router *mux.Router) {
		runnerController.ConfigureRoutes(router)
		environmentController.ConfigureRoutes(router)

		// May add a statistics controller if another route joins
		statisticsRouter := router.PathPrefix(StatisticsPath).Subrouter()
		statisticsRouter.
			HandleFunc(EnvironmentsPath, StatisticsExecutionEnvironments(environmentManager)).
			Methods(http.MethodGet).Name(EnvironmentsPath)
	}

	if auth.InitializeAuthentication() {
		// Create new authenticated sub router.
		// All routes added to v1 after this require authentication.
		authenticatedV1Router := v1SubRouter.PathPrefix("").Subrouter()
		authenticatedV1Router.Use(auth.HTTPAuthenticationMiddleware)
		configureRoutes(authenticatedV1Router)
	} else {
		configureRoutes(v1SubRouter)
	}
}

// Version handles the version route.
// It responds the release information stored in the configuration.
func Version(writer http.ResponseWriter, request *http.Request) {
	release := config.Config.Sentry.Release
	if release != "" {
		sendJSON(request.Context(), writer, release, http.StatusOK)
	} else {
		writer.WriteHeader(http.StatusNotFound)
	}
}

// StatisticsExecutionEnvironments handles the route for statistics about execution environments.
// It responds the prewarming pool size and the number of idle runners and used runners.
func StatisticsExecutionEnvironments(manager environment.Manager) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		result := make(map[string]*dto.StatisticalExecutionEnvironmentData)

		environmentsData := manager.Statistics()
		for id, data := range environmentsData {
			result[id.ToString()] = data
		}

		sendJSON(request.Context(), writer, result, http.StatusOK)
	}
}
