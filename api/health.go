package api

import (
	"net/http"
)

// Health handles the health route.
// It responds that the server is alive.
// If it is not, the response won't reach the client.
func Health(writer http.ResponseWriter, _ *http.Request) {
	writer.WriteHeader(http.StatusNoContent)
}
