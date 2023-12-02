package recovery

import (
	"github.com/openHPI/poseidon/internal/api"
	"github.com/openHPI/poseidon/pkg/dto"
	"github.com/openHPI/poseidon/tests"
	"github.com/openHPI/poseidon/tests/e2e"
	"github.com/openHPI/poseidon/tests/helpers"
	"github.com/shirou/gopsutil/v3/process"
	"net/http"
	"time"
)

func (s *E2ERecoveryTestSuite) SetupTest() {
	<-time.After(InactivityTimeout * time.Second)
	// We do not want runner from the previous tests

	var err error
	s.runnerID, err = e2e.ProvideRunner(&dto.RunnerRequest{
		ExecutionEnvironmentID: tests.DefaultEnvironmentIDAsInteger,
		InactivityTimeout:      InactivityTimeout,
	})
	if err != nil {
		log.WithError(err).Fatal("Could not provide runner")
	}

	<-time.After(tests.ShortTimeout)
	killPoseidon()
	<-time.After(tests.ShortTimeout)
}

func TearDown() {
	path := helpers.BuildURL(api.BasePath, api.EnvironmentsPath, tests.DefaultEnvironmentIDAsString)
	_, err := helpers.HTTPDelete(path, nil)
	if err != nil {
		log.WithError(err).Fatal("Could not remove default environment")
	}
}

func waitForPoseidon() {
	done := false
	for !done {
		<-time.After(time.Second)
		resp, err := http.Get(helpers.BuildURL(api.BasePath, api.HealthPath))
		done = err == nil && resp.StatusCode == http.StatusNoContent
	}
}

func killPoseidon() {
	processes, err := process.Processes()
	if err != nil {
		log.WithError(err).Error("Error listing processes")
	}
	for _, p := range processes {
		n, err := p.Name()
		if err != nil {
			continue
		}
		if n == "poseidon" {
			err = p.Kill()
			if err != nil {
				log.WithError(err).Error("Error killing Poseidon")
			} else {
				log.Info("Killed Poseidon")
			}
		}
	}
}
