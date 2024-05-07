package e2e

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/openHPI/poseidon/internal/api"
	"github.com/openHPI/poseidon/pkg/dto"
	"github.com/openHPI/poseidon/pkg/logging"
	"github.com/openHPI/poseidon/tests"
	"github.com/openHPI/poseidon/tests/helpers"
)

var log = logging.GetLogger("e2e-helpers")

func CreateDefaultEnvironment(prewarmingPoolSize uint, image string) dto.ExecutionEnvironmentRequest {
	path := helpers.BuildURL(api.BasePath, api.EnvironmentsPath, tests.DefaultEnvironmentIDAsString)
	const smallCPULimit uint = 20
	const smallMemoryLimit uint = 100

	defaultNomadEnvironment := dto.ExecutionEnvironmentRequest{
		PrewarmingPoolSize: prewarmingPoolSize,
		CPULimit:           smallCPULimit,
		MemoryLimit:        smallMemoryLimit,
		Image:              image,
		NetworkAccess:      false,
		ExposedPorts:       nil,
	}

	resp, err := helpers.HTTPPutJSON(path, defaultNomadEnvironment)
	if err != nil || resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusNoContent {
		log.WithError(err).Fatal("Couldn't create default environment for e2e tests")
	}
	err = resp.Body.Close()
	if err != nil {
		log.Fatal("Failed closing body")
	}
	return defaultNomadEnvironment
}

func WaitForDefaultEnvironment() {
	path := helpers.BuildURL(api.BasePath, api.RunnersPath)
	body := &dto.RunnerRequest{
		ExecutionEnvironmentID: tests.DefaultEnvironmentIDAsInteger,
		InactivityTimeout:      1,
	}
	var code int
	const maxRetries = 60
	for count := 0; count < maxRetries && code != http.StatusOK; count++ {
		<-time.After(time.Second)
		resp, err := helpers.HTTPPostJSON(path, body)
		if err == nil {
			code = resp.StatusCode
			log.WithField("count", count).WithField("statusCode", code).Info("Waiting for idle runners")
		} else {
			log.WithField("count", count).WithError(err).Warn("Waiting for idle runners")
		}
		_ = resp.Body.Close()
	}
	if code != http.StatusOK {
		log.Fatal("Failed to provide a runner")
	}
}

// ProvideRunner creates a runner with the given RunnerRequest via an external request.
// It needs a running Poseidon instance to work.
func ProvideRunner(request *dto.RunnerRequest) (string, error) {
	runnerURL := helpers.BuildURL(api.BasePath, api.RunnersPath)
	resp, err := helpers.HTTPPostJSON(runnerURL, request)
	if err != nil {
		return "", fmt.Errorf("cannot post provide runner: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		//nolint:goerr113 // dynamic error is ok in here, as it is a test
		return "", fmt.Errorf("expected response code 200 when getting runner, got %v", resp.StatusCode)
	}
	runnerResponse := new(dto.RunnerResponse)
	err = json.NewDecoder(resp.Body).Decode(runnerResponse)
	if err != nil {
		return "", fmt.Errorf("cannot decode runner response: %w", err)
	}
	_ = resp.Body.Close()
	return runnerResponse.ID, nil
}

// ProvideWebSocketURL creates a WebSocket endpoint from the ExecutionRequest via an external api request.
// It requires a running Poseidon instance.
func ProvideWebSocketURL(runnerID string, request *dto.ExecutionRequest) (string, error) {
	url := helpers.BuildURL(api.BasePath, api.RunnersPath, runnerID, api.ExecutePath)
	resp, err := helpers.HTTPPostJSON(url, request)
	if err != nil {
		return "", fmt.Errorf("cannot post provide websocket url: %w", err)
	} else if resp.StatusCode != http.StatusOK {
		return "", dto.ErrMissingData
	}

	executionResponse := new(dto.ExecutionResponse)
	err = json.NewDecoder(resp.Body).Decode(executionResponse)
	if err != nil {
		return "", fmt.Errorf("cannot parse execution response: %w", err)
	}
	_ = resp.Body.Close()
	return executionResponse.WebSocketURL, nil
}
