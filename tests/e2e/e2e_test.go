package e2e

import (
	"flag"
	nomadApi "github.com/hashicorp/nomad/api"
	"github.com/openHPI/poseidon/internal/api"
	"github.com/openHPI/poseidon/internal/config"
	"github.com/openHPI/poseidon/pkg/dto"
	"github.com/openHPI/poseidon/pkg/logging"
	"github.com/openHPI/poseidon/tests"
	"github.com/openHPI/poseidon/tests/helpers"
	"github.com/stretchr/testify/suite"
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
	log                     = logging.GetLogger("e2e")
	testDockerImage         = flag.String("dockerImage", "", "Docker image to use in E2E tests")
	nomadClient             *nomadApi.Client
	nomadNamespace          string
	environmentIDs          []dto.EnvironmentID
	defaultNomadEnvironment dto.ExecutionEnvironmentRequest
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
	if err := config.InitConfig(); err != nil {
		log.WithError(err).Fatal("Could not initialize configuration")
	}
	initNomad()
	initAWS()

	// wait for environment to become ready
	<-time.After(10 * time.Second)
	log.Info("Test Run")
	code := m.Run()

	deleteE2EEnvironments()
	cleanupJobsForEnvironment(&testing.T{}, tests.DefaultEnvironmentIDAsString)
	os.Exit(code)
}

func initAWS() {
	for i, function := range config.Config.AWS.Functions {
		log.WithField("function", function[0:3]).Info("Yes, we do have AWS functions.")
		id := dto.EnvironmentID(tests.DefaultEnvironmentIDAsInteger + i + 1)
		path := helpers.BuildURL(api.BasePath, api.EnvironmentsPath, id.ToString())
		request := dto.ExecutionEnvironmentRequest{Image: function}
		resp, err := helpers.HTTPPutJSON(path, request)
		if err != nil || resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusNoContent {
			log.WithField("function", function).WithError(err).Fatal("Couldn't create default environment for e2e tests")
		}
		environmentIDs = append(environmentIDs, id)
		err = resp.Body.Close()
		if err != nil {
			log.Fatal("Failed closing body")
		}
	}
}

func initNomad() {
	nomadNamespace = config.Config.Nomad.Namespace
	var err error
	nomadClient, err = nomadApi.NewClient(&nomadApi.Config{
		Address:   config.Config.Nomad.URL().String(),
		TLSConfig: &nomadApi.TLSConfig{},
		Namespace: nomadNamespace,
	})
	if err != nil {
		log.WithError(err).Fatal("Could not create Nomad client")
		return
	}
	createDefaultEnvironment()
	waitForDefaultEnvironment()
}

func createDefaultEnvironment() {
	if *testDockerImage == "" {
		log.Fatal("You must specify the -dockerImage flag!")
	}

	path := helpers.BuildURL(api.BasePath, api.EnvironmentsPath, tests.DefaultEnvironmentIDAsString)

	defaultNomadEnvironment = dto.ExecutionEnvironmentRequest{
		PrewarmingPoolSize: 10,
		CPULimit:           100,
		MemoryLimit:        100,
		Image:              *testDockerImage,
		NetworkAccess:      false,
		ExposedPorts:       nil,
	}

	resp, err := helpers.HTTPPutJSON(path, defaultNomadEnvironment)
	if err != nil || resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusNoContent {
		log.WithError(err).Fatal("Couldn't create default environment for e2e tests")
	}
	environmentIDs = append(environmentIDs, tests.DefaultEnvironmentIDAsInteger)
	err = resp.Body.Close()
	if err != nil {
		log.Fatal("Failed closing body")
	}
}

func waitForDefaultEnvironment() {
	path := helpers.BuildURL(api.BasePath, api.RunnersPath)
	body := &dto.RunnerRequest{
		ExecutionEnvironmentID: tests.DefaultEnvironmentIDAsInteger,
		InactivityTimeout:      1,
	}
	var code int
	const maxRetries = 60
	for count := 0; count < maxRetries && code != http.StatusOK; count++ {
		<-time.After(time.Second)
		if resp, err := helpers.HTTPPostJSON(path, body); err == nil {
			code = resp.StatusCode
			log.WithField("count", count).WithField("statusCode", code).Info("Waiting for idle runners")
		} else {
			log.WithField("count", count).WithError(err).Warn("Waiting for idle runners")
		}
	}
	if code != http.StatusOK {
		log.Fatal("Failed to provide a runner")
	}
}

func deleteE2EEnvironments() {
	for _, id := range environmentIDs {
		deleteEnvironment(&testing.T{}, id.ToString())
	}
}
