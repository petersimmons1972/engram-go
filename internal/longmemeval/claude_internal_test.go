package longmemeval

import (
	"strings"
	"testing"
)

func TestSanitizeOAIDebugBodyRedactsSensitiveContent(t *testing.T) {
	body := []byte(`{"error":"bad request","prompt":"private memory context","api_key":"sk-test-secret","messages":[{"content":"user profile details"}]}`)

	got := sanitizeOAIDebugBody(body)

	for _, sensitive := range []string{
		"private memory context",
		"sk-test-secret",
		"user profile details",
		`"prompt"`,
		`"messages"`,
	} {
		if strings.Contains(got, sensitive) {
			t.Fatalf("sanitized debug body leaked %q in %q", sensitive, got)
		}
	}
	if !strings.Contains(got, "bytes=") {
		t.Fatalf("sanitized debug body should retain safe size metadata, got %q", got)
	}
}
