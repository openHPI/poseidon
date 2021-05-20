package e2e

import (
	"github.com/stretchr/testify/suite"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/config"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/logging"
	"os"
	"testing"
	"time"
)

/*
* # E2E Tests
*
* For the e2e tests a nomad cluster must be connected and poseidon must be running.
 */

var log = logging.GetLogger("e2e")

type E2ETestSuite struct {
	suite.Suite
}

func (suite *E2ETestSuite) SetupTest() {
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
		log.Warn("Could not initialize configuration")
	}
	// ToDo: Add Nomad job here when it is possible to create execution environments. See #26.
	log.Info("Test Run")
	code := m.Run()
	os.Exit(code)
}
