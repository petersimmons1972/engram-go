package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSetupDocsMatchNetworkDefaultContract(t *testing.T) {
	tests := []struct {
		name         string
		path         string
		mustContain  []string
		mustNotMatch []string
	}{
		{
			name: "root README",
			path: filepath.Join("..", "..", "README.md"),
			mustContain: []string{
				"`make setup` targets the default home-network endpoint `https://engram.petersimmons.com`",
				"go run ./cmd/engram-setup --url http://127.0.0.1:8788",
				"`~/.config/engram/api_key`",
				"`~/projects/engram-go/.env`",
				"`~/.claude/mcp_servers.json`",
				"`~/.claude.json`",
			},
			mustNotMatch: []string{
				"make setup\n```\n\nBoth setups expose the server at `http://localhost:8788`",
			},
		},
		{
			name: "command README",
			path: filepath.Join("..", "README.md"),
			mustContain: []string{
				"Configure with default home-network endpoint (`https://engram.petersimmons.com`)",
				"go run ./cmd/engram-setup --url http://127.0.0.1:8788",
				"`~/.config/engram/api_key`",
				"`~/projects/engram-go/.env`",
				"`~/.claude/mcp_servers.json`",
				"`~/.claude.json`",
			},
			mustNotMatch: []string{
				"Configure with defaults (localhost:8788)",
			},
		},
		{
			name: "getting started",
			path: filepath.Join("..", "..", "docs", "getting-started.md"),
			mustContain: []string{
				"`make setup` targets the default home-network endpoint `https://engram.petersimmons.com`",
				"go run ./cmd/engram-setup --url http://127.0.0.1:8788",
				"`~/.claude/mcp_servers.json`",
				"`~/.claude.json`",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw, err := os.ReadFile(tt.path)
			if err != nil {
				t.Fatalf("read %s: %v", tt.path, err)
			}
			doc := string(raw)
			for _, want := range tt.mustContain {
				if !strings.Contains(doc, want) {
					t.Errorf("%s missing %q", tt.path, want)
				}
			}
			for _, bad := range tt.mustNotMatch {
				if strings.Contains(doc, bad) {
					t.Errorf("%s still contains outdated setup contract %q", tt.path, bad)
				}
			}
		})
	}
}
