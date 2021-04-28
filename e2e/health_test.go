package e2e

import (
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/api"
	"net/http"
	"testing"
)

func TestHealthRoute(t *testing.T) {
	resp, err := http.Get(buildURL(api.RouteHealth))
	require.NoError(t, err)
	assert.Equal(t, http.StatusNoContent, resp.StatusCode, "The response code should be NoContent")
}
