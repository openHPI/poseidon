package e2e_tests

import (
	"gitlab.hpi.de/codeocean/codemoon/poseidon/api"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/config"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/logging"
	"os"
	"strings"
	"testing"
)

/*
* # E2E Tests
*
* For the e2e tests a nomad cluster must be connected and poseidon must be running.
 */

var log = logging.GetLogger("e2e_tests")

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

func buildURL(parts ...string) (url string) {
	parts = append([]string{config.Config.PoseidonAPIURL().String(), api.RouteBase}, parts...)
	return strings.Join(parts, "")
}
