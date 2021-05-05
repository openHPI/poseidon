package e2e

import (
	"gitlab.hpi.de/codeocean/codemoon/poseidon/api"
	"gitlab.hpi.de/codeocean/codemoon/poseidon/config"
	"strings"
)

/*
* # E2E Tests
*
* For the e2e tests a nomad cluster must be connected and poseidon must be running.
 */

var baseURL = config.Config.PoseidonAPIURL().String()

func buildURL(parts ...string) (url string) {
	parts = append([]string{baseURL, api.RouteBase}, parts...)
	return strings.Join(parts, "")
}
