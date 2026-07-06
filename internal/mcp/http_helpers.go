package mcp

import (
	"encoding/json"
	"errors"
	"net/http"
)

const (
	quickStoreBodyLimitBytes  = 2 << 20
	quickRecallBodyLimitBytes = 512 << 10
	atomsBodyLimitBytes       = 2 << 20
)

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

func decodeLimitedJSON(w http.ResponseWriter, r *http.Request, dst any, limit int64) bool {
	r.Body = http.MaxBytesReader(w, r.Body, limit)
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			writeJSONError(w, http.StatusRequestEntityTooLarge, "request body too large")
			return false
		}
		writeJSONError(w, http.StatusBadRequest, "invalid request body")
		return false
	}
	return true
}
