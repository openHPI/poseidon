package e2e

import (
	"encoding/json"
	"github.com/stretchr/testify/assert"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/api"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/api/dto"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/tests/e2e/helpers"
	"io"
	"net/http"
	"strings"
	"testing"
)

func (suite *E2ETestSuite) TestProvideRunnerRoute() {
	runnerRequestString, _ := json.Marshal(dto.RunnerRequest{})
	reader := strings.NewReader(string(runnerRequestString))
	resp, err := http.Post(helpers.BuildURL(api.RouteBase, api.RouteRunners), "application/json", reader)
	suite.NoError(err)
	suite.Equal(http.StatusOK, resp.StatusCode, "The response code should be ok")

	runnerResponse := new(dto.RunnerResponse)
	err = json.NewDecoder(resp.Body).Decode(runnerResponse)
	suite.NoError(err)

	suite.True(runnerResponse.Id != "", "The response contains a runner id")
}

func newRunnerId(t *testing.T) string {
	runnerRequestString, _ := json.Marshal(dto.RunnerRequest{})
	reader := strings.NewReader(string(runnerRequestString))
	resp, err := http.Post(helpers.BuildURL(api.RouteBase, api.RouteRunners), "application/json", reader)
	assert.NoError(t, err)
	runnerResponse := new(dto.RunnerResponse)
	_ = json.NewDecoder(resp.Body).Decode(runnerResponse)
	return runnerResponse.Id
}

func (suite *E2ETestSuite) TestDeleteRunnerRoute() {
	runnerId := newRunnerId(suite.T())
	suite.NotEqual("", runnerId)

	suite.Run("Deleting the runner returns NoContent", func() {
		resp, err := httpDelete(helpers.BuildURL(api.RouteBase, api.RouteRunners, "/", runnerId), nil)
		suite.NoError(err)
		suite.Equal(http.StatusNoContent, resp.StatusCode)
	})

	suite.Run("Deleting it again returns NotFound", func() {
		resp, err := httpDelete(helpers.BuildURL(api.RouteBase, api.RouteRunners, "/", runnerId), nil)
		suite.NoError(err)
		suite.Equal(http.StatusNotFound, resp.StatusCode)
	})

	suite.Run("Deleting non-existing runner returns NotFound", func() {
		resp, err := httpDelete(helpers.BuildURL(api.RouteBase, api.RouteRunners, "/", "n0n-3x1st1ng-1d"), nil)
		suite.NoError(err)
		suite.Equal(http.StatusNotFound, resp.StatusCode)
	})
}

// HttpDelete sends a Delete Http Request with body to the passed url.
func httpDelete(url string, body io.Reader) (response *http.Response, err error) {
	req, _ := http.NewRequest(http.MethodDelete, url, body)
	client := &http.Client{}
	return client.Do(req)
}
