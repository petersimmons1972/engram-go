package longmemeval

import (
	"os"
	"strings"
	"testing"
)

// TestNeedsChatTemplateKwargs — #671: the chat_template_kwargs JSON field
// should ONLY be sent for models that understand the enable_thinking key
// (currently vLLM Nemotron). For OpenAI-compatible endpoints serving other
// models, sending an unknown extension field can return HTTP 400.
func TestNeedsChatTemplateKwargs(t *testing.T) {
	cases := []struct {
		model string
		want  bool
	}{
		// vLLM Nemotron family — send the field.
		{"nvidia/Nemotron-3-Nano-Omni-30B-A3B-Reasoning-NVFP4", true},
		{"nvidia/Nemotron-3-Nano-Omni", true},
		{"nemotron-3", true},
		// Other models — DO NOT send.
		{"gpt-4o-mini", false},
		{"meta-llama/Llama-3.1-70B-Instruct", false},
		{"BAAI/bge-m3", false},
		{"qwen2.5-72b", false},
		{"", false},
	}
	for _, c := range cases {
		got := needsChatTemplateKwargs(c.model)
		if got != c.want {
			t.Errorf("needsChatTemplateKwargs(%q) = %v, want %v", c.model, got, c.want)
		}
	}
}

// TestBuildOAIRequest_OmitsKwargsForNonNemotron — assert the marshaled JSON
// does NOT contain "chat_template_kwargs" when the model is not in the
// vLLM family.
func TestBuildOAIRequest_OmitsKwargsForNonNemotron(t *testing.T) {
	body, err := buildOAIRequestBody("gpt-4o-mini", "prompt")
	if err != nil {
		t.Fatalf("buildOAIRequestBody: %v", err)
	}
	if strings.Contains(string(body), "chat_template_kwargs") {
		t.Errorf("non-Nemotron model body must NOT contain chat_template_kwargs: %s", body)
	}
}

func TestBuildOAIRequest_IncludesKwargsForNemotron(t *testing.T) {
	body, err := buildOAIRequestBody("nvidia/Nemotron-3-Nano-Omni-30B-A3B-Reasoning-NVFP4", "prompt")
	if err != nil {
		t.Fatalf("buildOAIRequestBody: %v", err)
	}
	if !strings.Contains(string(body), "chat_template_kwargs") {
		t.Errorf("Nemotron model body must contain chat_template_kwargs: %s", body)
	}
	if !strings.Contains(string(body), `"enable_thinking":false`) {
		t.Errorf("expected enable_thinking:false in Nemotron body: %s", body)
	}
}

// TestNoHTTPDefaultClient — #687: callOAI must use the private oaiHTTPClient,
// not http.DefaultClient. The DefaultClient is a global singleton whose
// Transport can be mutated by any imported package's init().
func TestNoHTTPDefaultClient(t *testing.T) {
	src, err := os.ReadFile("claude.go")
	if err != nil {
		t.Fatalf("read claude.go: %v", err)
	}
	text := string(src)
	if strings.Contains(text, "http.DefaultClient.Do(") {
		t.Errorf("claude.go uses http.DefaultClient.Do — must use private client (#687)")
	}
}

// TestIsValidClaudeModel — #678: the `model` argument to `claude --print
// --model <model>` is currently not validated. An attacker-controlled or
// LLM-hallucinated value could include argv-injection (e.g. starting with
// "--") or unknown model names. We restrict to a strict allowlist.
func TestIsValidClaudeModel(t *testing.T) {
	cases := []struct {
		model string
		want  bool
	}{
		{"opus", true},
		{"sonnet", true},
		{"haiku", true},
		{"", false},
		{"--dangerously-skip-permissions", false},
		{"opus --foo", false},
		{"sonnet; rm -rf /", false},
		{"sonnet\n--evil", false},
		{"unknown-model", false},
		{"OPUS", false}, // case-sensitive — be strict
	}
	for _, c := range cases {
		got := isValidClaudeModel(c.model)
		if got != c.want {
			t.Errorf("isValidClaudeModel(%q) = %v, want %v", c.model, got, c.want)
		}
	}
}
