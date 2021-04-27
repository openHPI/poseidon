package api

import (
	"github.com/gorilla/mux"
	"net/http"
)

// NewRouter returns an HTTP handler (http.Handler) which can then
// be used by the net/http package to serve the api of our API.
// We use gorilla/mux because it is more convenient than net/http,
// e.g. when extracting path parameters.
func NewRouter() *mux.Router {
	router := mux.NewRouter()
	// this can later be restricted to a specific host with
	// `router.Host(...)` and to HTTPS with `router.Schemes("https")`
	v1 := router.PathPrefix("/api/v1").Subrouter()
	v1.HandleFunc("/health", Health).Methods(http.MethodGet)

	return v1
}
