package main

import (
	"gitlab.hpi.de/codeocean/codemoon/poseidon/config"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/nomad"
	"os"
	"testing"
	"time"
)

// overwrite TestMain for custom setup
func TestMain(m *testing.M) {
	log.Print("Test Setup")
	setup()
	log.Print("Test Run")
	code := m.Run()
	os.Exit(code)
}

func setup() {
	createNomadJob()
	go main()
	time.Sleep(15 * time.Second)
}

func createNomadJob() {
	nomadAPIClient, err := nomad.New(config.Config.NomadAPIURL())
	if err != nil {
		log.WithError(err).Fatal("[Test] Can not parse nomad config")
	}
	nomadAPIClient.CreateDebugJob()
}
