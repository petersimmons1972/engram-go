package mcp

import (
	"testing"
)

func TestIsOperationalConfig(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    bool
	}{
		// --- Should be filtered ---
		{
			name: "DNS arrow entry with IP",
			content: `Engram Memory
ollama → 192.168.1.50
OLLAMA_URL=http://ollama:11434`,
			want: true,
		},
		{
			name: "connection string section",
			content: `Database
postgresql://engram:secret@192.168.0.200:5432/engram
PGHOST=192.168.0.200:5432`,
			want: true,
		},
		{
			name: "IP port pairs and env URL",
			content: `Engram runs at 192.168.0.131:8788
ENGRAM_URL=http://192.168.0.131:8788/mcp`,
			want: true,
		},
		{
			name: "multiple connection strings",
			content: `Services:
- postgresql://user:pass@host:5432/db
- redis://192.168.0.1:6379
- http://ollama:11434/api`,
			want: true,
		},
		{
			name: "DNS arrow with http env var",
			content: `ollama -> 192.168.1.50
OLLAMA_URL=http://192.168.1.50:11434`,
			want: true,
		},
		// --- Should NOT be filtered ---
		{
			name:    "plain decision memory",
			content: "We decided to use summary-based recall instead of compression because compression reduces storage bytes but not LLM context window bytes.",
			want:    false,
		},
		{
			name:    "bug record with no infrastructure",
			content: "Bug: the background summarizer skips memories where summary = content, treating them as already summarized when they are not.",
			want:    false,
		},
		{
			name:    "pattern memory about K8s",
			content: "When scaling a StatefulSet to 0, also scale its associated Deployment to 0 to avoid CrashLoopBackOff from apps trying to reach a missing database.",
			want:    false,
		},
		{
			name:    "URL mentioned in passing",
			content: "The Engram MCP server documentation is at the project README. See the GitHub repo for details on the memory_recall API.",
			want:    false,
		},
		{
			name:    "single http URL not enough",
			content: "Documentation lives at https://github.com/petersimmons1972/engram-go — check there for the latest schema.",
			want:    false,
		},
		{
			name:    "empty content",
			content: "",
			want:    false,
		},
		{
			name:    "lesson learned no infra",
			content: "Compression optimizes storage bytes, not LLM context window bytes. These are different problems. To reduce context window cost, return less text: summaries instead of full content.",
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isOperationalConfig(tt.content)
			if got != tt.want {
				t.Errorf("isOperationalConfig() = %v, want %v\ncontent: %q", got, tt.want, tt.content)
			}
		})
	}
}
