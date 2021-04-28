package main

import (
	"context"
	"fmt"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/api"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/config"
	"log"
	"net/http"
	"os"
	"os/signal"
	"time"
)

func main() {
	config.InitConfig()

	server := &http.Server{
		Addr:         fmt.Sprintf("%s:%d", config.Config.Server.Address, config.Config.Server.Port),
		WriteTimeout: time.Second * 15,
		ReadTimeout:  time.Second * 15,
		IdleTimeout:  time.Second * 60,
		Handler:      api.NewRouter(),
	}
	go func() {
		if err := server.ListenAndServe(); err != nil {
			if err == http.ErrServerClosed {
				log.Println(err)
			} else {
				log.Fatal("Error during listening and serving: ", err)
			}
		}
	}()

	// wait for SIGINT
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt)
	<-signals
	log.Println("Received SIGINT, shutting down ...")

	// shutdown the server but wait up to 15 seconds to close remaining connections
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	_ = server.Shutdown(ctx)
}
