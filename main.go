package main

import (
	"context"
	"fmt"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/api"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/config"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/logging"
	"net/http"
	"os"
	"os/signal"
	"time"
)

var log = logging.GetLogger("main")

func main() {
	config.InitConfig()
	logging.InitializeLogging(config.Config.Logger.Level)

	server := &http.Server{
		Addr:         fmt.Sprintf("%s:%d", config.Config.Server.Address, config.Config.Server.Port),
		WriteTimeout: time.Second * 15,
		ReadTimeout:  time.Second * 15,
		IdleTimeout:  time.Second * 60,
		Handler:      api.NewRouter(),
	}

	log.WithField("address", server.Addr).Info("Starting server")
	go func() {
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
	}()

	// wait for SIGINT
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt)
	<-signals
	log.Info("Received SIGINT, shutting down ...")

	// shutdown the server but wait up to 15 seconds to close remaining connections
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	_ = server.Shutdown(ctx)
}
