package main

import (
	"context"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/api"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/config"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/environment"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/logging"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/nomad"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

var log = logging.GetLogger("main")

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
		if err == http.ErrServerClosed {
			log.WithError(err).Info("Server closed")
		} else {
			log.WithError(err).Fatal("Error during listening and serving")
		}
	}
}

func initServer(apiClient nomad.ExecutorApi, runnerPool environment.RunnerPool) *http.Server {
	return &http.Server{
		Addr:         config.Config.PoseidonAPIURL().Host,
		WriteTimeout: time.Second * 15,
		ReadTimeout:  time.Second * 15,
		IdleTimeout:  time.Second * 60,
		Handler:      api.NewRouter(apiClient, runnerPool),
	}
}

// shutdownOnOSSignal listens for a signal from the operation system
// When receiving a signal the server shuts down but waits up to 15 seconds to close remaining connections
func shutdownOnOSSignal(server *http.Server) {
	// wait for SIGINT
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)
	<-signals
	log.Info("Received SIGINT, shutting down ...")

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	_ = server.Shutdown(ctx)
}

func main() {
	if err := config.InitConfig(); err != nil {
		log.WithError(err).Warn("Could not initialize configuration")
	}
	logging.InitializeLogging(config.Config.Logger.Level)

	// API initialization
	nomadAPIClient, err := nomad.New(config.Config.NomadAPIURL())
	if err != nil {
		log.WithError(err).WithField("nomad url", config.Config.NomadAPIURL()).Fatal("Error parsing the nomad url")
	}

	// ToDo: Move to create execution environment
	runnerPool := environment.NewLocalRunnerPool()
	environment.DebugInit(runnerPool, nomadAPIClient)

	server := initServer(nomadAPIClient, runnerPool)
	go runServer(server)
	shutdownOnOSSignal(server)
}
