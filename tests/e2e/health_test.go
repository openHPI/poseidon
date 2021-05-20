package e2e

import (
	"github.com/stretchr/testify/assert"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/api"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/tests/e2e/helpers"
	"net/http"
	"testing"
)

func TestHealthRoute(t *testing.T) {
	resp, err := http.Get(helpers.BuildURL(api.RouteBase, api.RouteHealth))
	if assert.NoError(t, err) {
		assert.Equal(t, http.StatusNoContent, resp.StatusCode, "The response code should be NoContent")
	}
}
