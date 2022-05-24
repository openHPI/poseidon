package main

import (
	"context"
	"errors"
	"github.com/getsentry/sentry-go"
	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
	influxdb2API "github.com/influxdata/influxdb-client-go/v2/api"
	"github.com/openHPI/poseidon/internal/api"
	"github.com/openHPI/poseidon/internal/config"
	"github.com/openHPI/poseidon/internal/environment"
	"github.com/openHPI/poseidon/internal/nomad"
	"github.com/openHPI/poseidon/internal/runner"
	"github.com/openHPI/poseidon/pkg/logging"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

var (
	gracefulShutdownWait = 15 * time.Second
	log                  = logging.GetLogger("main")
)

func initSentry(options *sentry.ClientOptions) {
	if err := sentry.Init(*options); err != nil {
		log.Errorf("sentry.Init: %s", err)
	}
}

func shutdownSentry() {
	if err := recover(); err != nil {
		sentry.CurrentHub().Recover(err)
		sentry.Flush(logging.GracefulSentryShutdown)
	}
}

func initInfluxDB(db *config.InfluxDB) (writeAPI influxdb2API.WriteAPI, cancel func()) {
	if db.URL == "" {
		return nil, func() {}
	}

	client := influxdb2.NewClient(db.URL, db.Token)
	writeAPI = client.WriteAPI(db.Organization, db.Bucket)
	cancel = func() {
		writeAPI.Flush()
		client.Close()
	}
	return writeAPI, cancel
}

func runServer(server *http.Server) {
	log.WithField("address", server.Addr).Info("Starting server")
	var err error
	if config.Config.Server.TLS.Active {
		server.TLSConfig = config.TLSConfig
		log.
			WithField("CertFile", config.Config.Server.TLS.CertFile).
			WithField("KeyFile", config.Config.Server.TLS.KeyFile).
			Debug("Using TLS")
		err = server.ListenAndServeTLS(config.Config.Server.TLS.CertFile, config.Config.Server.TLS.KeyFile)
	} else {
		err = server.ListenAndServe()
	}
	if err != nil {
		if errors.Is(err, http.ErrServerClosed) {
			log.WithError(err).Info("Server closed")
		} else {
			log.WithError(err).Fatal("Error during listening and serving")
		}
	}
}

type managerCreator func() (runnerManager runner.Manager, environmentManager environment.ManagerHandler)

// createManagerHandler adds the managers of the passed managerCreator to the chain of responsibility.
func createManagerHandler(handler managerCreator, enabled bool,
	nextRunnerManager runner.Manager, nextEnvironmentManager environment.ManagerHandler) (
	runnerManager runner.Manager, environmentManager environment.ManagerHandler) {
	if !enabled {
		return nextRunnerManager, nextEnvironmentManager
	}

	runnerManager, environmentManager = handler()
	runnerManager.SetNextHandler(nextRunnerManager)
	environmentManager.SetNextHandler(nextEnvironmentManager)
	return runnerManager, environmentManager
}

func createNomadManager() (runnerManager runner.Manager, environmentManager environment.ManagerHandler) {
	// API initialization
	nomadAPIClient, err := nomad.NewExecutorAPI(&config.Config.Nomad)
	if err != nil {
		log.WithError(err).WithField("nomad config", config.Config.Nomad).Fatal("Error creating Nomad API client")
	}

	runnerManager = runner.NewNomadRunnerManager(nomadAPIClient, context.Background())
	environmentManager, err = environment.
		NewNomadEnvironmentManager(runnerManager, nomadAPIClient, config.Config.Server.TemplateJobFile)
	if err != nil {
		log.WithError(err).Fatal("Error initializing environment manager")
	}
	return runnerManager, environmentManager
}

func createAWSManager() (runnerManager runner.Manager, environmentManager environment.ManagerHandler) {
	runnerManager = runner.NewAWSRunnerManager()
	return runnerManager, environment.NewAWSEnvironmentManager(runnerManager)
}

// initServer builds the http server and configures it with the chain of responsibility for multiple managers.
func initServer(influxClient influxdb2API.WriteAPI) *http.Server {
	runnerManager, environmentManager := createManagerHandler(createNomadManager, config.Config.Nomad.Enabled,
		runner.NewAbstractManager(), &environment.AbstractManager{})
	runnerManager, environmentManager = createManagerHandler(createAWSManager, config.Config.AWS.Enabled,
		runnerManager, environmentManager)

	return &http.Server{
		Addr:        config.Config.Server.URL().Host,
		ReadTimeout: time.Second * 15,
		IdleTimeout: time.Second * 60,
		Handler:     api.NewRouter(runnerManager, environmentManager, influxClient),
	}
}

// shutdownOnOSSignal listens for a signal from the operating system
// When receiving a signal the server shuts down but waits up to 15 seconds to close remaining connections.
func shutdownOnOSSignal(server *http.Server) {
	// wait for SIGINT
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)
	<-signals
	log.Info("Received SIGINT, shutting down ...")

	ctx, cancel := context.WithTimeout(context.Background(), gracefulShutdownWait)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		log.WithError(err).Warn("error shutting server down")
	}
}

func main() {
	if err := config.InitConfig(); err != nil {
		log.WithError(err).Warn("Could not initialize configuration")
	}
	logging.InitializeLogging(config.Config.Logger.Level)
	initSentry(&config.Config.Sentry)
	defer shutdownSentry()

	influxDB, cancel := initInfluxDB(&config.Config.InfluxDB)
	defer cancel()

	server := initServer(influxDB)
	go runServer(server)
	shutdownOnOSSignal(server)
}
