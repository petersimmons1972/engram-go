package claude_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/petersimmons1972/engram/internal/claude"
	"github.com/stretchr/testify/require"
)

func TestNew_EmptyKey_Error(t *testing.T) {
	_, err := claude.New("")
	require.Error(t, err)
}

func TestClient_Complete_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"content": []map[string]string{
				{"type": "text", "text": "answer"},
			},
		})
	}))
	defer srv.Close()

	c, err := claude.New("test-key")
	require.NoError(t, err)
	c.BaseURL = srv.URL

	result, err := c.Complete(context.Background(), "sys", "prompt", "claude-sonnet-4-6", "claude-opus-4-6", 2, 1024)
	require.NoError(t, err)
	require.Equal(t, "answer", result)
}

func TestClient_Complete_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	c, err := claude.New("test-key")
	require.NoError(t, err)
	c.BaseURL = srv.URL

	_, err = c.Complete(context.Background(), "sys", "prompt", "claude-sonnet-4-6", "claude-opus-4-6", 2, 1024)
	require.Error(t, err)
	require.Contains(t, err.Error(), "401")
}

func TestClient_Complete_EmptyContent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"content": []map[string]string{},
		})
	}))
	defer srv.Close()

	c, err := claude.New("test-key")
	require.NoError(t, err)
	c.BaseURL = srv.URL

	_, err = c.Complete(context.Background(), "sys", "prompt", "claude-sonnet-4-6", "claude-opus-4-6", 2, 1024)
	require.Error(t, err)
}

func TestClient_Complete_PassesAdvisorTool(t *testing.T) {
	var capturedBody struct {
		Tools []struct {
			Type string `json:"type"`
			Name string `json:"name"`
		} `json:"tools"`
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&capturedBody)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"content": []map[string]string{
				{"type": "text", "text": "ok"},
			},
		})
	}))
	defer srv.Close()

	c, err := claude.New("test-key")
	require.NoError(t, err)
	c.BaseURL = srv.URL

	_, err = c.Complete(context.Background(), "sys", "prompt", "claude-sonnet-4-6", "claude-opus-4-6", 2, 1024)
	require.NoError(t, err)
	require.Len(t, capturedBody.Tools, 1)
	require.Equal(t, "advisor_20260301", capturedBody.Tools[0].Type)
	require.Equal(t, "advisor", capturedBody.Tools[0].Name)
}
