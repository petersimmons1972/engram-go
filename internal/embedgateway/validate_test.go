package embedgateway

import (
	"strings"
	"testing"

	"github.com/petersimmons1972/engram/internal/embedmodel"
)

func TestValidateEmbedResponse_AcceptsAllAliases(t *testing.T) {
	vec := make([]float32, embedmodel.RequiredDims)
	for _, alias := range embedmodel.AcceptedAliases {
		if err := validateEmbedResponse(vec, alias); err != nil {
			t.Fatalf("validateEmbedResponse(%q) returned error: %v", alias, err)
		}
	}
}

func TestValidateEmbedResponse_RejectsWrongModel(t *testing.T) {
	err := validateEmbedResponse(make([]float32, embedmodel.RequiredDims), "nomic-embed-text")
	if err == nil {
		t.Fatal("validateEmbedResponse returned nil for wrong model")
	}
	if !strings.Contains(err.Error(), "not in bge-m3 alias set") {
		t.Fatalf("error = %q, want alias-set rejection", err)
	}
}

func TestValidateEmbedResponse_RejectsWrongDims(t *testing.T) {
	err := validateEmbedResponse(make([]float32, 768), embedmodel.CanonicalBGEM3)
	if err == nil {
		t.Fatal("validateEmbedResponse returned nil for wrong dims")
	}
	if !strings.Contains(err.Error(), "got 768 dims") {
		t.Fatalf("error = %q, want dim rejection", err)
	}
}

func TestValidateEmbedResponse_RejectsBothWrong(t *testing.T) {
	err := validateEmbedResponse(make([]float32, 768), "nomic-embed-text")
	if err == nil {
		t.Fatal("validateEmbedResponse returned nil for wrong model and dims")
	}
	if !strings.Contains(err.Error(), "not in bge-m3 alias set") {
		t.Fatalf("error = %q, want model rejection first", err)
	}
}
