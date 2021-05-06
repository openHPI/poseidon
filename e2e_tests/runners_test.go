package e2e_tests

import (
	"encoding/json"
	"github.com/stretchr/testify/assert"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/api"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/api/dto"
	"net/http"
	"strings"
	"testing"
)

func TestProvideRunnerRoute(t *testing.T) {
	reader := strings.NewReader(`{"executionEnvironmentId":0}`)
	resp, err := http.Post(buildURL(api.RouteRunners), "application/json", reader)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode, "The response code should be ok")

	runnerResponse := new(dto.RunnerResponse)
	err = json.NewDecoder(resp.Body).Decode(runnerResponse)
	assert.NoError(t, err)

	assert.True(t, runnerResponse.Id != "", "The response contains a runner id")
}
