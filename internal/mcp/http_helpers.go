package mcp

import (
	"encoding/json"
	"net/http"
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
