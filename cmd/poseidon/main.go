package main

import (
	"context"
	"errors"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/internal/api"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/internal/config"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/internal/environment"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/internal/nomad"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/internal/runner"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/pkg/logging"
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

func runServer(server *http.Server) {
	log.WithField("address", server.Addr).Info("Starting server")
	var err error
	if config.Config.Server.TLS {
		server.TLSConfig = config.TLSConfig
		log.
			WithField("CertFile", config.Config.Server.CertFile).
			WithField("KeyFile", config.Config.Server.KeyFile).
			Debug("Using TLS")
		err = server.ListenAndServeTLS(config.Config.Server.CertFile, config.Config.Server.KeyFile)
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

func initServer() *http.Server {
	// API initialization
	nomadAPIClient, err := nomad.NewExecutorAPI(config.Config.NomadAPIURL(), config.Config.Nomad.Namespace)
	if err != nil {
		log.WithError(err).WithField("nomad url", config.Config.NomadAPIURL()).Fatal("Error parsing the nomad url")
	}

	runnerManager := runner.NewNomadRunnerManager(nomadAPIClient, context.Background())
	environmentManager := environment.NewNomadEnvironmentManager(runnerManager, nomadAPIClient)

	return &http.Server{
		Addr:         config.Config.PoseidonAPIURL().Host,
		WriteTimeout: time.Second * 15,
		ReadTimeout:  time.Second * 15,
		IdleTimeout:  time.Second * 60,
		Handler:      api.NewRouter(runnerManager, environmentManager),
	}
}

// shutdownOnOSSignal listens for a signal from the operation system
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

	server := initServer()
	go runServer(server)
	shutdownOnOSSignal(server)
}
