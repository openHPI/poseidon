package api

import (
	"encoding/json"
	"log"
	"net/http"
)

// Health sends the response that the API works. If it
// it is able to do so, it is obviously correct.
func Health(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Header().Set("Content-Type", "application/json")
	response, err := json.Marshal(Message{"I'm alive!"})
	if err != nil {
		log.Printf("Error formatting the health route: %v\n", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
	if _, err := w.Write(response); err != nil {
		log.Printf("Error handling the health route: %v\n", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
