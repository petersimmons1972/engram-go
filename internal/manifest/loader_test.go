package manifest_test

import (
	"fmt"
	"os"
	"testing"

	"github.com/petersimmons1972/engram/internal/manifest"
)

func writeYAML(t *testing.T, content string) string {
	t.Helper()
	f, _ := os.CreateTemp(t.TempDir(), "*.yaml")
	f.WriteString(content)
	f.Close()
	return f.Name()
}

func TestLoad_Valid(t *testing.T) {
	path := writeYAML(t, `
models:
  - name: mistral:7b
    params_b: 7.2
    vram_gb: 4.5
    tier: "4-6GB"
    vendor: "Mistral AI"
    family: mistral
    instruct: true
    include: true
    rationale: "Fast and accurate."
    notes: null
`)
	m, err := manifest.Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(m.Included()) != 1 {
		t.Fatalf("want 1 included model, got %d", len(m.Included()))
	}
}

func TestLoad_RejectsBaseModel(t *testing.T) {
	path := writeYAML(t, `
models:
  - name: mistral:7b-base
    params_b: 7.2
    vram_gb: 4.5
    tier: "4-6GB"
    vendor: "Mistral AI"
    family: mistral
    instruct: false
    include: true
    rationale: "Base model."
    notes: null
`)
	_, err := manifest.Load(path)
	if err == nil {
		t.Fatal("expected error for non-instruct model, got nil")
	}
}

func TestLoad_RejectsDuplicateNames(t *testing.T) {
	path := writeYAML(t, `
models:
  - name: mistral:7b
    params_b: 7.2
    vram_gb: 4.5
    tier: "4-6GB"
    vendor: "Mistral AI"
    family: mistral
    instruct: true
    include: true
    rationale: "First."
    notes: null
  - name: mistral:7b
    params_b: 7.2
    vram_gb: 4.5
    tier: "4-6GB"
    vendor: "Mistral AI"
    family: mistral
    instruct: true
    include: true
    rationale: "Duplicate."
    notes: null
`)
	_, err := manifest.Load(path)
	if err == nil {
		t.Fatal("expected error for duplicate model name, got nil")
	}
}

func TestLoad_RejectsVRAMExceededIfIncluded(t *testing.T) {
	path := writeYAML(t, `
models:
  - name: bigmodel:27b
    params_b: 27.8
    vram_gb: 17.0
    tier: "excluded"
    vendor: "Someone"
    family: big
    instruct: true
    include: true
    rationale: "Too big."
    notes: null
`)
	_, err := manifest.Load(path)
	if err == nil {
		t.Fatal("expected error for included model exceeding 16GB VRAM, got nil")
	}
}

func TestLoad_AllowsVRAMExceededIfExcluded(t *testing.T) {
	path := writeYAML(t, `
models:
  - name: bigmodel:27b
    params_b: 27.8
    vram_gb: 17.0
    tier: "excluded"
    vendor: "Someone"
    family: big
    instruct: true
    include: false
    rationale: "Too big."
    notes: null
`)
	m, err := manifest.Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(m.Excluded()) != 1 {
		t.Fatalf("want 1 excluded model, got %d", len(m.Excluded()))
	}
}

func TestLoad_RejectsTooManyIncluded(t *testing.T) {
	var content string
	content = "models:\n"
	for i := 0; i < 26; i++ {
		content += fmt.Sprintf(`  - name: model:%d
    params_b: 7.0
    vram_gb: 4.5
    tier: "4-6GB"
    vendor: "V"
    family: f
    instruct: true
    include: true
    rationale: "x"
    notes: null
`, i)
	}
	path := writeYAML(t, content)
	_, err := manifest.Load(path)
	if err == nil {
		t.Fatal("expected error for >25 included models, got nil")
	}
}

func TestIncluded_ReturnsOnlyIncludedModels(t *testing.T) {
	path := writeYAML(t, `
models:
  - name: a:7b
    params_b: 7
    vram_gb: 4
    tier: "4-6GB"
    vendor: "V"
    family: a
    instruct: true
    include: true
    rationale: "good"
  - name: b:13b
    params_b: 13
    vram_gb: 8
    tier: "7-10GB"
    vendor: "V"
    family: b
    instruct: true
    include: false
    rationale: "excluded"
`)
	m, err := manifest.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	inc := m.Included()
	if len(inc) != 1 {
		t.Fatalf("want 1 included, got %d", len(inc))
	}
	if inc[0].Name != "a:7b" {
		t.Errorf("want a:7b, got %q", inc[0].Name)
	}
}

func TestExcluded_ReturnsOnlyExcludedModels(t *testing.T) {
	path := writeYAML(t, `
models:
  - name: a:7b
    params_b: 7
    vram_gb: 4
    tier: "4-6GB"
    vendor: "V"
    family: a
    instruct: true
    include: true
    rationale: "good"
  - name: b:13b
    params_b: 13
    vram_gb: 8
    tier: "7-10GB"
    vendor: "V"
    family: b
    instruct: true
    include: false
    rationale: "excluded"
`)
	m, err := manifest.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	exc := m.Excluded()
	if len(exc) != 1 {
		t.Fatalf("want 1 excluded, got %d", len(exc))
	}
	if exc[0].Name != "b:13b" {
		t.Errorf("want b:13b, got %q", exc[0].Name)
	}
}

func TestIncluded_EmptyWhenAllExcluded(t *testing.T) {
	path := writeYAML(t, `
models:
  - name: a:7b
    params_b: 7
    vram_gb: 4
    tier: "4-6GB"
    vendor: "V"
    family: a
    instruct: true
    include: false
    rationale: "no"
`)
	m, err := manifest.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(m.Included()) != 0 {
		t.Errorf("want empty included list")
	}
	if len(m.Excluded()) != 1 {
		t.Errorf("want 1 excluded, got %d", len(m.Excluded()))
	}
}
