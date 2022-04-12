package api

import (
	"github.com/gorilla/mux"
	influxdb2API "github.com/influxdata/influxdb-client-go/v2/api"
	"github.com/openHPI/poseidon/internal/api/auth"
	"github.com/openHPI/poseidon/internal/config"
	"github.com/openHPI/poseidon/internal/environment"
	"github.com/openHPI/poseidon/internal/runner"
	"github.com/openHPI/poseidon/pkg/dto"
	"github.com/openHPI/poseidon/pkg/logging"
	"github.com/openHPI/poseidon/pkg/monitoring"
	"net/http"
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
func NewRouter(runnerManager runner.Manager, environmentManager environment.ManagerHandler,
	influxClient influxdb2API.WriteAPI) *mux.Router {
	router := mux.NewRouter()
	// this can later be restricted to a specific host with
	// `router.Host(...)` and to HTTPS with `router.Schemes("https")`
	configureV1Router(router, runnerManager, environmentManager)
	router.Use(logging.HTTPLoggingMiddleware)
	router.Use(monitoring.InfluxDB2Middleware(influxClient, environmentManager))
	return router
}

// configureV1Router configures a given router with the routes of version 1 of the Poseidon API.
func configureV1Router(router *mux.Router,
	runnerManager runner.Manager, environmentManager environment.ManagerHandler) {
	router.NotFoundHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.WithField("request", r).Debug("Not Found Handler")
		w.WriteHeader(http.StatusNotFound)
	})
	v1 := router.PathPrefix(BasePath).Subrouter()
	v1.HandleFunc(HealthPath, Health).Methods(http.MethodGet).Name(HealthPath)
	v1.HandleFunc(VersionPath, Version).Methods(http.MethodGet).Name(VersionPath)

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

// StatisticsExecutionEnvironments handles the route for statistics about execution environments.
// It responds the prewarming pool size and the number of idle runners and used runners.
func StatisticsExecutionEnvironments(manager environment.Manager) http.HandlerFunc {
	return func(writer http.ResponseWriter, _ *http.Request) {
		result := make(map[string]*dto.StatisticalExecutionEnvironmentData)
		environmentsData := manager.Statistics()
		for id, data := range environmentsData {
			result[id.ToString()] = data
		}
		sendJSON(writer, result, http.StatusOK)
	}
}
