package main

import "testing"

func TestOllaHealthCheck_unreachable(t *testing.T) {
	if ollaHealthCheck("http://127.0.0.1:19999/v1") {
		t.Error("unreachable endpoint must return false")
	}
}

func TestBuildPreserveSkipSet_preserveMode(t *testing.T) {
	labels := map[string]string{"q1": "CORRECT", "q2": "PARTIALLY_CORRECT", "q3": "INCORRECT"}
	skip, retry := buildPreserveSkipSet(labels, true)
	if !skip["q1"] {
		t.Error("CORRECT must be skipped")
	}
	if skip["q2"] {
		t.Error("PARTIALLY_CORRECT must not be skipped")
	}
	if !retry["q2"] {
		t.Error("PARTIALLY_CORRECT must be in retry set")
	}
	if !retry["q3"] {
		t.Error("INCORRECT must be in retry set")
	}
}

func TestBuildPreserveSkipSet_forceRescore(t *testing.T) {
	labels := map[string]string{"q1": "CORRECT"}
	skip, _ := buildPreserveSkipSet(labels, false)
	if skip["q1"] {
		t.Error("force-rescore must not skip CORRECT")
	}
}
