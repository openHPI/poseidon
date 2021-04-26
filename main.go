package main

import (
	"gitlab.hpi.de/codeocean/codemoon/coolcodeoceannomadmiddleware/api"
	"log"
	"net/http"
)

func main() {
	err := http.ListenAndServe("0.0.0.0:4000", api.NewRouter())
	if err != nil {
		log.Fatal("Error during listening and serving: ", err)
	}
}
