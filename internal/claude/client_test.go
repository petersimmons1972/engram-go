package claude_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/petersimmons1972/engram/internal/claude"
	"github.com/stretchr/testify/require"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func jsonResp(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body))}
}

func TestNew_EmptyKey_Error(t *testing.T) {
	_, err := claude.New("")
	require.Error(t, err)
}

func TestClient_Complete_Success(t *testing.T) {
	c := claude.NewWithTransport("test-key", roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/v1/messages" {
			return jsonResp(http.StatusNotFound, ""), nil
		}
		return jsonResp(http.StatusOK, `{"content":[{"type":"text","text":"answer"}]}`), nil
	}))

	result, err := c.Complete(context.Background(), "sys", "prompt", "claude-sonnet-4-6", "claude-opus-4-6", 2, 1024)
	require.NoError(t, err)
	require.Equal(t, "answer", result)
}

func TestClient_Complete_HTTPError(t *testing.T) {
	c := claude.NewWithTransport("test-key", roundTripFunc(func(*http.Request) (*http.Response, error) {
		return jsonResp(http.StatusUnauthorized, ""), nil
	}))

	_, err := c.Complete(context.Background(), "sys", "prompt", "claude-sonnet-4-6", "claude-opus-4-6", 2, 1024)
	require.Error(t, err)
	require.Contains(t, err.Error(), "401")
}

func TestClient_Complete_EmptyContent(t *testing.T) {
	c := claude.NewWithTransport("test-key", roundTripFunc(func(*http.Request) (*http.Response, error) {
		return jsonResp(http.StatusOK, `{"content":[]}`), nil
	}))

	_, err := c.Complete(context.Background(), "sys", "prompt", "claude-sonnet-4-6", "claude-opus-4-6", 2, 1024)
	require.Error(t, err)
}

func TestClient_Complete_PassesAdvisorTool(t *testing.T) {
	var capturedBody struct {
		Tools []struct {
			Type    string `json:"type"`
			Name    string `json:"name"`
			Model   string `json:"model"`
			MaxUses int    `json:"max_uses"`
		} `json:"tools"`
	}
	c := claude.NewWithTransport("test-key", roundTripFunc(func(r *http.Request) (*http.Response, error) {
		require.Equal(t, "/v1/messages", r.URL.Path)
		require.NoError(t, json.NewDecoder(r.Body).Decode(&capturedBody))
		return jsonResp(http.StatusOK, `{"content":[{"type":"text","text":"ok"}]}`), nil
	}))

	_, err := c.Complete(context.Background(), "sys", "prompt", "claude-sonnet-4-6", "claude-opus-4-6", 2, 1024)
	require.NoError(t, err)
	require.Len(t, capturedBody.Tools, 1)
	require.Equal(t, "advisor_20260301", capturedBody.Tools[0].Type)
	require.Equal(t, "advisor", capturedBody.Tools[0].Name)
	require.Equal(t, "claude-opus-4-6", capturedBody.Tools[0].Model)
	require.Equal(t, 2, capturedBody.Tools[0].MaxUses)
}

func TestClient_Complete_RequestBuildError(t *testing.T) {
	c := claude.NewWithTransport("test-key", roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("transport broke")
	}))
	c.BaseURL = "://bad-url"
	_, err := c.Complete(context.Background(), "sys", "prompt", "claude-sonnet-4-6", "claude-opus-4-6", 2, 1024)
	require.Error(t, err)
}

func TestClient_Complete_ClaudeToolTypeFromEnv(t *testing.T) {
	tests := []struct {
		name       string
		envValue   string
		wantType   string
	}{
		{"default_when_unset", "", "advisor_20260301"},
		{"custom_from_env", "advisor_custom", "advisor_custom"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Save original env var
			original, wasSet := os.LookupEnv("ENGRAM_CLAUDE_TOOL_TYPE")
			defer func() {
				if wasSet {
					_ = os.Setenv("ENGRAM_CLAUDE_TOOL_TYPE", original)
				} else {
					_ = os.Unsetenv("ENGRAM_CLAUDE_TOOL_TYPE")
				}
			}()

			if tc.envValue == "" {
				_ = os.Unsetenv("ENGRAM_CLAUDE_TOOL_TYPE")
			} else {
				_ = os.Setenv("ENGRAM_CLAUDE_TOOL_TYPE", tc.envValue)
			}

			var capturedBody struct {
				Tools []struct {
					Type string `json:"type"`
				} `json:"tools"`
			}
			c := claude.NewWithTransport("test-key", roundTripFunc(func(r *http.Request) (*http.Response, error) {
				require.Equal(t, "/v1/messages", r.URL.Path)
				require.NoError(t, json.NewDecoder(r.Body).Decode(&capturedBody))
				return jsonResp(http.StatusOK, `{"content":[{"type":"text","text":"ok"}]}`), nil
			}))

			_, err := c.Complete(context.Background(), "sys", "prompt", "claude-sonnet-4-6", "claude-opus-4-6", 2, 1024)
			require.NoError(t, err)
			require.Len(t, capturedBody.Tools, 1)
			require.Equal(t, tc.wantType, capturedBody.Tools[0].Type)
		})
	}
}
