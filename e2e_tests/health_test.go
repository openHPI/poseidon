package e2e_tests

import (
	"github.com/stretchr/testify/assert"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/api"
	"net/http"
	"testing"
)

func TestHealthRoute(t *testing.T) {
	resp, err := http.Get(buildURL(api.RouteHealth))
	if assert.NoError(t, err) {
		assert.Equal(t, http.StatusNoContent, resp.StatusCode, "The response code should be NoContent")
	}
}
