package api

import (
	"encoding/json"
	"fmt"
	"github.com/openHPI/poseidon/pkg/dto"
	"net/http"
)

func writeInternalServerError(writer http.ResponseWriter, err error, errorCode dto.ErrorCode) {
	sendJSON(writer, &dto.InternalServerError{Message: err.Error(), ErrorCode: errorCode}, http.StatusInternalServerError)
}

func writeBadRequest(writer http.ResponseWriter, err error) {
	sendJSON(writer, &dto.ClientError{Message: err.Error()}, http.StatusBadRequest)
}

func writeNotFound(writer http.ResponseWriter, err error) {
	sendJSON(writer, &dto.ClientError{Message: err.Error()}, http.StatusNotFound)
}

func sendJSON(writer http.ResponseWriter, content interface{}, httpStatusCode int) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(httpStatusCode)
	response, err := json.Marshal(content)
	if err != nil {
		// cannot produce infinite recursive loop, since json.Marshal of dto.InternalServerError won't return an error
		writeInternalServerError(writer, err, dto.ErrorUnknown)
		return
	}
	if _, err = writer.Write(response); err != nil {
		log.WithError(err).Error("Could not write JSON response")
		http.Error(writer, err.Error(), http.StatusInternalServerError)
	}
}

func parseJSONRequestBody(writer http.ResponseWriter, request *http.Request, structure interface{}) error {
	if err := json.NewDecoder(request.Body).Decode(structure); err != nil {
		writeBadRequest(writer, err)
		return fmt.Errorf("error parsing JSON request body: %w", err)
	}
	return nil
}
