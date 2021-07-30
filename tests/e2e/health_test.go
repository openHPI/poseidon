package e2e

import (
	"github.com/openHPI/poseidon/internal/api"
	"github.com/openHPI/poseidon/tests/helpers"
	"github.com/stretchr/testify/assert"
	"net/http"
	"testing"
)

func TestHealthRoute(t *testing.T) {
	resp, err := http.Get(helpers.BuildURL(api.BasePath, api.HealthPath))
	if assert.NoError(t, err) {
		assert.Equal(t, http.StatusNoContent, resp.StatusCode, "The response code should be NoContent")
	}
}
