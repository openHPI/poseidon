package api

import (
	"context"
	"errors"
	"fmt"
	"github.com/coreos/go-systemd/v22/daemon"
	"github.com/openHPI/poseidon/internal/config"
	"github.com/openHPI/poseidon/internal/environment"
	"github.com/openHPI/poseidon/pkg/dto"
	"net/http"
	"os"
	"runtime/pprof"
	"strings"
	"time"
)

var ErrorPrewarmingPoolDepleting = errors.New("the prewarming pool is depleting")

// Health handles the health route.
// It responds that the server is alive.
// If it is not, the response won't reach the client.
func Health(manager environment.Manager) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		ctx, cancel := context.WithCancel(request.Context())
		defer cancel()
		go debugGoroutines(ctx)

		if err := checkPrewarmingPool(manager); err != nil {
			sendJSON(writer, &dto.InternalServerError{Message: err.Error(), ErrorCode: dto.PrewarmingPoolDepleting},
				http.StatusServiceUnavailable, request.Context())
			return
		}

		writer.WriteHeader(http.StatusNoContent)
	}
}

func checkPrewarmingPool(manager environment.Manager) error {
	var depletingEnvironments []int
	for _, data := range manager.Statistics() {
		if float64(data.IdleRunners)/float64(data.PrewarmingPoolSize) < config.Config.Server.Alert.PrewarmingPoolThreshold {
			depletingEnvironments = append(depletingEnvironments, data.ID)
		}
	}
	if len(depletingEnvironments) > 0 {
		arrayToString := strings.Trim(strings.Join(strings.Fields(fmt.Sprint(depletingEnvironments)), ", "), "[]")
		return fmt.Errorf("%w: environments %s", ErrorPrewarmingPoolDepleting, arrayToString)
	}
	return nil
}

// debugGoroutines temporarily debugs a behavior where we observe long latencies in the Health route.
func debugGoroutines(ctx context.Context) {
	interval, err := daemon.SdWatchdogEnabled(false)
	if err != nil || interval == 0 {
		return
	}
	log.Trace("Starting timeout for debugging the Goroutines")

	const notificationIntervalFactor = 3
	select {
	case <-ctx.Done():
		return
	case <-time.After(interval / notificationIntervalFactor):
		log.Warn("Health route latency is too high")
		err := pprof.Lookup("goroutine").WriteTo(os.Stderr, 1)
		if err != nil {
			log.WithError(err).Warn("Failed to log the goroutines")
		}
	}
}
