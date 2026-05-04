package embed

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestPermanentErrorJSONShapeAndUnwrap(t *testing.T) {
	wrappedErr := errors.New("original context")
	pe := &PermanentError{
		Code:        "EMBED_TIMEOUT",
		Stored:      "2024-01-15T10:30:00Z",
		Current:     "2024-01-15T10:35:00Z",
		Remediation: "Retry with exponential backoff",
		Wrapped:     wrappedErr,
	}

	// Test 1: Error() string format
	errStr := pe.Error()
	if errStr == "" {
		t.Fatal("Error() returned empty string")
	}
	t.Logf("Error() output: %s", errStr)

	// Test 2: errors.Is() with ErrPermanent sentinel
	if !errors.Is(pe, ErrPermanent) {
		t.Errorf("errors.Is(pe, ErrPermanent) = false; want true")
	}

	// Test 3: errors.Is() with wrapped error
	if !errors.Is(pe, wrappedErr) {
		t.Errorf("errors.Is(pe, wrappedErr) = false; want true")
	}

	// Test 4: MarshalJSON produces exact shape
	data, err := json.Marshal(pe)
	if err != nil {
		t.Fatalf("MarshalJSON() error = %v", err)
	}

	// Unmarshal into a map to verify shape and order
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("Unmarshal result error = %v", err)
	}

	// Verify exact keys present
	expectedKeys := []string{"code", "stored", "current", "remediation"}
	for _, key := range expectedKeys {
		if _, ok := m[key]; !ok {
			t.Errorf("JSON missing key %q", key)
		}
	}

	// Verify no extra keys
	if len(m) != 4 {
		t.Errorf("JSON has %d keys; want exactly 4. Keys: %v", len(m), m)
	}

	// Verify values
	if m["code"] != "EMBED_TIMEOUT" {
		t.Errorf("JSON code = %v; want %q", m["code"], "EMBED_TIMEOUT")
	}
	if m["stored"] != "2024-01-15T10:30:00Z" {
		t.Errorf("JSON stored = %v; want %q", m["stored"], "2024-01-15T10:30:00Z")
	}
	if m["current"] != "2024-01-15T10:35:00Z" {
		t.Errorf("JSON current = %v; want %q", m["current"], "2024-01-15T10:35:00Z")
	}
	if m["remediation"] != "Retry with exponential backoff" {
		t.Errorf("JSON remediation = %v; want %q", m["remediation"], "Retry with exponential backoff")
	}

	// Test 5: Unwrap() returns correct errors
	unwrapped := pe.Unwrap()
	if len(unwrapped) != 2 {
		t.Fatalf("Unwrap() returned %d errors; want 2", len(unwrapped))
	}
	if unwrapped[0] != ErrPermanent {
		t.Errorf("Unwrap()[0] = %v; want ErrPermanent", unwrapped[0])
	}
	if unwrapped[1] != wrappedErr {
		t.Errorf("Unwrap()[1] = %v; want %v", unwrapped[1], wrappedErr)
	}
}

func TestPermanentErrorNilWrapped(t *testing.T) {
	pe := &PermanentError{
		Code:        "EMBED_INVALID",
		Stored:      "2024-01-15T10:30:00Z",
		Current:     "2024-01-15T10:35:00Z",
		Remediation: "Check input format",
		Wrapped:     nil,
	}

	// Test 1: errors.Is() with sentinel still works
	if !errors.Is(pe, ErrPermanent) {
		t.Errorf("errors.Is(pe, ErrPermanent) = false; want true with nil Wrapped")
	}

	// Test 2: Unwrap() filters out nil Wrapped
	unwrapped := pe.Unwrap()
	if len(unwrapped) != 1 {
		t.Fatalf("Unwrap() returned %d errors; want 1 (nil Wrapped filtered)", len(unwrapped))
	}
	if unwrapped[0] != ErrPermanent {
		t.Errorf("Unwrap()[0] = %v; want ErrPermanent", unwrapped[0])
	}

	// Test 3: JSON marshalling works with nil Wrapped
	data, err := json.Marshal(pe)
	if err != nil {
		t.Fatalf("MarshalJSON() error = %v", err)
	}

	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("Unmarshal result error = %v", err)
	}

	if len(m) != 4 {
		t.Errorf("JSON has %d keys; want exactly 4", len(m))
	}
}
