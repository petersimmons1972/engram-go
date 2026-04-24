# Model Evaluation Binary Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a Go binary (`cmd/benchmark`) that discovers, pulls, benchmarks, and documents Ollama models for pattern detection, producing `docs/models.md` and `docs/benchmark-results.json` in one unattended run.

**Architecture:** Manifest-driven: `models/candidates.yaml` lists candidates with metadata; the binary validates, pulls missing models, runs 3 inferences each, scores results, and writes documentation. Sequential execution (shared GPU). Result cache keyed on prompt+fixture+Ollama version+model digest.

**Tech Stack:** Go 1.22+, `gopkg.in/yaml.v3` (manifest), stdlib `net/http` (Ollama HTTP API), `text/template` (SVG generation), `go test` (unit tests).

---

## File Map

```
cmd/benchmark/main.go                    ← CLI entry point, orchestration
internal/manifest/
  loader.go                              ← parse + validate candidates.yaml
  loader_test.go
internal/vram/
  detect.go                              ← platform GPU probe (nvidia/amd/apple/fallback)
  detect_test.go
internal/ollama/
  client.go                              ← pull, list, show, chat, evict via HTTP
  client_test.go
  types.go                               ← request/response structs
internal/runner/
  runner.go                              ← per-model run orchestration
  runner_test.go
  thinking_markers.go                    ← leak detection token table
  thinking_markers_test.go
internal/scorer/
  scorer.go                              ← pure scoring function, no I/O
  scorer_test.go
internal/cache/
  cache.go                               ← sha256-keyed result cache (benchmark-results.json)
  cache_test.go
internal/reporter/
  markdown.go                            ← writes docs/models.md
  markdown_test.go
  svg.go                                 ← writes docs/assets/svg/model-tiers.svg
  svg_test.go
internal/types/
  types.go                               ← shared Duration, Score, RunResult, Verdict
models/
  candidates.yaml                        ← the manifest
testdata/
  sample.jsonl                           ← 40-event synthetic fixture (one of each pattern type)
docs/
  models.md                              ← generated
  benchmark-results.json                 ← generated + cache
  assets/svg/model-tiers.svg             ← generated
.github/workflows/benchmark.yml
go.mod
```

---

## Task 1: Go Module Scaffold

**Files:**
- Create: `go.mod`
- Create: `internal/types/types.go`

- [ ] **Step 1: Init module**

```bash
cd ~/projects/instinct
go mod init github.com/petersimmons1972/instinct
```

Expected: `go.mod` created with `module github.com/petersimmons1972/instinct` and `go 1.22`

- [ ] **Step 2: Add yaml dependency**

```bash
go get gopkg.in/yaml.v3
```

- [ ] **Step 3: Create shared types**

Create `internal/types/types.go`:

```go
package types

import (
	"encoding/json"
	"time"
)

// Duration wraps time.Duration with human-readable JSON serialisation.
type Duration time.Duration

func (d Duration) MarshalJSON() ([]byte, error) {
	return json.Marshal(time.Duration(d).String())
}

func (d *Duration) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	dur, err := time.ParseDuration(s)
	if err != nil {
		return err
	}
	*d = Duration(dur)
	return nil
}

func (d Duration) Std() time.Duration { return time.Duration(d) }

type Verdict string

const (
	VerdictRecommended    Verdict = "recommended"
	VerdictUsable         Verdict = "usable"
	VerdictNotRecommended Verdict = "not-recommended"
	VerdictFailed         Verdict = "failed"
	VerdictTimedOut       Verdict = "timeout"
	VerdictPullFailed     Verdict = "pull-failed"
	VerdictSkippedVRAM    Verdict = "skipped-vram"
)

type RunAttempt struct {
	Duration     Duration `json:"duration"`
	RawContent   string   `json:"raw_content"`
	ThinkingText string   `json:"thinking_text"`
	Error        string   `json:"error,omitempty"`
	TimedOut     bool     `json:"timed_out"`
}

type RunResult struct {
	Model        string       `json:"model"`
	ModelDigest  string       `json:"model_digest"`
	PullDuration Duration     `json:"pull_duration"`
	Runs         []RunAttempt `json:"runs"`
	CacheKey     string       `json:"cache_key"`
	Skipped      bool         `json:"skipped,omitempty"`
	SkipReason   string       `json:"skip_reason,omitempty"`
}

type Score struct {
	JSONValid     bool     `json:"json_valid"`
	PatternCount  int      `json:"pattern_count"`
	ValidPatterns int      `json:"valid_patterns"`
	QualityPct    float64  `json:"quality_pct"`
	AvgLatency    Duration `json:"avg_latency"`
	Composite     float64  `json:"composite"`
	ThinkingLeak  bool     `json:"thinking_leak"`
	Verdict       Verdict  `json:"verdict"`
	VerdictReason string   `json:"verdict_reason"`
}

type ModelResult struct {
	Model  string  `json:"model"`
	VRAMGB float64 `json:"vram_gb"`
	Tier   string  `json:"tier"`
	Vendor string  `json:"vendor"`
	Score  Score   `json:"score"`
}
```

- [ ] **Step 4: Verify it compiles**

```bash
cd ~/projects/instinct && go build ./internal/types/...
```

Expected: no output (success)

- [ ] **Step 5: Commit**

```bash
git add go.mod go.sum internal/types/types.go
git commit -m "feat: go module scaffold + shared types"
```

---

## Task 2: Manifest Loader

**Files:**
- Create: `internal/manifest/loader.go`
- Create: `internal/manifest/loader_test.go`
- Create: `models/candidates.yaml`

- [ ] **Step 1: Write failing tests**

Create `internal/manifest/loader_test.go`:

```go
package manifest_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/petersimmons1972/instinct/internal/manifest"
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
```

- [ ] **Step 2: Run tests — verify they fail**

```bash
cd ~/projects/instinct && go test ./internal/manifest/... 2>&1 | head -5
```

Expected: `cannot find package`

- [ ] **Step 3: Implement loader**

Create `internal/manifest/loader.go`:

```go
package manifest

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

const maxIncluded = 25

type Model struct {
	Name      string   `yaml:"name"`
	ParamsB   float64  `yaml:"params_b"`
	VRAMgb    float64  `yaml:"vram_gb"`
	Tier      string   `yaml:"tier"`
	Vendor    string   `yaml:"vendor"`
	Family    string   `yaml:"family"`
	Instruct  bool     `yaml:"instruct"`
	Include   bool     `yaml:"include"`
	Rationale string   `yaml:"rationale"`
	Notes     *string  `yaml:"notes"`
}

type Manifest struct {
	Models []Model `yaml:"models"`
}

func (m *Manifest) Included() []Model {
	var out []Model
	for _, model := range m.Models {
		if model.Include {
			out = append(out, model)
		}
	}
	return out
}

func (m *Manifest) Excluded() []Model {
	var out []Model
	for _, model := range m.Models {
		if !model.Include {
			out = append(out, model)
		}
	}
	return out
}

func Load(path string) (*Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading manifest: %w", err)
	}
	var m Manifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parsing manifest: %w", err)
	}

	seen := map[string]bool{}
	includedCount := 0
	for _, model := range m.Models {
		if seen[model.Name] {
			return nil, fmt.Errorf("duplicate model name: %q", model.Name)
		}
		seen[model.Name] = true

		if !model.Instruct && model.Include {
			return nil, fmt.Errorf("model %q: instruct must be true for included models (base models cannot follow structured prompts)", model.Name)
		}
		if model.VRAMgb > 16.0 && model.Include {
			return nil, fmt.Errorf("model %q: vram_gb %.1f exceeds 16GB limit for included models", model.Name, model.VRAMgb)
		}
		if model.Include {
			includedCount++
		}
	}
	if includedCount > maxIncluded {
		return nil, fmt.Errorf("manifest has %d included models (max %d); add models as include: false or raise the cap", includedCount, maxIncluded)
	}
	return &m, nil
}
```

- [ ] **Step 4: Fix missing fmt import in test**

Add `"fmt"` to imports in `loader_test.go`.

- [ ] **Step 5: Run tests — verify they pass**

```bash
cd ~/projects/instinct && go test ./internal/manifest/... -v 2>&1 | tail -10
```

Expected: `ok  github.com/petersimmons1972/instinct/internal/manifest`

- [ ] **Step 6: Create candidates.yaml**

Create `models/candidates.yaml`:

```yaml
# models/candidates.yaml
# Community contributions: submit a PR adding an entry below.
# Requirements: text-only instruct variant, vram_gb <= 16, not superseded by a newer family member.
# Max 25 include: true entries enforced at load time.

models:
  # ── ≤2GB tier ──────────────────────────────────────────────────────────────
  - name: llama3.2:1b
    params_b: 1.2
    vram_gb: 1.0
    tier: "≤2GB"
    vendor: "Meta"
    family: llama3
    instruct: true
    include: true
    rationale: >
      Ultra-small baseline. Useful for machines with integrated graphics or
      very limited VRAM. Sets the floor for what is possible.
    notes: null

  - name: llama3.2:3b
    params_b: 3.2
    vram_gb: 2.0
    tier: "≤2GB"
    vendor: "Meta"
    family: llama3
    instruct: true
    include: true
    rationale: >
      Small and fast. Recommended entry point for 4GB VRAM cards that need
      headroom for the host OS.
    notes: null

  - name: phi3.5:3.8b
    params_b: 3.8
    vram_gb: 2.5
    tier: "≤2GB"
    vendor: "Microsoft"
    family: phi3
    instruct: true
    include: true
    rationale: >
      Microsoft's compact instruct model. Competes with Llama 3.2 3B on
      instruction following at similar VRAM cost.
    notes: null

  # ── 4–6GB tier ─────────────────────────────────────────────────────────────
  - name: gemma3:4b
    params_b: 4.3
    vram_gb: 3.0
    tier: "4-6GB"
    vendor: "Google"
    family: gemma3
    instruct: true
    include: true
    rationale: >
      Google's 4B instruct model. Strong structured output relative to size.
    notes: null

  - name: mistral:7b
    params_b: 7.2
    vram_gb: 4.5
    tier: "4-6GB"
    vendor: "Mistral AI"
    family: mistral
    instruct: true
    include: true
    rationale: >
      Proven benchmark winner: the only model in our initial test to detect
      all four pattern types correctly across multiple runs. Recommended
      default for any machine with a 4GB card or larger.
    notes: null

  - name: llama3.1:8b
    params_b: 8.0
    vram_gb: 5.0
    tier: "4-6GB"
    vendor: "Meta"
    family: llama3
    instruct: true
    include: true
    rationale: >
      Meta's flagship 8B model. Widely deployed; good baseline for comparison
      against Mistral at the same tier.
    notes: null

  - name: deepseek-r1:8b
    params_b: 8.0
    vram_gb: 5.0
    tier: "4-6GB"
    vendor: "DeepSeek"
    family: deepseek-r1
    instruct: true
    include: true
    rationale: >
      Reasoning-focused model. Returned valid JSON but zero patterns in initial
      benchmark — included for community investigation of prompt tuning.
    notes: "Thinking mode active; chain-of-thought output may need prompt adaptation."

  - name: qwen2.5:7b
    params_b: 7.6
    vram_gb: 4.5
    tier: "4-6GB"
    vendor: "Alibaba"
    family: qwen2.5
    instruct: true
    include: true
    rationale: >
      Alibaba's general 7B model. Included alongside qwen2.5-coder:7b to
      compare general vs. code-tuned variants at identical VRAM cost.
    notes: null

  - name: qwen2.5-coder:7b
    params_b: 7.6
    vram_gb: 4.5
    tier: "4-6GB"
    vendor: "Alibaba"
    family: qwen2.5
    instruct: true
    include: true
    rationale: >
      Code-tuned variant. Pattern detection involves structured JSON output —
      a coder model may outperform the general variant on schema adherence.
    notes: null

  # ── 7–10GB tier ────────────────────────────────────────────────────────────
  - name: gemma3:12b
    params_b: 12.0
    vram_gb: 7.5
    tier: "7-10GB"
    vendor: "Google"
    family: gemma3
    instruct: true
    include: true
    rationale: >
      Google's mid-size model. Steps up from gemma3:4b with meaningfully more
      capacity for pattern reasoning.
    notes: null

  - name: mistral-nemo:12b
    params_b: 12.0
    vram_gb: 7.5
    tier: "7-10GB"
    vendor: "Mistral AI"
    family: mistral
    instruct: true
    include: true
    rationale: >
      Mistral's newer 12B model. Direct comparison to mistral:7b to evaluate
      whether the extra 5GB VRAM cost buys meaningful pattern detection gains.
    notes: null

  - name: phi4:14b
    params_b: 14.0
    vram_gb: 9.0
    tier: "7-10GB"
    vendor: "Microsoft"
    family: phi4
    instruct: true
    include: true
    rationale: >
      Microsoft's 14B flagship. Strong instruction following in published
      benchmarks; first-time test against the pattern detection task.
    notes: null

  - name: qwen2.5:14b
    params_b: 14.0
    vram_gb: 9.0
    tier: "7-10GB"
    vendor: "Alibaba"
    family: qwen2.5
    instruct: true
    include: true
    rationale: >
      Scales up from qwen2.5:7b. Evaluates whether doubling parameters
      improves pattern detection at the cost of VRAM.
    notes: null

  - name: deepseek-r1:14b
    params_b: 14.8
    vram_gb: 9.0
    tier: "7-10GB"
    vendor: "DeepSeek"
    family: deepseek-r1
    instruct: true
    include: true
    rationale: >
      Larger reasoning model. Zero patterns in initial benchmark — evaluation
      candidate only. Included to test whether extra capacity unlocks the
      chain-of-thought for structured output.
    notes: "Thinking mode active. Valid JSON produced but zero patterns detected in prior run."

  # ── Excluded (documented rejections) ───────────────────────────────────────
  - name: qwen3.5:27b
    params_b: 27.8
    vram_gb: 17.0
    tier: "excluded"
    vendor: "Alibaba"
    family: qwen3
    instruct: true
    include: false
    rationale: >
      Exceeds the 16GB VRAM limit. Cannot run safely on a 20GB card with
      host OS headroom. Excluded until a quantised variant fits the budget.
    notes: null

  - name: qwen3:8b
    params_b: 8.2
    vram_gb: 5.0
    tier: "excluded"
    vendor: "Alibaba"
    family: qwen3
    instruct: true
    include: false
    rationale: >
      Built-in thinking mode leaks thinking tokens into the JSON content field,
      producing invalid output regardless of prompt. The /no_think prefix was
      tested and confirmed ineffective. Excluded until Ollama resolves the
      format:json + thinking mode interaction.
    notes: "Thinking tokens leak into content field. format:json incompatible with Qwen 3 thinking mode."

  - name: gemma4:e4b
    params_b: 8.0
    vram_gb: 5.0
    tier: "excluded"
    vendor: "Google"
    family: gemma4
    instruct: true
    include: false
    rationale: >
      Multimodal model (text + image). Produced valid JSON but zero patterns
      across two independent benchmark runs. Excluded; may be re-evaluated
      with a prompt tuned for multimodal instruct style.
    notes: "Multimodal. Zero patterns detected in both benchmark runs."

  - name: gpt-oss:latest
    params_b: 20.9
    vram_gb: 13.0
    tier: "excluded"
    vendor: "OpenAI"
    family: gpt-oss
    instruct: true
    include: false
    rationale: >
      Thinking mode (Harmony format) leaks into the JSON content field via
      <|channel|>analysis markers, producing invalid output. Excluded for the
      same reason as qwen3:8b until Ollama's format:json handles this family.
    notes: "Harmony-format thinking tokens leak into content. JSON invalid on all test runs."
```

- [ ] **Step 7: Verify manifest loads**

```bash
cd ~/projects/instinct && cat > /tmp/verify_manifest.go << 'EOF'
//go:build ignore
package main

import (
	"fmt"
	"github.com/petersimmons1972/instinct/internal/manifest"
)

func main() {
	m, err := manifest.Load("models/candidates.yaml")
	if err != nil {
		panic(err)
	}
	fmt.Printf("Included: %d, Excluded: %d\n", len(m.Included()), len(m.Excluded()))
}
EOF
go run /tmp/verify_manifest.go
```

Expected: `Included: 14, Excluded: 4`

- [ ] **Step 8: Commit**

```bash
git add internal/manifest/ models/candidates.yaml
git commit -m "feat(manifest): loader with validation + initial candidates.yaml"
```

---

## Task 3: VRAM Detection

**Files:**
- Create: `internal/vram/detect.go`
- Create: `internal/vram/detect_test.go`

- [ ] **Step 1: Write failing tests**

Create `internal/vram/detect_test.go`:

```go
package vram_test

import (
	"testing"

	"github.com/petersimmons1972/instinct/internal/vram"
)

func TestParseNvidiaMB(t *testing.T) {
	cases := []struct {
		input string
		wantGB float64
		wantOK bool
	}{
		{"8192 MiB\n", 8.0, true},
		{"16384 MiB\n", 16.0, true},
		{"[Not Supported]\n", 0, false},
		{"", 0, false},
	}
	for _, c := range cases {
		gotGB, gotOK := vram.ParseNvidiaMiB(c.input)
		if gotOK != c.wantOK || (gotOK && gotGB != c.wantGB) {
			t.Errorf("ParseNvidiaMiB(%q) = (%.1f, %v), want (%.1f, %v)",
				c.input, gotGB, gotOK, c.wantGB, c.wantOK)
		}
	}
}

func TestFallback(t *testing.T) {
	info := vram.Fallback()
	if info.GB != 8.0 {
		t.Errorf("want fallback 8.0 GB, got %.1f", info.GB)
	}
	if info.Source != "fallback" {
		t.Errorf("want source=fallback, got %q", info.Source)
	}
}
```

- [ ] **Step 2: Run — verify fail**

```bash
cd ~/projects/instinct && go test ./internal/vram/... 2>&1 | head -3
```

Expected: `cannot find package`

- [ ] **Step 3: Implement**

Create `internal/vram/detect.go`:

```go
package vram

import (
	"fmt"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
)

type Info struct {
	GB     float64
	Source string // "nvidia" | "amd" | "apple" | "fallback"
	Label  string // human description e.g. "NVIDIA RTX 3090 (24GB)"
}

func Fallback() Info {
	return Info{GB: 8.0, Source: "fallback", Label: "unknown GPU (assumed 8GB)"}
}

// Detect probes GPU VRAM. Never returns an error — falls back to 8GB.
func Detect() Info {
	if info, ok := probeNvidia(); ok {
		return info
	}
	if info, ok := probeAMD(); ok {
		return info
	}
	if runtime.GOOS == "darwin" {
		if info, ok := probeApple(); ok {
			return info
		}
	}
	return Fallback()
}

func probeNvidia() (Info, bool) {
	out, err := exec.Command("nvidia-smi",
		"--query-gpu=name,memory.total",
		"--format=csv,noheader,nounits").Output()
	if err != nil {
		return Info{}, false
	}
	line := strings.TrimSpace(string(out))
	parts := strings.Split(line, ", ")
	if len(parts) < 2 {
		return Info{}, false
	}
	mb, err := strconv.ParseFloat(strings.TrimSpace(parts[len(parts)-1]), 64)
	if err != nil {
		return Info{}, false
	}
	name := strings.Join(parts[:len(parts)-1], ", ")
	gb := mb / 1024.0
	return Info{
		GB:     gb,
		Source: "nvidia",
		Label:  fmt.Sprintf("%s (%.0fGB)", name, gb),
	}, true
}

// ParseNvidiaMiB parses "8192 MiB\n" → 8.0, true. Exported for testing.
func ParseNvidiaMiB(s string) (float64, bool) {
	s = strings.TrimSpace(s)
	s = strings.TrimSuffix(s, " MiB")
	mb, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, false
	}
	return mb / 1024.0, true
}

func probeAMD() (Info, bool) {
	out, err := exec.Command("rocm-smi", "--showmeminfo", "vram", "--json").Output()
	if err != nil {
		return Info{}, false
	}
	// rocm-smi JSON: {"card0": {"VRAM Total Memory (B)": "17163091968", ...}}
	s := string(out)
	idx := strings.Index(s, `"VRAM Total Memory (B)"`)
	if idx < 0 {
		return Info{}, false
	}
	rest := s[idx+len(`"VRAM Total Memory (B)": "`):]
	end := strings.IndexByte(rest, '"')
	if end < 0 {
		return Info{}, false
	}
	bytes, err := strconv.ParseFloat(rest[:end], 64)
	if err != nil {
		return Info{}, false
	}
	gb := bytes / (1024 * 1024 * 1024)
	return Info{GB: gb, Source: "amd", Label: fmt.Sprintf("AMD GPU (%.0fGB)", gb)}, true
}

func probeApple() (Info, bool) {
	// Unified memory: use 50% of total RAM as safe VRAM estimate.
	out, err := exec.Command("sysctl", "-n", "hw.memsize").Output()
	if err != nil {
		return Info{}, false
	}
	totalBytes, err := strconv.ParseFloat(strings.TrimSpace(string(out)), 64)
	if err != nil {
		return Info{}, false
	}
	gb := (totalBytes / 2) / (1024 * 1024 * 1024)
	return Info{
		GB:     gb,
		Source: "apple",
		Label:  fmt.Sprintf("Apple Silicon unified memory (%.0fGB allocated)", gb),
	}, true
}
```

- [ ] **Step 4: Run tests — verify they pass**

```bash
cd ~/projects/instinct && go test ./internal/vram/... -v 2>&1 | tail -8
```

Expected: `ok  github.com/petersimmons1972/instinct/internal/vram`

- [ ] **Step 5: Commit**

```bash
git add internal/vram/
git commit -m "feat(vram): GPU detection for nvidia/amd/apple with fallback"
```

---

## Task 4: Ollama Client

**Files:**
- Create: `internal/ollama/types.go`
- Create: `internal/ollama/client.go`
- Create: `internal/ollama/client_test.go`

- [ ] **Step 1: Create types**

Create `internal/ollama/types.go`:

```go
package ollama

import "encoding/json"

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ChatRequest struct {
	Model     string          `json:"model"`
	Messages  []Message       `json:"messages"`
	Stream    bool            `json:"stream"`
	Format    json.RawMessage `json:"format,omitempty"`
	Options   map[string]any  `json:"options,omitempty"`
	KeepAlive *int            `json:"keep_alive,omitempty"`
}

type ChatResponse struct {
	Model   string `json:"model"`
	Message struct {
		Role     string `json:"role"`
		Content  string `json:"content"`
		Thinking string `json:"thinking"`
	} `json:"message"`
	Done bool `json:"done"`
}

type TagsResponse struct {
	Models []struct {
		Name   string `json:"name"`
		Digest string `json:"digest"`
	} `json:"models"`
}

type ShowResponse struct {
	Modelfile string `json:"modelfile"`
	Details   struct {
		Family string `json:"family"`
	} `json:"details"`
	ModelInfo map[string]any `json:"model_info"`
}

type VersionResponse struct {
	Version string `json:"version"`
}
```

- [ ] **Step 2: Write failing tests**

Create `internal/ollama/client_test.go`:

```go
package ollama_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/petersimmons1972/instinct/internal/ollama"
)

func TestVersion(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/version" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(map[string]string{"version": "0.3.14"})
	}))
	defer srv.Close()

	c := ollama.NewClient(srv.URL)
	v, err := c.Version(context.Background())
	if err != nil {
		t.Fatalf("Version() error: %v", err)
	}
	if v != "0.3.14" {
		t.Errorf("want 0.3.14, got %q", v)
	}
}

func TestIsAvailable_True(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"models": []map[string]any{
				{"name": "mistral:7b", "digest": "sha256:abc123"},
			},
		})
	}))
	defer srv.Close()

	c := ollama.NewClient(srv.URL)
	ok, digest, err := c.IsAvailable(context.Background(), "mistral:7b")
	if err != nil {
		t.Fatalf("IsAvailable() error: %v", err)
	}
	if !ok {
		t.Error("want available=true")
	}
	if digest != "sha256:abc123" {
		t.Errorf("want digest sha256:abc123, got %q", digest)
	}
}

func TestIsAvailable_False(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"models": []any{}})
	}))
	defer srv.Close()

	c := ollama.NewClient(srv.URL)
	ok, _, err := c.IsAvailable(context.Background(), "missing:7b")
	if err != nil {
		t.Fatalf("IsAvailable() error: %v", err)
	}
	if ok {
		t.Error("want available=false")
	}
}

func TestEvict(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		if body["keep_alive"] != float64(0) {
			t.Errorf("want keep_alive=0, got %v", body["keep_alive"])
		}
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]any{"done": true})
	}))
	defer srv.Close()

	c := ollama.NewClient(srv.URL)
	if err := c.Evict(context.Background(), "mistral:7b"); err != nil {
		t.Fatalf("Evict() error: %v", err)
	}
	if !called {
		t.Error("expected HTTP call to Ollama")
	}
}
```

- [ ] **Step 3: Run — verify fail**

```bash
cd ~/projects/instinct && go test ./internal/ollama/... 2>&1 | head -3
```

Expected: `cannot find package`

- [ ] **Step 4: Implement client**

Create `internal/ollama/client.go`:

```go
package ollama

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const defaultBase = "http://localhost:11434"

type Client struct {
	base string
	http *http.Client
}

func NewClient(base string) *Client {
	if base == "" {
		base = defaultBase
	}
	return &Client{base: base, http: &http.Client{}}
}

func (c *Client) get(ctx context.Context, path string, out any) error {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, c.base+path, nil)
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return json.NewDecoder(resp.Body).Decode(out)
}

func (c *Client) post(ctx context.Context, path string, body, out any) error {
	b, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, c.base+path, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if out != nil {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return nil
}

func (c *Client) Version(ctx context.Context) (string, error) {
	var v VersionResponse
	if err := c.get(ctx, "/api/version", &v); err != nil {
		return "", err
	}
	return v.Version, nil
}

func (c *Client) IsAvailable(ctx context.Context, model string) (bool, string, error) {
	var tags TagsResponse
	if err := c.get(ctx, "/api/tags", &tags); err != nil {
		return false, "", err
	}
	for _, m := range tags.Models {
		if m.Name == model {
			return true, m.Digest, nil
		}
	}
	return false, "", nil
}

// Pull streams pull progress to w. Returns (digest, error).
// Uses 1MB scanner buffer — Ollama progress lines can exceed default 64KB.
func (c *Client) Pull(ctx context.Context, model string, w io.Writer) (string, error) {
	body := map[string]string{"name": model}
	b, _ := json.Marshal(body)
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, c.base+"/api/pull", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("pull request: %w", err)
	}
	defer resp.Body.Close()

	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 1024*1024), 1024*1024)
	var lastStatus string
	for sc.Scan() {
		line := sc.Text()
		var m map[string]any
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			continue
		}
		if errMsg, ok := m["error"].(string); ok {
			return "", fmt.Errorf("pull error: %s", errMsg)
		}
		if status, ok := m["status"].(string); ok {
			lastStatus = status
			fmt.Fprintf(w, "\r  pulling %s: %s        ", model, status)
		}
	}
	fmt.Fprintln(w)
	if sc.Err() != nil {
		return "", sc.Err()
	}
	if strings.Contains(lastStatus, "error") {
		return "", fmt.Errorf("pull failed: %s", lastStatus)
	}
	// Fetch digest after pull
	_, digest, err := c.IsAvailable(ctx, model)
	return digest, err
}

// Chat sends a single non-streaming chat request with 300s timeout built into ctx.
func (c *Client) Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	req.Stream = false
	var resp ChatResponse
	if err := c.post(ctx, "/api/chat", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// Evict unloads the model from GPU memory.
func (c *Client) Evict(ctx context.Context, model string) error {
	ka := 0
	body := ChatRequest{
		Model:     model,
		Messages:  []Message{{Role: "user", Content: "x"}},
		Stream:    false,
		KeepAlive: &ka,
	}
	var resp map[string]any
	return c.post(ctx, "/api/generate", body, &resp)
}
```

- [ ] **Step 5: Run tests — verify they pass**

```bash
cd ~/projects/instinct && go test ./internal/ollama/... -v 2>&1 | tail -10
```

Expected: `ok  github.com/petersimmons1972/instinct/internal/ollama`

- [ ] **Step 6: Commit**

```bash
git add internal/ollama/
git commit -m "feat(ollama): HTTP client with pull, chat, evict, version"
```

---

## Task 5: Test Fixture

**Files:**
- Create: `testdata/sample.jsonl`
- Create: `testdata/validate_test.go`

- [ ] **Step 1: Create fixture**

Create `testdata/sample.jsonl` — hand-crafted with one of each pattern type:

```jsonl
{"timestamp":"2026-04-22T10:00:00Z","session_id":"sess-fix-001","project_id":"testproj","tool_name":"Bash","tool_input_hash":"aaa001","tool_output_summary":"curl https://api.example.com/users — exit 0","exit_status":0,"schema_version":1}
{"timestamp":"2026-04-22T10:00:05Z","session_id":"sess-fix-001","project_id":"testproj","tool_name":"Edit","tool_input_hash":"aaa002","tool_output_summary":"Replaced curl with xh in api_client.py","exit_status":0,"schema_version":1}
{"timestamp":"2026-04-22T10:00:10Z","session_id":"sess-fix-001","project_id":"testproj","tool_name":"Bash","tool_input_hash":"aaa003","tool_output_summary":"curl https://api.example.com/orders — exit 0","exit_status":0,"schema_version":1}
{"timestamp":"2026-04-22T10:00:15Z","session_id":"sess-fix-001","project_id":"testproj","tool_name":"Edit","tool_input_hash":"aaa004","tool_output_summary":"Replaced curl with xh in orders_client.py","exit_status":0,"schema_version":1}
{"timestamp":"2026-04-22T10:01:00Z","session_id":"sess-fix-001","project_id":"testproj","tool_name":"Bash","tool_input_hash":"bbb001","tool_output_summary":"pytest tests/test_auth.py — FAILED: KeyError: ENCRYPTION_KEY","exit_status":1,"schema_version":1}
{"timestamp":"2026-04-22T10:01:10Z","session_id":"sess-fix-001","project_id":"testproj","tool_name":"Edit","tool_input_hash":"bbb002","tool_output_summary":"Added ENCRYPTION_KEY export to .env.test","exit_status":0,"schema_version":1}
{"timestamp":"2026-04-22T10:01:20Z","session_id":"sess-fix-001","project_id":"testproj","tool_name":"Bash","tool_input_hash":"bbb003","tool_output_summary":"pytest tests/test_auth.py — FAILED: KeyError: ENCRYPTION_KEY","exit_status":1,"schema_version":1}
{"timestamp":"2026-04-22T10:01:30Z","session_id":"sess-fix-001","project_id":"testproj","tool_name":"Edit","tool_input_hash":"bbb004","tool_output_summary":"Added ENCRYPTION_KEY export to .env.test (missed second location)","exit_status":0,"schema_version":1}
{"timestamp":"2026-04-22T10:02:00Z","session_id":"sess-fix-001","project_id":"testproj","tool_name":"Bash","tool_input_hash":"ccc001","tool_output_summary":"git status — 3 modified files","exit_status":0,"schema_version":1}
{"timestamp":"2026-04-22T10:02:10Z","session_id":"sess-fix-001","project_id":"testproj","tool_name":"Bash","tool_input_hash":"ccc002","tool_output_summary":"git diff --staged — shows api_client.py changes","exit_status":0,"schema_version":1}
{"timestamp":"2026-04-22T10:02:20Z","session_id":"sess-fix-001","project_id":"testproj","tool_name":"Bash","tool_input_hash":"ccc003","tool_output_summary":"gh pr list — 2 open PRs","exit_status":0,"schema_version":1}
{"timestamp":"2026-04-22T10:02:30Z","session_id":"sess-fix-001","project_id":"testproj","tool_name":"Bash","tool_input_hash":"ddd001","tool_output_summary":"git status — 2 modified files","exit_status":0,"schema_version":1}
{"timestamp":"2026-04-22T10:02:40Z","session_id":"sess-fix-001","project_id":"testproj","tool_name":"Bash","tool_input_hash":"ddd002","tool_output_summary":"git diff --staged — shows orders_client.py changes","exit_status":0,"schema_version":1}
{"timestamp":"2026-04-22T10:02:50Z","session_id":"sess-fix-001","project_id":"testproj","tool_name":"Bash","tool_input_hash":"ddd003","tool_output_summary":"gh pr list — 3 open PRs","exit_status":0,"schema_version":1}
{"timestamp":"2026-04-22T10:03:00Z","session_id":"sess-fix-002","project_id":"testproj","tool_name":"Read","tool_input_hash":"eee001","tool_output_summary":"Read src/utils.py 450 lines","exit_status":0,"schema_version":1}
{"timestamp":"2026-04-22T10:03:10Z","session_id":"sess-fix-002","project_id":"testproj","tool_name":"Edit","tool_input_hash":"eee002","tool_output_summary":"Edited src/utils.py line 142","exit_status":0,"schema_version":1}
{"timestamp":"2026-04-22T10:03:20Z","session_id":"sess-fix-002","project_id":"testproj","tool_name":"Bash","tool_input_hash":"eee003","tool_output_summary":"pytest tests/ -x — 12 passed","exit_status":0,"schema_version":1}
{"timestamp":"2026-04-22T10:04:00Z","session_id":"sess-fix-002","project_id":"testproj","tool_name":"Read","tool_input_hash":"fff001","tool_output_summary":"Read src/models.py 280 lines","exit_status":0,"schema_version":1}
{"timestamp":"2026-04-22T10:04:10Z","session_id":"sess-fix-002","project_id":"testproj","tool_name":"Edit","tool_input_hash":"fff002","tool_output_summary":"Edited src/models.py line 88","exit_status":0,"schema_version":1}
{"timestamp":"2026-04-22T10:04:20Z","session_id":"sess-fix-002","project_id":"testproj","tool_name":"Bash","tool_input_hash":"fff003","tool_output_summary":"pytest tests/ -x — 12 passed","exit_status":0,"schema_version":1}
```

- [ ] **Step 2: Write fixture validation test**

Create `testdata/validate_test.go`:

```go
package testdata_test

import (
	"bufio"
	"encoding/json"
	"os"
	"testing"
)

func TestFixtureValid(t *testing.T) {
	f, err := os.Open("sample.jsonl")
	if err != nil {
		t.Fatalf("open sample.jsonl: %v", err)
	}
	defer f.Close()

	required := map[string]bool{"correction": false, "error_resolution": false, "workflow": false}
	// We can't run LLM detection here, so just validate schema and count
	sc := bufio.NewScanner(f)
	lineCount := 0
	for sc.Scan() {
		lineCount++
		var event map[string]any
		if err := json.Unmarshal(sc.Bytes(), &event); err != nil {
			t.Errorf("line %d: invalid JSON: %v", lineCount, err)
			continue
		}
		for _, field := range []string{"timestamp", "session_id", "project_id", "tool_name", "tool_input_hash", "tool_output_summary", "exit_status", "schema_version"} {
			if _, ok := event[field]; !ok {
				t.Errorf("line %d: missing field %q", lineCount, field)
			}
		}
	}
	if lineCount < 20 {
		t.Errorf("fixture too small: %d events (want ≥20)", lineCount)
	}
	// Document that these pattern types are represented in the fixture
	_ = required // Validated by human review of fixture content
	t.Logf("fixture: %d events", lineCount)
}
```

- [ ] **Step 3: Run test**

```bash
cd ~/projects/instinct && go test ./testdata/... -v 2>&1 | tail -5
```

Expected: `ok  github.com/petersimmons1972/instinct/testdata`

- [ ] **Step 4: Commit**

```bash
git add testdata/
git commit -m "feat(testdata): synthetic fixture with correction/error_resolution/workflow patterns"
```

---

## Task 6: Thinking Markers

**Files:**
- Create: `internal/runner/thinking_markers.go`
- Create: `internal/runner/thinking_markers_test.go`

- [ ] **Step 1: Write failing tests**

Create `internal/runner/thinking_markers_test.go`:

```go
package runner_test

import (
	"testing"

	"github.com/petersimmons1972/instinct/internal/runner"
)

func TestDetectThinkingLeak_Clean(t *testing.T) {
	if runner.DetectThinkingLeak(`{"patterns":[]}`) {
		t.Error("clean JSON should not be flagged as thinking leak")
	}
}

func TestDetectThinkingLeak_ThinkTag(t *testing.T) {
	if !runner.DetectThinkingLeak(`<think>Let me analyze...</think>{"patterns":[]}`) {
		t.Error("content with <think> tag should be flagged")
	}
}

func TestDetectThinkingLeak_ThinkingTag(t *testing.T) {
	if !runner.DetectThinkingLeak(`<thinking>reasoning here</thinking>{"patterns":[]}`) {
		t.Error("content with <thinking> tag should be flagged")
	}
}

func TestDetectThinkingLeak_Thought(t *testing.T) {
	if !runner.DetectThinkingLeak(` Thought: I should analyze the patterns...`) {
		t.Error("content with ' Thought:' should be flagged")
	}
}

func TestDetectThinkingLeak_HarmonyFormat(t *testing.T) {
	if !runner.DetectThinkingLeak(`<|channel|>analysis<|message|>{"patterns":[]}`) {
		t.Error("GPT-OSS Harmony format should be flagged")
	}
}
```

- [ ] **Step 2: Run — verify fail**

```bash
cd ~/projects/instinct && go test ./internal/runner/... 2>&1 | head -3
```

Expected: `cannot find package`

- [ ] **Step 3: Implement**

Create `internal/runner/thinking_markers.go`:

```go
package runner

import "strings"

// thinkingMarkers lists token sequences that indicate thinking-mode content
// has leaked into the JSON output field. Keyed by description; each value
// is a string to search for (case-sensitive).
var thinkingMarkers = []string{
	"<think>",
	"</think>",
	"<thinking>",
	"</thinking>",
	" Thought:",       // DeepSeek R1 style
	"<|channel|>analysis", // GPT-OSS Harmony format
}

// DetectThinkingLeak returns true if content contains known thinking-mode tokens.
// Called on Ollama's message.content field. Also check message.thinking separately.
func DetectThinkingLeak(content string) bool {
	for _, marker := range thinkingMarkers {
		if strings.Contains(content, marker) {
			return true
		}
	}
	return false
}
```

- [ ] **Step 4: Run tests — verify pass**

```bash
cd ~/projects/instinct && go test ./internal/runner/... -run TestDetect -v 2>&1 | tail -8
```

Expected: all 5 tests pass

- [ ] **Step 5: Commit**

```bash
git add internal/runner/thinking_markers.go internal/runner/thinking_markers_test.go
git commit -m "feat(runner): thinking leak detection markers"
```

---

## Task 7: Scorer

**Files:**
- Create: `internal/scorer/scorer.go`
- Create: `internal/scorer/scorer_test.go`

- [ ] **Step 1: Write failing tests**

Create `internal/scorer/scorer_test.go`:

```go
package scorer_test

import (
	"testing"
	"time"

	"github.com/petersimmons1972/instinct/internal/scorer"
	"github.com/petersimmons1972/instinct/internal/types"
)

func attempt(content string, thinking string, dur time.Duration, timedOut bool) types.RunAttempt {
	return types.RunAttempt{
		RawContent:   content,
		ThinkingText: thinking,
		Duration:     types.Duration(dur),
		TimedOut:     timedOut,
	}
}

const validPattern = `{"patterns":[{"type":"correction","description":"Use xh not curl","domain":"bash","evidence":"curl replaced with xh twice","tag_signature":"sig-curl-xh","confidence":0.9}]}`
const emptyPatterns = `{"patterns":[]}`
const invalidJSON = `not json`

func TestScore_Recommended(t *testing.T) {
	result := types.RunResult{
		Runs: []types.RunAttempt{
			attempt(validPattern, "", 10*time.Second, false),
			attempt(validPattern, "", 11*time.Second, false),
			attempt(validPattern, "", 12*time.Second, false),
		},
	}
	s := scorer.Score(result)
	if s.Verdict != types.VerdictRecommended {
		t.Errorf("want Recommended, got %s (reason: %s)", s.Verdict, s.VerdictReason)
	}
	if s.ValidPatterns != 1 {
		t.Errorf("want 1 valid pattern, got %d", s.ValidPatterns)
	}
	if !s.JSONValid {
		t.Error("want JSONValid=true")
	}
}

func TestScore_Failed_InvalidJSON(t *testing.T) {
	result := types.RunResult{
		Runs: []types.RunAttempt{
			attempt(invalidJSON, "", 5*time.Second, false),
			attempt(invalidJSON, "", 5*time.Second, false),
			attempt(invalidJSON, "", 5*time.Second, false),
		},
	}
	s := scorer.Score(result)
	if s.Verdict != types.VerdictFailed {
		t.Errorf("want Failed, got %s", s.Verdict)
	}
}

func TestScore_NotRecommended_ThinkingLeak(t *testing.T) {
	result := types.RunResult{
		Runs: []types.RunAttempt{
			attempt(validPattern, "some thinking content", 10*time.Second, false),
			attempt(validPattern, "", 10*time.Second, false),
			attempt(validPattern, "", 10*time.Second, false),
		},
	}
	s := scorer.Score(result)
	if s.Verdict != types.VerdictNotRecommended {
		t.Errorf("want NotRecommended (thinking leak), got %s", s.Verdict)
	}
}

func TestScore_Usable_ZeroPatterns(t *testing.T) {
	result := types.RunResult{
		Runs: []types.RunAttempt{
			attempt(emptyPatterns, "", 8*time.Second, false),
			attempt(emptyPatterns, "", 8*time.Second, false),
			attempt(emptyPatterns, "", 8*time.Second, false),
		},
	}
	s := scorer.Score(result)
	if s.Verdict != types.VerdictUsable {
		t.Errorf("want Usable, got %s", s.Verdict)
	}
}

func TestScore_TimedOut(t *testing.T) {
	result := types.RunResult{
		Runs: []types.RunAttempt{
			attempt("", "", 300*time.Second, true),
			attempt("", "", 300*time.Second, true),
			attempt("", "", 300*time.Second, true),
		},
	}
	s := scorer.Score(result)
	if s.Verdict != types.VerdictTimedOut {
		t.Errorf("want TimedOut, got %s", s.Verdict)
	}
}

func TestScore_Composite(t *testing.T) {
	result := types.RunResult{
		Runs: []types.RunAttempt{
			attempt(validPattern, "", 10*time.Second, false),
			attempt(validPattern, "", 10*time.Second, false),
			attempt(validPattern, "", 10*time.Second, false),
		},
	}
	s := scorer.Score(result)
	// composite = (1.0 * 1 * 2) - (10 * 0.05) = 2.0 - 0.5 = 1.5
	if s.Composite < 1.4 || s.Composite > 1.6 {
		t.Errorf("composite score out of expected range: %.2f", s.Composite)
	}
}

func TestValidPattern_TagSignatureRegex(t *testing.T) {
	good := `{"patterns":[{"type":"workflow","description":"x","domain":"bash","evidence":"y","tag_signature":"sig-git-diff-pr","confidence":0.8}]}`
	bad := `{"patterns":[{"type":"workflow","description":"x","domain":"bash","evidence":"y","tag_signature":"SIG INVALID","confidence":0.8}]}`
	if r := scorer.Score(types.RunResult{Runs: []types.RunAttempt{attempt(good,"",5*time.Second,false),attempt(good,"",5*time.Second,false),attempt(good,"",5*time.Second,false)}}); r.ValidPatterns != 1 {
		t.Errorf("good tag_signature: want 1 valid, got %d", r.ValidPatterns)
	}
	if r := scorer.Score(types.RunResult{Runs: []types.RunAttempt{attempt(bad,"",5*time.Second,false),attempt(bad,"",5*time.Second,false),attempt(bad,"",5*time.Second,false)}}); r.ValidPatterns != 0 {
		t.Errorf("bad tag_signature: want 0 valid, got %d", r.ValidPatterns)
	}
}
```

- [ ] **Step 2: Run — verify fail**

```bash
cd ~/projects/instinct && go test ./internal/scorer/... 2>&1 | head -3
```

Expected: `cannot find package`

- [ ] **Step 3: Implement**

Create `internal/scorer/scorer.go`:

```go
package scorer

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/petersimmons1972/instinct/internal/runner"
	"github.com/petersimmons1972/instinct/internal/types"
)

var tagSlugRe = regexp.MustCompile(`^[a-z0-9-]{3,64}$`)

var validTypes = map[string]bool{
	"correction":       true,
	"error_resolution": true,
	"workflow":         true,
}

type rawPattern struct {
	Type        string  `json:"type"`
	Description string  `json:"description"`
	Domain      string  `json:"domain"`
	Evidence    string  `json:"evidence"`
	TagSig      string  `json:"tag_signature"`
	Confidence  float64 `json:"confidence"`
}

type rawResponse struct {
	Patterns []rawPattern `json:"patterns"`
}

func isValidPattern(p rawPattern) bool {
	return validTypes[p.Type] &&
		strings.TrimSpace(p.Description) != "" &&
		strings.TrimSpace(p.Evidence) != "" &&
		tagSlugRe.MatchString(p.TagSig) &&
		p.Confidence >= 0.0 && p.Confidence <= 1.0
}

// Score computes a Score from a RunResult. Pure function — no I/O.
func Score(result types.RunResult) types.Score {
	// All timed out?
	allTimedOut := true
	for _, r := range result.Runs {
		if !r.TimedOut {
			allTimedOut = false
			break
		}
	}
	if allTimedOut {
		return types.Score{Verdict: types.VerdictTimedOut, VerdictReason: "all runs timed out"}
	}

	// Skipped?
	if result.Skipped {
		return types.Score{Verdict: types.VerdictSkippedVRAM, VerdictReason: result.SkipReason}
	}

	// Check thinking leak (any run)
	thinkingLeak := false
	for _, r := range result.Runs {
		if r.ThinkingText != "" || runner.DetectThinkingLeak(r.RawContent) {
			thinkingLeak = true
			break
		}
	}

	// Parse JSON for each run
	type runParsed struct {
		valid    bool
		patterns []rawPattern
		latency  time.Duration
	}
	parsed := make([]runParsed, len(result.Runs))
	for i, r := range result.Runs {
		parsed[i].latency = r.Duration.Std()
		var resp rawResponse
		if err := json.Unmarshal([]byte(r.RawContent), &resp); err == nil {
			parsed[i].valid = true
			parsed[i].patterns = resp.Patterns
		}
	}

	// JSON valid if ≥2 of 3 runs parsed
	validCount := 0
	for _, p := range parsed {
		if p.valid {
			validCount++
		}
	}
	jsonValid := validCount >= 2

	if !jsonValid {
		return types.Score{
			JSONValid:     false,
			Verdict:       types.VerdictFailed,
			VerdictReason: fmt.Sprintf("JSON invalid on %d of %d runs", len(parsed)-validCount, len(parsed)),
		}
	}

	if thinkingLeak {
		return types.Score{
			JSONValid:     jsonValid,
			ThinkingLeak:  true,
			Verdict:       types.VerdictNotRecommended,
			VerdictReason: "thinking mode tokens leak into JSON content field",
		}
	}

	// Median pattern count across valid runs
	counts := []int{}
	for _, p := range parsed {
		if p.valid {
			counts = append(counts, len(p.patterns))
		}
	}
	sort.Ints(counts)
	medianCount := counts[len(counts)/2]

	// Pick the run whose pattern count == median (first match)
	var medianPatterns []rawPattern
	for _, p := range parsed {
		if p.valid && len(p.patterns) == medianCount {
			medianPatterns = p.patterns
			break
		}
	}

	// Score valid patterns
	validPatterns := 0
	for _, p := range medianPatterns {
		if isValidPattern(p) {
			validPatterns++
		}
	}

	// Average latency
	var totalLatency time.Duration
	latencyCount := 0
	for _, p := range parsed {
		if p.latency > 0 {
			totalLatency += p.latency
			latencyCount++
		}
	}
	var avgLatency time.Duration
	if latencyCount > 0 {
		avgLatency = totalLatency / time.Duration(latencyCount)
	}

	qualityPct := 0.0
	if len(medianPatterns) > 0 {
		qualityPct = float64(validPatterns) / float64(len(medianPatterns))
	}
	composite := (qualityPct * float64(validPatterns) * 2) - (avgLatency.Seconds() * 0.05)

	if validPatterns == 0 {
		return types.Score{
			JSONValid:    jsonValid,
			PatternCount: medianCount,
			AvgLatency:   types.Duration(avgLatency),
			Composite:    composite,
			Verdict:      types.VerdictUsable,
			VerdictReason: "valid JSON; zero patterns detected — may improve with prompt tuning",
		}
	}

	return types.Score{
		JSONValid:     jsonValid,
		PatternCount:  medianCount,
		ValidPatterns: validPatterns,
		QualityPct:    qualityPct,
		AvgLatency:    types.Duration(avgLatency),
		Composite:     composite,
		Verdict:       types.VerdictRecommended,
		VerdictReason: fmt.Sprintf("detected %d valid pattern(s) with %.0f%% schema conformance", validPatterns, qualityPct*100),
	}
}
```

- [ ] **Step 4: Run tests — verify pass**

```bash
cd ~/projects/instinct && go test ./internal/scorer/... -v 2>&1 | tail -12
```

Expected: all 6 tests pass

- [ ] **Step 5: Commit**

```bash
git add internal/scorer/
git commit -m "feat(scorer): pure scoring function with verdict tree and composite score"
```

---

## Task 8: Cache

**Files:**
- Create: `internal/cache/cache.go`
- Create: `internal/cache/cache_test.go`

- [ ] **Step 1: Write failing tests**

Create `internal/cache/cache_test.go`:

```go
package cache_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/petersimmons1972/instinct/internal/cache"
	"github.com/petersimmons1972/instinct/internal/types"
)

func TestRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "results.json")
	c := cache.New(path)

	result := types.ModelResult{
		Model:  "mistral:7b",
		VRAMGB: 4.5,
		Tier:   "4-6GB",
		Vendor: "Mistral AI",
		Score: types.Score{
			Verdict:   types.VerdictRecommended,
			Composite: 7.44,
		},
	}
	key := "sha256abc"
	if err := c.Write("mistral:7b", key, result); err != nil {
		t.Fatalf("Write: %v", err)
	}

	got, ok, err := c.Read("mistral:7b", key, 24*time.Hour)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if !ok {
		t.Fatal("expected cache hit")
	}
	if got.Score.Verdict != types.VerdictRecommended {
		t.Errorf("want Recommended, got %s", got.Score.Verdict)
	}
}

func TestRead_Miss_WrongKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "results.json")
	c := cache.New(path)
	result := types.ModelResult{Model: "mistral:7b"}
	c.Write("mistral:7b", "key-a", result)

	_, ok, err := c.Read("mistral:7b", "key-b", 24*time.Hour)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if ok {
		t.Error("expected cache miss for wrong key")
	}
}

func TestRead_Miss_Expired(t *testing.T) {
	path := filepath.Join(t.TempDir(), "results.json")
	c := cache.New(path)
	result := types.ModelResult{Model: "mistral:7b"}
	c.Write("mistral:7b", "key-a", result)

	_, ok, err := c.Read("mistral:7b", "key-a", -1*time.Second) // expired immediately
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if ok {
		t.Error("expected cache miss for expired entry")
	}
}
```

- [ ] **Step 2: Run — verify fail**

```bash
cd ~/projects/instinct && go test ./internal/cache/... 2>&1 | head -3
```

Expected: `cannot find package`

- [ ] **Step 3: Implement**

Create `internal/cache/cache.go`:

```go
package cache

import (
	"encoding/json"
	"os"
	"sync"
	"time"

	"github.com/petersimmons1972/instinct/internal/types"
)

type entry struct {
	CacheKey  string           `json:"cache_key"`
	StoredAt  time.Time        `json:"stored_at"`
	Result    types.ModelResult `json:"result"`
}

type store struct {
	Entries map[string]entry `json:"entries"` // keyed by model name
}

type Cache struct {
	path string
	mu   sync.Mutex
}

func New(path string) *Cache {
	return &Cache{path: path}
}

func (c *Cache) load() (store, error) {
	var s store
	s.Entries = map[string]entry{}
	data, err := os.ReadFile(c.path)
	if os.IsNotExist(err) {
		return s, nil
	}
	if err != nil {
		return s, err
	}
	err = json.Unmarshal(data, &s)
	return s, err
}

func (c *Cache) save(s store) error {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(c.path, data, 0644)
}

// Write stores a result for model with the given cache key.
func (c *Cache) Write(model, cacheKey string, result types.ModelResult) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	s, err := c.load()
	if err != nil {
		return err
	}
	s.Entries[model] = entry{
		CacheKey: cacheKey,
		StoredAt: time.Now(),
		Result:   result,
	}
	return c.save(s)
}

// Read returns the cached result if the key matches and entry is fresher than maxAge.
func (c *Cache) Read(model, cacheKey string, maxAge time.Duration) (types.ModelResult, bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	s, err := c.load()
	if err != nil {
		return types.ModelResult{}, false, err
	}
	e, ok := s.Entries[model]
	if !ok {
		return types.ModelResult{}, false, nil
	}
	if e.CacheKey != cacheKey {
		return types.ModelResult{}, false, nil
	}
	if time.Since(e.StoredAt) > maxAge {
		return types.ModelResult{}, false, nil
	}
	return e.Result, true, nil
}
```

- [ ] **Step 4: Run tests — verify pass**

```bash
cd ~/projects/instinct && go test ./internal/cache/... -v 2>&1 | tail -8
```

Expected: `ok  github.com/petersimmons1972/instinct/internal/cache`

- [ ] **Step 5: Commit**

```bash
git add internal/cache/
git commit -m "feat(cache): sha256-keyed result cache with TTL"
```

---

## Task 9: Runner

**Files:**
- Create: `internal/runner/runner.go`
- Create: `internal/runner/runner_test.go`

- [ ] **Step 1: Write failing tests**

Create `internal/runner/runner_test.go`:

```go
package runner_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/petersimmons1972/instinct/internal/manifest"
	"github.com/petersimmons1972/instinct/internal/ollama"
	"github.com/petersimmons1972/instinct/internal/runner"
)

func mockOllama(t *testing.T, responseContent string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/tags":
			json.NewEncoder(w).Encode(map[string]any{
				"models": []map[string]any{
					{"name": "mistral:7b", "digest": "sha256:test"},
				},
			})
		case "/api/version":
			json.NewEncoder(w).Encode(map[string]string{"version": "0.3.0"})
		case "/api/chat":
			json.NewEncoder(w).Encode(map[string]any{
				"model": "mistral:7b",
				"message": map[string]string{
					"role":    "assistant",
					"content": responseContent,
				},
				"done": true,
			})
		case "/api/generate": // evict
			json.NewEncoder(w).Encode(map[string]any{"done": true})
		}
	}))
}

func TestRun_ProducesRunResult(t *testing.T) {
	content := `{"patterns":[{"type":"correction","description":"Use xh","domain":"bash","evidence":"curl used twice","tag_signature":"sig-curl-xh","confidence":0.9}]}`
	srv := mockOllama(t, content)
	defer srv.Close()

	model := manifest.Model{
		Name:   "mistral:7b",
		VRAMgb: 4.5,
		Tier:   "4-6GB",
		Vendor: "Mistral AI",
		Family: "mistral",
	}
	client := ollama.NewClient(srv.URL)
	result, err := runner.Run(context.Background(), client, model, "testdata/sample.jsonl", 1)
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if len(result.Runs) != 1 {
		t.Errorf("want 1 run, got %d", len(result.Runs))
	}
	if result.Runs[0].RawContent != content {
		t.Errorf("unexpected content: %q", result.Runs[0].RawContent)
	}
}
```

- [ ] **Step 2: Run — verify fail**

```bash
cd ~/projects/instinct && go test ./internal/runner/... -run TestRun 2>&1 | head -5
```

Expected: `undefined: runner.Run`

- [ ] **Step 3: Implement**

Create `internal/runner/runner.go`:

```go
package runner

// Sequential by design — do not parallelise. Models share GPU.

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/petersimmons1972/instinct/internal/manifest"
	"github.com/petersimmons1972/instinct/internal/ollama"
	"github.com/petersimmons1972/instinct/internal/types"
)

const systemPrompt = `You are a pattern detection system analyzing Claude Code tool call sequences.

Analyze the tool call events and identify recurring patterns of these types:

1. CORRECTION: Evidence the user corrected the AI — re-do after rollback, same action reversed within 3 steps.
2. ERROR_RESOLUTION: The same error (exit_status=1 + similar output) followed by the same fix, 2+ times.
3. WORKFLOW: A sequence of 3+ tool calls that recurs within or across sessions.

Return a JSON object with a single key "patterns" containing an array. Each pattern must have:
- "type": "correction" | "error_resolution" | "workflow"
- "description": one sentence, present tense
- "domain": one word (testing|git|editing|bash|agent|memory|general)
- "evidence": brief explanation, max 100 chars
- "tag_signature": lowercase slug e.g. "sig-edit-bash-fail"
- "confidence": 0.0 to 1.0

If no patterns found, return {"patterns":[]}.
Return ONLY valid JSON — no prose, no markdown fences.`

func loadEvents(fixturePath string) (string, error) {
	f, err := os.Open(fixturePath)
	if err != nil {
		return "", fmt.Errorf("opening fixture %s: %w", fixturePath, err)
	}
	defer f.Close()

	var lines []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var event map[string]any
		if err := json.Unmarshal(sc.Bytes(), &event); err != nil {
			continue
		}
		toolName, _ := event["tool_name"].(string)
		summary, _ := event["tool_output_summary"].(string)
		ts, _ := event["timestamp"].(string)
		exitStatus, _ := event["exit_status"].(float64)
		lines = append(lines, fmt.Sprintf("[%s] %s | exit=%.0f | %s", ts, toolName, exitStatus, truncate(summary, 80)))
	}
	return fmt.Sprintf("Tool call events (%d total):\n", len(lines)) + joinLines(lines), nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

func joinLines(lines []string) string {
	result := ""
	for _, l := range lines {
		result += l + "\n"
	}
	return result
}

// Run executes numRuns inference calls against model and returns raw results.
func Run(ctx context.Context, client *ollama.Client, model manifest.Model, fixturePath string, numRuns int) (types.RunResult, error) {
	result := types.RunResult{Model: model.Name}

	// Check if already available, get digest
	available, digest, err := client.IsAvailable(ctx, model.Name)
	if err != nil {
		return result, fmt.Errorf("checking availability: %w", err)
	}
	if !available {
		// Pull
		pullStart := time.Now()
		d, err := client.Pull(ctx, model.Name, os.Stdout)
		result.PullDuration = types.Duration(time.Since(pullStart))
		if err != nil {
			result.Skipped = true
			result.SkipReason = fmt.Sprintf("pull failed: %v", err)
			return result, nil
		}
		digest = d
	}
	result.ModelDigest = digest

	userMsg, err := loadEvents(fixturePath)
	if err != nil {
		return result, err
	}

	formatJSON := json.RawMessage(`"json"`)

	for i := 0; i < numRuns; i++ {
		runCtx, cancel := context.WithTimeout(ctx, 300*time.Second)
		attempt := types.RunAttempt{}
		start := time.Now()

		resp, err := client.Chat(runCtx, ollama.ChatRequest{
			Model: model.Name,
			Messages: []ollama.Message{
				{Role: "system", Content: systemPrompt},
				{Role: "user", Content: userMsg},
			},
			Format:  formatJSON,
			Options: map[string]any{"temperature": 0.1, "num_predict": 1024},
		})
		attempt.Duration = types.Duration(time.Since(start))
		cancel()

		if err != nil {
			if ctx.Err() != nil {
				attempt.TimedOut = true
			} else {
				attempt.Error = err.Error()
			}
		} else {
			attempt.RawContent = resp.Message.Content
			attempt.ThinkingText = resp.Message.Thinking
		}
		result.Runs = append(result.Runs, attempt)
	}

	// Evict from GPU after all runs
	evictCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	_ = client.Evict(evictCtx, model.Name)
	cancel()

	return result, nil
}
```

- [ ] **Step 4: Run tests — verify pass**

```bash
cd ~/projects/instinct && go test ./internal/runner/... -v 2>&1 | tail -12
```

Expected: all tests pass

- [ ] **Step 5: Commit**

```bash
git add internal/runner/runner.go internal/runner/runner_test.go
git commit -m "feat(runner): per-model run orchestration with pull, inference, evict"
```

---

## Task 10: Reporter

**Files:**
- Create: `internal/reporter/markdown.go`
- Create: `internal/reporter/markdown_test.go`
- Create: `internal/reporter/svg.go`
- Create: `internal/reporter/svg_test.go`

- [ ] **Step 1: Write failing markdown test**

Create `internal/reporter/markdown_test.go`:

```go
package reporter_test

import (
	"strings"
	"testing"
	"time"

	"github.com/petersimmons1972/instinct/internal/manifest"
	"github.com/petersimmons1972/instinct/internal/reporter"
	"github.com/petersimmons1972/instinct/internal/types"
	"github.com/petersimmons1972/instinct/internal/vram"
)

func sampleResults() []types.ModelResult {
	return []types.ModelResult{
		{
			Model:  "mistral:7b",
			VRAMGB: 4.5,
			Tier:   "4-6GB",
			Vendor: "Mistral AI",
			Score: types.Score{
				Verdict:       types.VerdictRecommended,
				ValidPatterns: 4,
				AvgLatency:    types.Duration(13 * time.Second),
				Composite:     7.44,
				VerdictReason: "detected 4 valid patterns",
			},
		},
	}
}

func sampleManifest() *manifest.Manifest {
	note := "thinking leak"
	return &manifest.Manifest{
		Models: []manifest.Model{
			{Name: "mistral:7b", VRAMgb: 4.5, Tier: "4-6GB", Vendor: "Mistral AI", Family: "mistral", Instruct: true, Include: true, Rationale: "Fast and accurate."},
			{Name: "qwen3:8b", VRAMgb: 5.0, Tier: "excluded", Vendor: "Alibaba", Family: "qwen3", Instruct: true, Include: false, Rationale: "Thinking leak.", Notes: &note},
		},
	}
}

func TestRenderMarkdown_ContainsRecommended(t *testing.T) {
	info := reporter.RunInfo{OllamaVersion: "0.3.14", OS: "linux", GPU: vram.Info{GB: 20, Source: "nvidia", Label: "RTX 3090"}}
	md := reporter.RenderMarkdown(sampleResults(), sampleManifest(), info)
	if !strings.Contains(md, "mistral:7b") {
		t.Error("markdown should contain mistral:7b")
	}
	if !strings.Contains(md, "## Recommended") {
		t.Error("markdown should have Recommended section")
	}
	if !strings.Contains(md, "## Rejected before testing") {
		t.Error("markdown should have Rejected section")
	}
	if !strings.Contains(md, "qwen3:8b") {
		t.Error("markdown should list excluded model qwen3:8b")
	}
	if !strings.Contains(md, "Generated by instinct-benchmark") {
		t.Error("markdown should have generation header")
	}
}
```

- [ ] **Step 2: Run — verify fail**

```bash
cd ~/projects/instinct && go test ./internal/reporter/... 2>&1 | head -3
```

Expected: `cannot find package`

- [ ] **Step 3: Implement markdown reporter**

Create `internal/reporter/markdown.go`:

```go
package reporter

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/petersimmons1972/instinct/internal/manifest"
	"github.com/petersimmons1972/instinct/internal/types"
	"github.com/petersimmons1972/instinct/internal/vram"
)

type RunInfo struct {
	OllamaVersion string
	OS            string
	GPU           vram.Info
	BinaryVersion string
	Timestamp     time.Time
}

func RenderMarkdown(results []types.ModelResult, m *manifest.Manifest, info RunInfo) string {
	if info.Timestamp.IsZero() {
		info.Timestamp = time.Now().UTC()
	}
	var b strings.Builder

	fmt.Fprintf(&b, "<!-- Generated by instinct-benchmark. Do not edit by hand.\n")
	fmt.Fprintf(&b, "     Re-run: go run ./cmd/benchmark\n")
	fmt.Fprintf(&b, "     Last run: %s | Ollama %s | %s | GPU: %s -->\n\n",
		info.Timestamp.Format(time.RFC3339), info.OllamaVersion, info.OS, info.GPU.Label)

	b.WriteString("# Model Compatibility\n\n")
	b.WriteString("Instinct uses a local Ollama model to detect patterns in your Claude Code sessions.\n")
	b.WriteString("This page documents which models work, which don't, and why —\n")
	b.WriteString("so you can pick the right one for your hardware without guessing.\n\n")

	// Suggested default
	best := bestRecommended(results)
	if best != nil {
		fmt.Fprintf(&b, "> **Suggested default:** `%s` (composite score %.2f, tier %s)\n\n", best.Model, best.Score.Composite, best.Tier)
	}

	// Choose by VRAM
	b.WriteString("## Choose by VRAM\n\n")
	b.WriteString("| VRAM available | Recommended model | Notes |\n")
	b.WriteString("|----------------|-------------------|-------|\n")
	for _, tier := range []string{"≤2GB", "4-6GB", "7-10GB", "11-16GB"} {
		rec := firstRecommendedInTier(results, tier)
		if rec != nil {
			fmt.Fprintf(&b, "| %s | `%s` | %.1f GB VRAM, avg %s |\n",
				tier, rec.Model, rec.VRAMGB, rec.Score.AvgLatency.Std().Round(time.Second))
		} else {
			fmt.Fprintf(&b, "| %s | — | No tested model available |\n", tier)
		}
	}
	b.WriteString("\n")

	// Sections by verdict
	writeSection(&b, "## Recommended\n\n", results, types.VerdictRecommended)
	writeSection(&b, "## Usable (may need prompt tuning)\n\n", results, types.VerdictUsable)
	writeSection(&b, "## Not recommended\n\n", results, types.VerdictNotRecommended)
	writeSection(&b, "## Failed\n\n", results, types.VerdictFailed, types.VerdictTimedOut, types.VerdictPullFailed)

	// Excluded (include: false from manifest)
	excluded := m.Excluded()
	if len(excluded) > 0 {
		b.WriteString("## Rejected before testing\n\n")
		b.WriteString("| Model | Vendor | VRAM est. | Reason |\n")
		b.WriteString("|-------|--------|-----------|--------|\n")
		for _, model := range excluded {
			reason := strings.ReplaceAll(strings.TrimSpace(model.Rationale), "\n", " ")
			fmt.Fprintf(&b, "| `%s` | %s | %.1f GB | %s |\n",
				model.Name, model.Vendor, model.VRAMgb, reason)
		}
		b.WriteString("\n")
	}

	// Methodology
	b.WriteString("## Methodology\n\n")
	b.WriteString("- **Sample:** 40 tool-call events (deterministic fixture; use `--use-live-buffer` for live data)\n")
	b.WriteString("- **Runs:** 3 per model, median pattern count, mean latency\n")
	b.WriteString("- **Scored on:** JSON validity, pattern schema conformance, pattern count, latency\n")
	b.WriteString("- **VRAM:** estimated at Q4_K_M quantisation; actual usage varies by system\n")
	fmt.Fprintf(&b, "- **Hardware:** %s\n", info.GPU.Label)
	fmt.Fprintf(&b, "- **Last run:** %s\n\n", info.Timestamp.Format("2006-01-02 15:04 UTC"))

	// Contributing
	b.WriteString("## Contributing\n\n")
	b.WriteString("Submit a PR editing `models/candidates.yaml`. See `CONTRIBUTING.md`.\n")
	b.WriteString("CI validates the manifest schema and — for new `include: true` models — runs the benchmark\n")
	b.WriteString("and posts results as a PR comment before merge.\n")

	return b.String()
}

func writeSection(b *strings.Builder, header string, results []types.ModelResult, verdicts ...types.Verdict) {
	var rows []types.ModelResult
	for _, r := range results {
		for _, v := range verdicts {
			if r.Score.Verdict == v {
				rows = append(rows, r)
				break
			}
		}
	}
	if len(rows) == 0 {
		return
	}
	b.WriteString(header)
	b.WriteString("| Model | Vendor | VRAM | Params | Avg latency | Patterns | Notes |\n")
	b.WriteString("|-------|--------|------|--------|-------------|----------|-------|\n")
	for _, r := range rows {
		fmt.Fprintf(b, "| `%s` | %s | %.1f GB | — | %s | %d | %s |\n",
			r.Model, r.Vendor, r.VRAMGB,
			r.Score.AvgLatency.Std().Round(time.Second),
			r.Score.ValidPatterns,
			r.Score.VerdictReason)
	}
	b.WriteString("\n")
}

func bestRecommended(results []types.ModelResult) *types.ModelResult {
	var candidates []types.ModelResult
	for _, r := range results {
		if r.Score.Verdict == types.VerdictRecommended {
			candidates = append(candidates, r)
		}
	}
	if len(candidates) == 0 {
		return nil
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].VRAMGB != candidates[j].VRAMGB {
			return candidates[i].VRAMGB < candidates[j].VRAMGB
		}
		return candidates[i].Score.Composite > candidates[j].Score.Composite
	})
	return &candidates[0]
}

func firstRecommendedInTier(results []types.ModelResult, tier string) *types.ModelResult {
	for i := range results {
		if results[i].Tier == tier && results[i].Score.Verdict == types.VerdictRecommended {
			return &results[i]
		}
	}
	return nil
}
```

- [ ] **Step 4: Implement SVG reporter**

Create `internal/reporter/svg.go`:

```go
package reporter

import (
	"fmt"
	"strings"
	"text/template"

	"github.com/petersimmons1972/instinct/internal/types"
)

// Visual identity: instinct — Cassandre geometric streamline moderne
// Palette: #0D1117 bg, #4FAAFF accent, #E6EDF3 text, #1E3A5F structure
// Canvas: 750x420 (matches pipeline-overview.svg per VISUAL-IDENTITY.md)

const svgTmpl = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 750 420">
  <rect width="750" height="420" fill="#0D1117"/>
  <!-- Title -->
  <text x="375" y="36" font-family="'Helvetica Neue',Arial,sans-serif" font-size="13"
    fill="#4FAAFF" text-anchor="middle" letter-spacing="0.15em">MODEL COMPATIBILITY</text>
  <!-- Tier registers -->
{{- range $i, $tier := .Tiers}}
  <rect x="40" y="{{$tier.Y}}" width="670" height="60" fill="#1E3A5F" opacity="0.3"/>
  <text x="50" y="{{add $tier.Y 20}}" font-family="'Helvetica Neue',Arial,sans-serif"
    font-size="10" fill="#E6EDF3" letter-spacing="0.1em">{{$tier.Label}}</text>
{{- range $j, $model := $tier.Models}}
  <!-- Signal line: {{$model.Name}} -->
  <line x1="{{$model.X}}" y1="{{add $tier.Y 30}}" x2="375" y2="380"
    stroke="{{$model.Color}}" stroke-width="1" opacity="{{$model.Opacity}}"/>
  <circle cx="{{$model.X}}" cy="{{add $tier.Y 30}}" r="3" fill="{{$model.Color}}" opacity="{{$model.Opacity}}"/>
  <text x="{{$model.X}}" y="{{add $tier.Y 50}}" font-family="'Helvetica Neue',Arial,sans-serif"
    font-size="8" fill="#E6EDF3" text-anchor="middle" opacity="{{$model.Opacity}}">{{$model.ShortName}}</text>
{{- end}}
{{- end}}
  <!-- Convergence node -->
  <polygon points="375,370 365,388 385,388" fill="#4FAAFF" opacity="0.9"/>
  <text x="375" y="410" font-family="'Helvetica Neue',Arial,sans-serif"
    font-size="9" fill="#4FAAFF" text-anchor="middle" letter-spacing="0.1em">INSTINCT</text>
</svg>`

type svgModel struct {
	Name      string
	ShortName string
	X         int
	Color     string
	Opacity   string
}

type svgTier struct {
	Label  string
	Y      int
	Models []svgModel
}

type svgData struct {
	Tiers []svgTier
}

func RenderSVG(results []types.ModelResult) string {
	tiers := []struct {
		label string
		y     int
	}{
		{"≤2GB", 60},
		{"4–6GB", 140},
		{"7–10GB", 220},
		{"11–16GB", 300},
	}

	var data svgData
	for i, tier := range tiers {
		st := svgTier{Label: tier.label, Y: tier.y}
		var models []types.ModelResult
		for _, r := range results {
			if r.Tier == tier.label || (i == 3 && r.VRAMGB > 10 && r.VRAMGB <= 16) {
				models = append(models, r)
			}
		}
		spacing := 670 / (len(models) + 1)
		for j, m := range models {
			color := "#4FAAFF"
			if m.Score.Verdict != types.VerdictRecommended {
				color = "#1E3A5F"
			}
			opacity := fmt.Sprintf("%.1f", 0.3+(m.Score.Composite/10.0))
			if m.Score.Verdict == types.VerdictRecommended {
				opacity = "0.9"
			}
			short := m.Model
			if idx := strings.Index(short, ":"); idx > 0 {
				short = short[:idx]
			}
			st.Models = append(st.Models, svgModel{
				Name:      m.Model,
				ShortName: short,
				X:         40 + spacing*(j+1),
				Color:     color,
				Opacity:   opacity,
			})
		}
		data.Tiers = append(data.Tiers, st)
	}

	funcMap := template.FuncMap{
		"add": func(a, b int) int { return a + b },
	}
	tmpl := template.Must(template.New("svg").Funcs(funcMap).Parse(svgTmpl))
	var b strings.Builder
	tmpl.Execute(&b, data)
	return b.String()
}
```

- [ ] **Step 5: Write SVG test**

Create `internal/reporter/svg_test.go`:

```go
package reporter_test

import (
	"strings"
	"testing"
	"time"

	"github.com/petersimmons1972/instinct/internal/reporter"
	"github.com/petersimmons1972/instinct/internal/types"
)

func TestRenderSVG_ContainsExpected(t *testing.T) {
	results := []types.ModelResult{
		{Model: "mistral:7b", VRAMGB: 4.5, Tier: "4-6GB", Score: types.Score{
			Verdict: types.VerdictRecommended, Composite: 7.44,
			AvgLatency: types.Duration(13 * time.Second),
		}},
	}
	svg := reporter.RenderSVG(results)
	if !strings.Contains(svg, `viewBox="0 0 750 420"`) {
		t.Error("SVG should have correct viewBox")
	}
	if !strings.Contains(svg, "#4FAAFF") {
		t.Error("SVG should use accent colour #4FAAFF")
	}
	if !strings.Contains(svg, "#0D1117") {
		t.Error("SVG should use background colour #0D1117")
	}
}
```

- [ ] **Step 6: Run all reporter tests**

```bash
cd ~/projects/instinct && go test ./internal/reporter/... -v 2>&1 | tail -12
```

Expected: all tests pass

- [ ] **Step 7: Commit**

```bash
git add internal/reporter/
git commit -m "feat(reporter): markdown + SVG generation with instinct visual identity"
```

---

## Task 11: CLI Entry Point

**Files:**
- Create: `cmd/benchmark/main.go`

- [ ] **Step 1: Implement main.go**

Create `cmd/benchmark/main.go`:

```go
package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/petersimmons1972/instinct/internal/cache"
	"github.com/petersimmons1972/instinct/internal/manifest"
	"github.com/petersimmons1972/instinct/internal/ollama"
	"github.com/petersimmons1972/instinct/internal/reporter"
	"github.com/petersimmons1972/instinct/internal/runner"
	"github.com/petersimmons1972/instinct/internal/scorer"
	"github.com/petersimmons1972/instinct/internal/types"
	"github.com/petersimmons1972/instinct/internal/vram"
)

// Version is injected at build time via -ldflags "-X main.Version=$(git describe --tags --always)"
var Version = "dev"

func main() {
	var (
		manifestPath  = flag.String("manifest", "models/candidates.yaml", "path to candidates.yaml")
		fixturePath   = flag.String("fixture", "testdata/sample.jsonl", "event fixture file")
		resultsPath   = flag.String("results", "docs/benchmark-results.json", "output results JSON")
		docsPath      = flag.String("docs", "docs/models.md", "output documentation markdown")
		svgPath       = flag.String("svg", "docs/assets/svg/model-tiers.svg", "output SVG diagram")
		ollamaURL     = flag.String("ollama", "http://localhost:11434", "Ollama base URL")
		singleModel   = flag.String("model", "", "run only this model (exact manifest name)")
		numRuns       = flag.Int("runs", 3, "inference runs per model")
		dryRun        = flag.Bool("dry-run", false, "validate manifest only, skip inference")
		force         = flag.Bool("force", false, "bypass result cache")
		useLiveBuffer = flag.Bool("use-live-buffer", false, "use ~/.local/state/instinct/buffer.jsonl instead of fixture")
	)
	flag.Parse()

	fmt.Printf("instinct-benchmark %s\n", Version)

	// Load manifest
	m, err := manifest.Load(*manifestPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: invalid manifest: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Manifest: %d included, %d excluded\n", len(m.Included()), len(m.Excluded()))

	if *dryRun {
		fmt.Println("Dry run complete — manifest valid.")
		os.Exit(0)
	}

	// Resolve fixture path
	eventSource := *fixturePath
	if *useLiveBuffer {
		liveBuffer := filepath.Join(os.Getenv("HOME"), ".local/state/instinct/buffer.jsonl")
		if _, err := os.Stat(liveBuffer); err == nil {
			eventSource = liveBuffer
			fmt.Printf("Using live buffer: %s\n", liveBuffer)
		} else {
			fmt.Printf("Live buffer not found, falling back to fixture: %s\n", *fixturePath)
		}
	}

	// GPU detection
	gpuInfo := vram.Detect()
	fmt.Printf("GPU: %s\n", gpuInfo.Label)
	maxVRAM := gpuInfo.GB * 0.8 // 20% headroom

	// Ollama client
	client := ollama.NewClient(*ollamaURL)
	ctx := context.Background()

	ollamaVersion, err := client.Version(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: cannot reach Ollama at %s: %v\n", *ollamaURL, err)
		os.Exit(1)
	}
	fmt.Printf("Ollama: %s\n\n", ollamaVersion)

	// Cache key components
	fixtureData, _ := os.ReadFile(eventSource)
	cacheKeyBase := fmt.Sprintf("%s:%s", systemPromptHash(), string(fixtureData))

	resultCache := cache.New(*resultsPath)

	// Select models to run
	models := m.Included()
	if *singleModel != "" {
		var filtered []manifest.Model
		for _, model := range models {
			if model.Name == *singleModel {
				filtered = append(filtered, model)
			}
		}
		if len(filtered) == 0 {
			fmt.Fprintf(os.Stderr, "error: model %q not found in manifest\n", *singleModel)
			os.Exit(1)
		}
		models = filtered
	}

	var allResults []types.ModelResult
	completedCount := 0

	for _, model := range models {
		fmt.Printf("  %-35s", model.Name)

		// VRAM filter
		if model.VRAMgb > maxVRAM {
			result := types.ModelResult{
				Model:  model.Name,
				VRAMGB: model.VRAMgb,
				Tier:   model.Tier,
				Vendor: model.Vendor,
				Score: types.Score{
					Verdict:       types.VerdictSkippedVRAM,
					VerdictReason: fmt.Sprintf("requires %.1fGB VRAM, available %.1fGB (with headroom)", model.VRAMgb, maxVRAM),
				},
			}
			fmt.Printf("SKIP (%.1fGB VRAM > %.1fGB available)\n", model.VRAMgb, maxVRAM)
			allResults = append(allResults, result)
			continue
		}

		// Cache check
		cacheKey := cacheKeyFor(cacheKeyBase, ollamaVersion, model.Name)
		if !*force {
			if cached, ok, err := resultCache.Read(model.Name, cacheKey, 24*time.Hour); err == nil && ok {
				fmt.Printf("cached  verdict=%-15s composite=%.2f\n", cached.Score.Verdict, cached.Score.Composite)
				allResults = append(allResults, cached)
				completedCount++
				continue
			}
		}

		// Run
		runResult, err := runner.Run(ctx, client, model, eventSource, *numRuns)
		if err != nil {
			fmt.Printf("ERROR: %v\n", err)
			continue
		}

		score := scorer.Score(runResult)
		modelResult := types.ModelResult{
			Model:  model.Name,
			VRAMGB: model.VRAMgb,
			Tier:   model.Tier,
			Vendor: model.Vendor,
			Score:  score,
		}

		fmt.Printf("%-15s patterns=%-3d latency=%s\n",
			score.Verdict, score.ValidPatterns, score.AvgLatency.Std().Round(time.Second))

		// Write to cache after each model
		_ = resultCache.Write(model.Name, cacheKey, modelResult)
		allResults = append(allResults, modelResult)
		completedCount++
	}

	if completedCount == 0 {
		fmt.Fprintln(os.Stderr, "error: no models completed")
		os.Exit(2)
	}

	// Generate documentation
	info := reporter.RunInfo{
		OllamaVersion: ollamaVersion,
		OS:            runtime.GOOS + "/" + runtime.GOARCH,
		GPU:           gpuInfo,
		BinaryVersion: Version,
		Timestamp:     time.Now().UTC(),
	}

	md := reporter.RenderMarkdown(allResults, m, info)
	if err := os.WriteFile(*docsPath, []byte(md), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "writing docs: %v\n", err)
	} else {
		fmt.Printf("\nWrote %s\n", *docsPath)
	}

	svgContent := reporter.RenderSVG(allResults)
	os.MkdirAll(filepath.Dir(*svgPath), 0755)
	if err := os.WriteFile(*svgPath, []byte(svgContent), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "writing SVG: %v\n", err)
	} else {
		fmt.Printf("Wrote %s\n", *svgPath)
	}

	fmt.Printf("Done. Results cached at %s\n", *resultsPath)
}

func systemPromptHash() string {
	h := sha256.Sum256([]byte(runner.SystemPrompt()))
	return fmt.Sprintf("%x", h[:8])
}

func cacheKeyFor(base, ollamaVersion, modelName string) string {
	h := sha256.Sum256([]byte(base + ":" + ollamaVersion + ":" + modelName))
	return fmt.Sprintf("%x", h[:16])
}
```

- [ ] **Step 2: Export SystemPrompt from runner**

Add to `internal/runner/runner.go`:

```go
// SystemPrompt returns the shared system prompt. Exported for cache key computation.
func SystemPrompt() string { return systemPrompt }
```

- [ ] **Step 3: Build and verify**

```bash
cd ~/projects/instinct && go build ./cmd/benchmark/...
```

Expected: binary created with no errors

- [ ] **Step 4: Run dry-run**

```bash
cd ~/projects/instinct && go run ./cmd/benchmark --dry-run
```

Expected:
```
instinct-benchmark dev
Manifest: 14 included, 4 excluded
Dry run complete — manifest valid.
```

- [ ] **Step 5: Run full tests**

```bash
cd ~/projects/instinct && go test ./... 2>&1 | tail -15
```

Expected: all packages pass

- [ ] **Step 6: Commit**

```bash
git add cmd/ internal/runner/runner.go
git commit -m "feat(cmd/benchmark): CLI entry point with VRAM filter, cache, full output pipeline"
```

---

## Task 12: CI Workflow

**Files:**
- Create: `.github/workflows/benchmark.yml`

- [ ] **Step 1: Create workflow**

Create `.github/workflows/benchmark.yml`:

```yaml
name: benchmark

on:
  pull_request:
    paths:
      - "models/candidates.yaml"
  workflow_dispatch:

jobs:
  validate:
    name: Validate manifest
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: "1.22"
      - name: Build benchmark binary
        run: go build ./cmd/benchmark/...
      - name: Validate manifest (dry-run)
        run: go run ./cmd/benchmark --dry-run
      - name: Post manifest summary
        if: github.event_name == 'pull_request'
        run: |
          echo "### Manifest validation passed" >> $GITHUB_STEP_SUMMARY
          echo "$(go run ./cmd/benchmark --dry-run 2>&1)" >> $GITHUB_STEP_SUMMARY
```

- [ ] **Step 2: Commit**

```bash
git add .github/
git commit -m "ci: benchmark manifest validation on PR to candidates.yaml"
```

---

## Task 13: End-to-End Verification

- [ ] **Step 1: Run full test suite**

```bash
cd ~/projects/instinct && go test ./... -v 2>&1 | grep -E "^(ok|FAIL|---)" | head -30
```

Expected: all packages `ok`

- [ ] **Step 2: Run benchmark against a single model**

```bash
cd ~/projects/instinct && go run ./cmd/benchmark --model mistral:7b --runs 1
```

Expected: produces output, writes `docs/models.md` and `docs/assets/svg/model-tiers.svg`

- [ ] **Step 3: Verify generated docs**

```bash
head -10 docs/models.md && echo "---" && wc -l docs/models.md
```

Expected: generation comment header present, file > 30 lines

- [ ] **Step 4: Run benchmark --dry-run**

```bash
go run ./cmd/benchmark --dry-run
```

Expected: `Dry run complete — manifest valid.` exit 0

- [ ] **Step 5: Final commit + tag**

```bash
cd ~/projects/instinct
git add docs/ 
git commit -m "docs: initial generated model compatibility documentation"
git tag v0.1.0-benchmark
```

---

## Self-Review

**Spec coverage check:**

| Spec requirement | Task |
|-----------------|------|
| candidates.yaml manifest with full schema | Task 2 |
| Validation: instruct, VRAM limit, duplicates, max 25 | Task 2 |
| Initial 14-model candidate list + 4 exclusions | Task 2 |
| VRAM auto-detection: nvidia/amd/apple/fallback | Task 3 |
| Ollama pull with 1MB scanner buffer | Task 4 |
| PullFailed vs TimedOut verdicts | Task 4, 9 |
| Synthetic fixture with one of each pattern type | Task 5 |
| Thinking leak markers per family | Task 6 |
| Pure scorer, separate package | Task 7 |
| Verdict tree: Recommended/Usable/NotRecommended/Failed/TimedOut | Task 7 |
| Composite score for tie-breaking | Task 7 |
| sha256 cache key with prompt+fixture+version+digest | Task 8 |
| Duration as human-readable JSON | Task 1 |
| Run aggregation: median patterns, mean latency | Task 7, 9 |
| Model eviction after runs | Task 9 |
| 300s timeout per run | Task 9 |
| Markdown with Recommended/Usable/NotRecommended/Rejected sections | Task 10 |
| Suggested default derived from results, not const | Task 10, 11 |
| SVG tier diagram with instinct palette (750x420) | Task 10 |
| CLI flags: --model, --dry-run, --runs, --force, --use-live-buffer | Task 11 |
| VRAM filter with 20% headroom | Task 11 |
| Exit codes 0/1/2 | Task 11 |
| Soft cap 25 models enforced at load | Task 2 |
| CI dry-run on PR to candidates.yaml | Task 12 |
| Sequential execution comment | Task 9 |
| Binary version via ldflags | Task 11 |

**Placeholder scan:** No TBDs. All code blocks complete.

**Type consistency:** `manifest.Model` used consistently in runner and main. `types.ModelResult` flows from scorer → cache → reporter → main. `ollama.ChatRequest.Format` uses `json.RawMessage` throughout.
