package longmemeval

import "testing"

// TestNewRestClient_NormalizesBaseURL guards #1185: the REST endpoints
// (/quick-store, /atoms, /quick-recall) live at the server root. Callers commonly
// pass the MCP endpoint URL (.../mcp) from the Claude config; if that suffix is
// not stripped, QuickStore POSTs to /mcp/quick-store, which returned a bare
// number and failed decoding with the opaque "cannot unmarshal number" error.
func TestNewRestClient_NormalizesBaseURL(t *testing.T) {
	cases := map[string]string{
		"https://engram.test/mcp":  "https://engram.test",
		"https://engram.test/mcp/": "https://engram.test",
		"https://engram.test/sse":  "https://engram.test",
		"https://engram.test/":     "https://engram.test",
		"https://engram.test":      "https://engram.test",
	}
	for in, want := range cases {
		c := NewRestClient(in, "tok")
		if c.baseURL != want {
			t.Errorf("NewRestClient(%q).baseURL = %q, want %q", in, c.baseURL, want)
		}
	}
}
