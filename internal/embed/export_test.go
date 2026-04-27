// export_test.go exposes internal seams for use by the embed_test package.
// This file is compiled only during testing and does not affect the production binary.
package embed

import (
	"context"
	"net/http"
)

// NewOllamaClientWithTransport creates an OllamaClient using the supplied
// transport, bypassing the DNS-rebinding SSRF guard. For use in unit tests
// that need to point the client at a local httptest.Server.
func NewOllamaClientWithTransport(ctx context.Context, baseURL, model string, transport http.RoundTripper) (*OllamaClient, error) {
	return newOllamaClient(ctx, baseURL, model, 0, transport)
}
