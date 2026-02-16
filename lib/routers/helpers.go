package routers

import (
	"encoding/json"
	"net/http"
)

type ErrResponse struct {
	Message string `json:"error"` 
}

func RespondJSON(responseWriter http.ResponseWriter, statusCode int, payload any) {
	responseWriter.Header().Set("Content-Type", "application/json")
	responseWriter.WriteHeader(statusCode)
	json.NewEncoder(responseWriter).Encode(payload)
}

func RespondError(responseWriter http.ResponseWriter, statusCode int, errorMessage string) {
	RespondJSON(responseWriter, statusCode, map[string]string{"error": errorMessage})
}