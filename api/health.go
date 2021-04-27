package api

import (
	"encoding/json"
	"log"
	"net/http"
)

// Health tries to respond that the server is alive.
// If it is not, the response won't reach the client.
func Health(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Header().Set("Content-Type", "application/json")
	response, err := json.Marshal(Message{"I'm alive!"})
	if err != nil {
		log.Printf("JSON marshal error in health route: %v\n", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
	if _, err := w.Write(response); err != nil {
		log.Printf("Error handling the health route: %v\n", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
