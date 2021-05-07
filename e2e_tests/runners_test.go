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
	runnerRequestString, _ := json.Marshal(dto.RunnerRequest{})
	reader := strings.NewReader(string(runnerRequestString))
	resp, err := http.Post(buildURL(api.RouteRunners), "application/json", reader)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode, "The response code should be ok")

	runnerResponse := new(dto.RunnerResponse)
	err = json.NewDecoder(resp.Body).Decode(runnerResponse)
	assert.NoError(t, err)

	assert.True(t, runnerResponse.Id != "", "The response contains a runner id")
}

func newRunnerId(t *testing.T) string {
	runnerRequestString, _ := json.Marshal(dto.RunnerRequest{})
	reader := strings.NewReader(string(runnerRequestString))
	resp, err := http.Post(buildURL(api.RouteRunners), "application/json", reader)
	assert.NoError(t, err)
	runnerResponse := new(dto.RunnerResponse)
	_ = json.NewDecoder(resp.Body).Decode(runnerResponse)
	return runnerResponse.Id
}

func TestDeleteRunnerRoute(t *testing.T) {
	runnerId := newRunnerId(t)
	assert.NotEqual(t, "", runnerId)

	t.Run("Deleting the runner returns NoContent", func(t *testing.T) {
		resp, err := httpDelete(buildURL(api.RouteRunners, "/", runnerId), nil)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusNoContent, resp.StatusCode)
	})

	t.Run("Deleting it again returns NotFound", func(t *testing.T) {
		resp, err := httpDelete(buildURL(api.RouteRunners, "/", runnerId), nil)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	})

	t.Run("Deleting non-existing runner returns NotFound", func(t *testing.T) {
		resp, err := httpDelete(buildURL(api.RouteRunners, "/", "n0n-3x1st1ng-1d"), nil)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	})
}
