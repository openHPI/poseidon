package api

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/openHPI/poseidon/internal/config"
	"github.com/openHPI/poseidon/internal/environment"
	"github.com/openHPI/poseidon/pkg/dto"
)

var ErrorPrewarmingPoolDepleting = errors.New("the prewarming pool is depleting")

// Health handles the health route.
// It responds that the server is alive.
// If it is not, the response won't reach the client.
func Health(manager environment.Manager) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
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
