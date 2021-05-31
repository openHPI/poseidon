package e2e

import (
	"github.com/stretchr/testify/assert"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/api"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/tests/helpers"
	"net/http"
	"testing"
)

func TestHealthRoute(t *testing.T) {
	resp, err := http.Get(helpers.BuildURL(api.BasePath, api.HealthPath))
	if assert.NoError(t, err) {
		assert.Equal(t, http.StatusNoContent, resp.StatusCode, "The response code should be NoContent")
	}
}
