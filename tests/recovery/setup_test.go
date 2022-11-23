package recovery

import (
	"context"
	"github.com/openHPI/poseidon/internal/api"
	"github.com/openHPI/poseidon/pkg/dto"
	"github.com/openHPI/poseidon/tests"
	"github.com/openHPI/poseidon/tests/e2e"
	"github.com/openHPI/poseidon/tests/helpers"
	"net/http"
	"os"
	"os/exec"
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
	s.poseidonCancel()
	<-time.After(tests.ShortTimeout)
	ctx, cancelPoseidon := context.WithCancel(context.Background())
	s.poseidonCancel = cancelPoseidon
	startPoseidon(ctx, cancelPoseidon)
	waitForPoseidon()
}

func TearDown() {
	path := helpers.BuildURL(api.BasePath, api.EnvironmentsPath, tests.DefaultEnvironmentIDAsString)
	_, err := helpers.HTTPDelete(path, nil)
	if err != nil {
		log.WithError(err).Fatal("Could not remove default environment")
	}
}

func startPoseidon(ctx context.Context, cancelPoseidon context.CancelFunc) {
	poseidon := exec.CommandContext(ctx, *poseidonBinary) //nolint:gosec // We accept that another binary can be executed.
	poseidon.Stdout = os.Stdout
	poseidon.Stderr = os.Stderr
	if err := poseidon.Start(); err != nil {
		cancelPoseidon()
		log.WithError(err).Fatal("Failed to start Poseidon")
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
