package api

import (
	"encoding/json"
	"github.com/gorilla/mux"
	"log"
	"net/http"
)

// NewRouter returns an HTTP handler (http.Handler) which can be
// used by the net/http package to serve the routes of our API. It
// always returns a router for the newest version of our API. We
// use gorilla/mux because it is more convenient than net/http, e.g.
// when extracting path parameters.
func NewRouter() *mux.Router {
	router := mux.NewRouter()
	// this can later be restricted to a specific host with
	// `router.Host(...)` and to HTTPS with `router.Schemes("https")`
	return newRouterV1(router)
}

// newRouterV1 returns a subrouter containing the routes of version
// 1 of our API.
func newRouterV1(router *mux.Router) *mux.Router {
	v1 := router.PathPrefix("/api/v1").Subrouter()
	v1.HandleFunc("/health", Health).Methods(http.MethodGet)
	return v1
}

func writeJson(writer http.ResponseWriter, content interface{}) {
	writer.Header().Set("Content-Type", "application/json")
	response, err := json.Marshal(content)
	if err != nil {
		log.Printf("JSON marshal error: %v\n", err)
		http.Error(writer, err.Error(), http.StatusInternalServerError)
		return
	}
	if _, err := writer.Write(response); err != nil {
		log.Printf("Error writing JSON to response: %v\n", err)
		http.Error(writer, err.Error(), http.StatusInternalServerError)
	}
}
