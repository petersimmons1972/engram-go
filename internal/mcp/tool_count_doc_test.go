package mcp

// Regression guard for #665: docs/tools.md and README.md numbers must match
// the actual tool surface in registerTools() / hiddenToolNames().
//
// The docs embed parseable HTML count markers like:
//   <!-- count:visible-default -->17<!-- /count -->
//   <!-- count:hidden -->29<!-- /count -->
//   <!-- count:ai-enhanced -->4<!-- /count -->
//   <!-- count:total-callable-default -->46<!-- /count -->
// This test asserts every marker matches the value computed from code.

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"testing"
)

// canonical tool counts derived from code.
type toolCounts struct {
	unconditional         int // entries in the registry slice in registerTools()
	conditionalAIEnhanced int // tools added only when ClaudeEnabled
	hidden                int // entries in hiddenToolNames()
}

// computeToolCounts derives the canonical numbers by reading the source file.
// We parse instead of instantiating the server because instantiation requires
// a live DB pool, which is heavy for a unit test of a documentation invariant.
func computeToolCounts(t *testing.T) toolCounts {
	t.Helper()
	data, err := os.ReadFile("server.go")
	if err != nil {
		t.Fatalf("read server.go: %v", err)
	}
	src := string(data)

	// Count entries in the registry slice. Pattern: `\t\t{"memory_*",`
	registryEntry := regexp.MustCompile(`(?m)^\s+\{"memory_[a-z_]+",`)
	unconditional := len(registryEntry.FindAllString(src, -1))

	// Count entries in hiddenToolNames(). Pattern inside the func body.
	hiddenFnRe := regexp.MustCompile(`(?s)func hiddenToolNames\(\) map\[string\]bool \{.*?\n\}\n`)
	hiddenBlock := hiddenFnRe.FindString(src)
	if hiddenBlock == "" {
		t.Fatalf("could not locate hiddenToolNames body")
	}
	hiddenEntry := regexp.MustCompile(`(?m)^\s+"memory_[a-z_]+":\s+true,`)
	hidden := len(hiddenEntry.FindAllString(hiddenBlock, -1))

	// Count Claude-conditional registrations. Pattern: s.registerTool inside the
	// `if s.cfg.ClaudeEnabled {` block.
	claudeBlockRe := regexp.MustCompile(`(?s)if s\.cfg\.ClaudeEnabled \{(.*?)\}`)
	cb := claudeBlockRe.FindStringSubmatch(src)
	if len(cb) != 2 {
		t.Fatalf("could not locate ClaudeEnabled block")
	}
	claudeCallRe := regexp.MustCompile(`s\.registerTool\("memory_[a-z_]+"`)
	aiEnhanced := len(claudeCallRe.FindAllString(cb[1], -1))

	return toolCounts{
		unconditional:         unconditional,
		conditionalAIEnhanced: aiEnhanced,
		hidden:                hidden,
	}
}

// parseDocCounts reads a markdown file and returns every `<!-- count:NAME -->N<!-- /count -->`
// marker as a name→int map.
func parseDocCounts(t *testing.T, path string) map[string]int {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	re := regexp.MustCompile(`<!-- count:([a-z-]+) -->(\d+)<!-- /count -->`)
	out := map[string]int{}
	for _, m := range re.FindAllStringSubmatch(string(data), -1) {
		n, err := strconv.Atoi(m[2])
		if err != nil {
			t.Fatalf("non-numeric count %q in %s", m[2], path)
		}
		out[m[1]] = n
	}
	return out
}

// TestToolCountsMatchDocs verifies that every count marker in docs/tools.md
// and README.md matches the canonical value from code.
func TestToolCountsMatchDocs(t *testing.T) {
	c := computeToolCounts(t)
	visibleDefault := c.unconditional - c.hidden
	visibleWithAI := visibleDefault + c.conditionalAIEnhanced
	totalCallableDefault := c.unconditional
	totalCallableWithAI := c.unconditional + c.conditionalAIEnhanced

	t.Logf("canonical counts: unconditional=%d hidden=%d ai-enhanced=%d "+
		"visible-default=%d visible-with-ai=%d total-callable-default=%d total-callable-with-ai=%d",
		c.unconditional, c.hidden, c.conditionalAIEnhanced,
		visibleDefault, visibleWithAI, totalCallableDefault, totalCallableWithAI)

	expected := map[string]int{
		"unconditional":          c.unconditional,
		"hidden":                 c.hidden,
		"ai-enhanced":            c.conditionalAIEnhanced,
		"visible-default":        visibleDefault,
		"visible-with-ai":        visibleWithAI,
		"total-callable-default": totalCallableDefault,
		"total-callable-with-ai": totalCallableWithAI,
	}

	for _, doc := range []string{
		filepath.Join("..", "..", "docs", "tools.md"),
		filepath.Join("..", "..", "README.md"),
	} {
		got := parseDocCounts(t, doc)
		if len(got) == 0 {
			t.Errorf("%s has no count markers — add <!-- count:NAME -->N<!-- /count --> for at least one of: visible-default, hidden, ai-enhanced", doc)
			continue
		}
		for name, n := range got {
			want, ok := expected[name]
			if !ok {
				t.Errorf("%s: unknown count marker %q (allowed: unconditional, hidden, ai-enhanced, visible-default, visible-with-ai, total-callable-default, total-callable-with-ai)", doc, name)
				continue
			}
			if n != want {
				t.Errorf("%s: count:%s = %d, code says %d", doc, name, n, want)
			}
		}
	}
}
