package skillscheck

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

// TestSkillFrontmatterParses requires every skills/*/SKILL.md to carry
// frontmatter that a strict YAML parser accepts, with non-empty name and
// description. An unquoted ": " in a description is invalid YAML and can
// leave a skill registered with no trigger text.
func TestSkillFrontmatterParses(t *testing.T) {
	files, err := filepath.Glob("../../skills/*/SKILL.md")
	if err != nil || len(files) == 0 {
		t.Fatalf("no skills found (err=%v)", err)
	}
	for _, f := range files {
		raw, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		parts := bytes.SplitN(raw, []byte("---\n"), 3)
		if len(parts) < 3 || len(bytes.TrimSpace(parts[0])) != 0 {
			t.Errorf("%s: missing --- frontmatter block", f)
			continue
		}
		var fm map[string]any
		if err := yaml.Unmarshal(parts[1], &fm); err != nil {
			t.Errorf("%s: frontmatter is not valid YAML: %v", f, err)
			continue
		}
		for _, k := range []string{"name", "description"} {
			if s, _ := fm[k].(string); s == "" {
				t.Errorf("%s: frontmatter %q missing or empty", f, k)
			}
		}
		if n, _ := fm["name"].(string); n != "" && n != filepath.Base(filepath.Dir(f)) {
			t.Errorf("%s: name %q does not match directory", f, n)
		}
	}
}
