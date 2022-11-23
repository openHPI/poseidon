package recovery

import (
	"context"
	"encoding/json"
	"flag"
	nomadApi "github.com/hashicorp/nomad/api"
	"github.com/openHPI/poseidon/internal/api"
	"github.com/openHPI/poseidon/internal/config"
	"github.com/openHPI/poseidon/pkg/dto"
	"github.com/openHPI/poseidon/pkg/logging"
	"github.com/openHPI/poseidon/tests"
	"github.com/openHPI/poseidon/tests/e2e"
	"github.com/openHPI/poseidon/tests/helpers"
	"github.com/stretchr/testify/suite"
	"net/http"
	"os"
	"testing"
	"time"
)

/*
* # E2E Recovery Tests
*
* For the e2e tests a nomad cluster must be connected and poseidon must be running.
* These cases test the behavior of Poseidon when restarting / recovering.
 */

var (
	log             = logging.GetLogger("e2e-recovery")
	testDockerImage = flag.String("dockerImage", "", "Docker image to use in E2E tests")
	poseidonBinary  = flag.String("poseidonPath", "", "The path to the Poseidon binary")
	nomadClient     *nomadApi.Client
	nomadNamespace  string
)

// InactivityTimeout of the created runner in seconds.
const (
	InactivityTimeout  = 1
	PrewarmingPoolSize = 2
)

type E2ERecoveryTestSuite struct {
	suite.Suite
	runnerID       string
	poseidonCancel context.CancelFunc
}

// Overwrite TestMain for custom setup.
func TestMain(m *testing.M) {
	if err := config.InitConfig(); err != nil {
		log.WithError(err).Fatal("Could not initialize configuration")
	}

	if *poseidonBinary == "" {
		log.Fatal("You must specify the -path to the Poseidon binary!")
	}
	if *testDockerImage == "" {
		log.Fatal("You must specify the -dockerImage flag!")
	}

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

	os.Exit(m.Run())
}

func TestE2ERecoveryTests(t *testing.T) {
	testSuite := new(E2ERecoveryTestSuite)

	ctx, cancelPoseidon := context.WithCancel(context.Background())
	testSuite.poseidonCancel = cancelPoseidon

	startPoseidon(ctx, cancelPoseidon)
	waitForPoseidon()

	e2e.CreateDefaultEnvironment(PrewarmingPoolSize, *testDockerImage)
	e2e.WaitForDefaultEnvironment()

	suite.Run(t, testSuite)

	TearDown()
	testSuite.poseidonCancel()
	<-time.After(tests.ShortTimeout)
}

func (s *E2ERecoveryTestSuite) TestInactivityTimer_Valid() {
	_, err := e2e.ProvideWebSocketURL(s.runnerID, &dto.ExecutionRequest{Command: "true"})
	s.NoError(err)
}

func (s *E2ERecoveryTestSuite) TestInactivityTimer_Expired() {
	<-time.After(InactivityTimeout * time.Second)
	_, err := e2e.ProvideWebSocketURL(s.runnerID, &dto.ExecutionRequest{Command: "true"})
	s.Error(err)
}

// We expect the runner count to be equal to the prewarming pool size plus the one provided runner.
// If the count does not include the provided runner, the evaluation of the runner status may be wrong.
func (s *E2ERecoveryTestSuite) TestRunnerCount() {
	jobListStubs, _, err := nomadClient.Jobs().List(&nomadApi.QueryOptions{
		Prefix:    tests.DefaultEnvironmentIDAsString,
		Namespace: nomadNamespace,
	})
	s.Require().NoError(err)
	s.Equal(PrewarmingPoolSize+1, len(jobListStubs))
}

func (s *E2ERecoveryTestSuite) TestEnvironmentStatistics() {
	url := helpers.BuildURL(api.BasePath, api.StatisticsPath, api.EnvironmentsPath)
	response, err := http.Get(url) //nolint:gosec // The variability of this url is limited by our configurations.
	s.Require().NoError(err)
	s.Require().Equal(http.StatusOK, response.StatusCode)

	statistics := make(map[string]*dto.StatisticalExecutionEnvironmentData)
	err = json.NewDecoder(response.Body).Decode(&statistics)
	s.Require().NoError(err)
	err = response.Body.Close()
	s.Require().NoError(err)

	environmentStatistics, ok := statistics[tests.DefaultEnvironmentIDAsString]
	s.Require().True(ok)
	s.Equal(tests.DefaultEnvironmentIDAsInteger, environmentStatistics.ID)
	s.Equal(uint(PrewarmingPoolSize), environmentStatistics.PrewarmingPoolSize)
	s.Equal(uint(PrewarmingPoolSize), environmentStatistics.IdleRunners)
	s.Equal(uint(1), environmentStatistics.UsedRunners)
}
