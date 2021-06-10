package e2e

import (
	nomadApi "github.com/hashicorp/nomad/api"
	"github.com/stretchr/testify/suite"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/api"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/api/dto"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/config"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/logging"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/tests"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/tests/helpers"
	"net/http"
	"os"
	"testing"
	"time"
)

/*
* # E2E Tests
*
* For the e2e tests a nomad cluster must be connected and poseidon must be running.
 */

var (
	log            = logging.GetLogger("e2e")
	nomadClient    *nomadApi.Client
	nomadNamespace string
)

type E2ETestSuite struct {
	suite.Suite
}

func (s *E2ETestSuite) SetupTest() {
	// Waiting one second before each test allows Nomad to rescale after tests requested runners.
	<-time.After(time.Second)
}

func TestE2ETestSuite(t *testing.T) {
	suite.Run(t, new(E2ETestSuite))
}

// Overwrite TestMain for custom setup.
func TestMain(m *testing.M) {
	log.Info("Test Setup")
	err := config.InitConfig()
	if err != nil {
		log.WithError(err).Fatal("Could not initialize configuration")
	}
	nomadNamespace = config.Config.Nomad.Namespace
	nomadClient, err = nomadApi.NewClient(&nomadApi.Config{
		Address:   config.Config.NomadAPIURL().String(),
		TLSConfig: &nomadApi.TLSConfig{},
		Namespace: nomadNamespace,
	})
	if err != nil {
		log.WithError(err).Fatal("Could not create Nomad client")
	}
	log.Info("Test Run")
	createDefaultEnvironment()

	// wait for environment to become ready
	<-time.After(10 * time.Second)

	code := m.Run()
	cleanupJobsForEnvironment(&testing.T{}, "0")
	os.Exit(code)
}

func createDefaultEnvironment() {
	path := helpers.BuildURL(api.BasePath, api.EnvironmentsPath, tests.DefaultEnvironmentIDAsString)

	request := dto.ExecutionEnvironmentRequest{
		PrewarmingPoolSize: 10,
		CPULimit:           100,
		MemoryLimit:        100,
		Image:              "drp.codemoon.xopic.de/openhpi/co_execenv_python:3.8",
		NetworkAccess:      false,
		ExposedPorts:       nil,
	}

	resp, err := helpers.HttpPutJSON(path, request)
	if err != nil || resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusNoContent {
		log.Fatal("Couldn't create default environment for e2e tests")
	}
	err = resp.Body.Close()
	if err != nil {
		log.Fatal("Failed closing body")
	}
}
