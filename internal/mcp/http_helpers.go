package mcp

import (
	"encoding/json"
	"errors"
	"net/http"
)

var errRequestBodyTooLarge = errors.New("request body too large")

// writeJSON sets Content-Type to application/json, writes status, and encodes v.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v) //nolint:errcheck
}

// writeJSONError writes a JSON {"error": msg} response with the given status.
func writeJSONError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// decodeJSONBodyBounded decodes a JSON request body up to maxBytes. It returns
// errRequestBodyTooLarge when the body exceeds the configured limit.
func decodeJSONBodyBounded(w http.ResponseWriter, r *http.Request, maxBytes int64, dst any) error {
	body := http.MaxBytesReader(w, r.Body, maxBytes)
	defer func() { _ = body.Close() }()

	if err := json.NewDecoder(body).Decode(dst); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			return errRequestBodyTooLarge
		}
		return err
	}
	return nil
}
