package api

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/openHPI/poseidon/pkg/dto"
	"net/http"
)

func writeInternalServerError(writer http.ResponseWriter, err error, errorCode dto.ErrorCode, ctx context.Context) {
	sendJSON(writer, &dto.InternalServerError{Message: err.Error(), ErrorCode: errorCode},
		http.StatusInternalServerError, ctx)
}

func writeClientError(writer http.ResponseWriter, err error, status uint16, ctx context.Context) {
	sendJSON(writer, &dto.ClientError{Message: err.Error()}, int(status), ctx)
}

func sendJSON(writer http.ResponseWriter, content interface{}, httpStatusCode int, ctx context.Context) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(httpStatusCode)
	response, err := json.Marshal(content)
	if err != nil {
		// cannot produce infinite recursive loop, since json.Marshal of dto.InternalServerError won't return an error
		writeInternalServerError(writer, err, dto.ErrorUnknown, ctx)
		return
	}
	if _, err = writer.Write(response); err != nil {
		log.WithError(err).WithContext(ctx).Error("Could not write JSON response")
		http.Error(writer, err.Error(), http.StatusInternalServerError)
	}
}

func parseJSONRequestBody(writer http.ResponseWriter, request *http.Request, structure interface{}) error {
	if err := json.NewDecoder(request.Body).Decode(structure); err != nil {
		writeClientError(writer, err, http.StatusBadRequest, request.Context())
		return fmt.Errorf("error parsing JSON request body: %w", err)
	}
	return nil
}
