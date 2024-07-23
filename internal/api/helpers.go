package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/openHPI/poseidon/pkg/dto"
)

func writeInternalServerError(ctx context.Context, writer http.ResponseWriter, err error, errorCode dto.ErrorCode) {
	sendJSON(ctx, writer, &dto.InternalServerError{Message: err.Error(), ErrorCode: errorCode}, http.StatusInternalServerError)
}

func writeClientError(ctx context.Context, writer http.ResponseWriter, err error, status uint16) {
	sendJSON(ctx, writer, &dto.ClientError{Message: err.Error()}, int(status))
}

func sendJSON(ctx context.Context, writer http.ResponseWriter, content interface{}, httpStatusCode int) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(httpStatusCode)
	response, err := json.Marshal(content)
	if err != nil {
		// cannot produce infinite recursive loop, since json.Marshal of dto.InternalServerError won't return an error
		writeInternalServerError(ctx, writer, err, dto.ErrorUnknown)
		return
	}
	if _, err = writer.Write(response); err != nil {
		log.WithError(err).WithContext(ctx).Error("Could not write JSON response")
		http.Error(writer, err.Error(), http.StatusInternalServerError)
	}
}

func parseJSONRequestBody(writer http.ResponseWriter, request *http.Request, structure interface{}) error {
	if err := json.NewDecoder(request.Body).Decode(structure); err != nil {
		writeClientError(request.Context(), writer, err, http.StatusBadRequest)
		return fmt.Errorf("error parsing JSON request body: %w", err)
	}
	return nil
}
