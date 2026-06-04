package serve

import (
	"encoding/json"
	"net/http"
)

// jsonErrorBody mirrors the ErrorResponse shape the hosted query service
// returns. The widget's client runs every non-streaming response through
// checkApiError, which parses `error.message` out of the JSON body; a plain
// text body (e.g. from http.Error) would be discarded, losing the message.
type jsonErrorBody struct {
	Error   jsonErrorMessage `json:"error"`
	Success bool             `json:"success"`
}

type jsonErrorMessage struct {
	Message string `json:"message"`
}

// writeJSONError writes a structured JSON error body with the given HTTP
// status, so the widget can surface the message to the user.
func writeJSONError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(jsonErrorBody{
		Error:   jsonErrorMessage{Message: message},
		Success: false,
	})
}
