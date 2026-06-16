package main

import (
	"os"
	"strings"
	"testing"
)

func TestDualPreferenceRecallFlag_DefaultFalse(t *testing.T) {
	src, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatalf("read main.go: %v", err)
	}
	if !strings.Contains(string(src), `"dual-preference-recall", false`) {
		t.Fatal("main.go: --dual-preference-recall must default to false")
	}
}
