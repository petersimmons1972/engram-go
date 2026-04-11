// Package markdown provides import/export of memories as markdown files.
package markdown

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/petersimmons1972/engram/internal/types"
)

// Export writes each memory to a separate .md file in dir.
// File name: <id>.md
func Export(memories []*types.Memory, dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	for _, m := range memories {
		if m == nil || m.ID == "" {
			continue
		}
		if err := writeMemory(m, filepath.Join(dir, m.ID+".md")); err != nil {
			return fmt.Errorf("export %s: %w", m.ID, err)
		}
	}
	return nil
}

func writeMemory(m *types.Memory, path string) error {
	var sb strings.Builder
	sb.WriteString("---\n")
	sb.WriteString(fmt.Sprintf("id: %s\n", m.ID))
	sb.WriteString(fmt.Sprintf("memory_type: %s\n", m.MemoryType))
	sb.WriteString(fmt.Sprintf("importance: %d\n", m.Importance))
	if len(m.Tags) > 0 {
		sb.WriteString(fmt.Sprintf("tags: [%s]\n", strings.Join(m.Tags, ", ")))
	}
	sb.WriteString("---\n\n")
	sb.WriteString(m.Content)
	sb.WriteString("\n")
	return os.WriteFile(path, []byte(sb.String()), 0o644)
}

// Dump writes all memories as markdown files to dir (one per memory).
func Dump(memories []*types.Memory, dir string) error {
	return Export(memories, dir)
}

// ImportClaudeMD reads a CLAUDE.md file and returns a slice of memories,
// one per top-level section (## heading).
func ImportClaudeMD(path string) ([]*types.Memory, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("ImportClaudeMD %s: %w", path, err)
	}
	return splitSections(string(data)), nil
}

// Ingest reads all .md files in dir (recursively) and returns memories.
func Ingest(dir string) ([]*types.Memory, error) {
	var memories []*types.Memory
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			if d != nil && d.IsDir() {
				return nil // skip unreadable directories, continue walk
			}
			return err // abort on actual file-level errors
		}
		if d.IsDir() || !strings.HasSuffix(path, ".md") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		now := time.Now().UTC()
		m := &types.Memory{
			ID:           types.NewMemoryID(),
			Content:      stripFrontMatter(string(data)),
			MemoryType:   types.MemoryTypeContext,
			Importance:   2,
			StorageMode:  "document",
			CreatedAt:    now,
			UpdatedAt:    now,
			LastAccessed: now,
		}
		memories = append(memories, m)
		return nil
	})
	return memories, err
}

// stripFrontMatter removes YAML front matter (--- ... ---) from the beginning of s.
// If no front matter is present the input is returned unchanged.
func stripFrontMatter(s string) string {
	const delim = "---\n"
	if !strings.HasPrefix(s, delim) {
		return s
	}
	rest := s[len(delim):]
	idx := strings.Index(rest, "\n---\n")
	if idx < 0 {
		return s // malformed — leave as-is
	}
	return strings.TrimSpace(rest[idx+len("\n---\n"):])
}

// splitSections splits a markdown document on ## headings into per-section memories.
func splitSections(content string) []*types.Memory {
	var memories []*types.Memory
	var current strings.Builder
	var heading string

	scanner := bufio.NewScanner(strings.NewReader(content))
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "## ") {
			if current.Len() > 0 {
				memories = append(memories, sectionMemory(heading, current.String()))
				current.Reset()
			}
			heading = strings.TrimPrefix(line, "## ")
			continue
		}
		current.WriteString(line)
		current.WriteString("\n")
	}
	if current.Len() > 0 {
		memories = append(memories, sectionMemory(heading, current.String()))
	}
	return memories
}

func sectionMemory(heading, content string) *types.Memory {
	content = strings.TrimSpace(content)
	if heading != "" {
		content = heading + "\n\n" + content
	}
	now := time.Now().UTC()
	return &types.Memory{
		ID:           types.NewMemoryID(),
		Content:      content,
		MemoryType:   types.MemoryTypeContext,
		Importance:   2,
		StorageMode:  "focused",
		CreatedAt:    now,
		UpdatedAt:    now,
		LastAccessed: now,
	}
}
