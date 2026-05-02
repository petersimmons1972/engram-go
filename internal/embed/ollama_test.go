package embed_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/petersimmons1972/engram/internal/embed"
	"github.com/stretchr/testify/require"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func resp(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body))}
}

func newTestClient(t *testing.T, model string, rt http.RoundTripper) (*embed.OllamaClient, error) {
	t.Helper()
	return embed.NewOllamaClientWithTransport(context.Background(), "http://ollama", model, rt)
}

func TestNewOllamaClient_ModelPresent(t *testing.T) {
	c, err := newTestClient(t, "nomic-embed-text", roundTripFunc(func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case "/api/tags":
			return resp(http.StatusOK, `{"models":[{"name":"nomic-embed-text:latest"}]}`), nil
		case "/api/embed":
			return resp(http.StatusOK, `{"embeddings":[[0,1,2]]}`), nil
		default:
			return resp(http.StatusNotFound, ""), nil
		}
	}))
	require.NoError(t, err)
	require.Equal(t, "nomic-embed-text", c.Name())
}

func TestNewOllamaClient_ModelAbsent_TriggersPull(t *testing.T) {
	_, err := newTestClient(t, "nomic-embed-text", roundTripFunc(func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case "/api/tags":
			return resp(http.StatusOK, `{"models":[]}`), nil
		case "/api/pull":
			return resp(http.StatusOK, `{"status":"success"}`), nil
		case "/api/embed":
			return resp(http.StatusOK, `{"embeddings":[[0,1,2]]}`), nil
		default:
			return resp(http.StatusNotFound, ""), nil
		}
	}))
	require.NoError(t, err)
}

func TestOllamaClient_Embed(t *testing.T) {
	c, err := newTestClient(t, "nomic-embed-text", roundTripFunc(func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case "/api/tags":
			return resp(http.StatusOK, `{"models":[{"name":"nomic-embed-text:latest"}]}`), nil
		case "/api/embed":
			return resp(http.StatusOK, `{"embeddings":[[0,1,2,3]]}`), nil
		default:
			return resp(http.StatusNotFound, ""), nil
		}
	}))
	require.NoError(t, err)

	vec, err := c.Embed(context.Background(), "hello world")
	require.NoError(t, err)
	require.Len(t, vec, 4)
	require.Equal(t, 4, c.Dimensions())
}

func TestNewOllamaClient_RejectsBadJSON(t *testing.T) {
	_, err := newTestClient(t, "nomic-embed-text", roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/api/tags" {
			return resp(http.StatusOK, `not-json`), nil
		}
		return resp(http.StatusOK, `{"embeddings":[[0]]}`), nil
	}))
	require.Error(t, err)
}

func TestNewOllamaClient_RequestBody(t *testing.T) {
	var seen map[string]any
	c, err := newTestClient(t, "nomic-embed-text", roundTripFunc(func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case "/api/tags":
			return resp(http.StatusOK, `{"models":[{"name":"nomic-embed-text:latest"}]}`), nil
		case "/api/embed":
			require.NoError(t, json.NewDecoder(r.Body).Decode(&seen))
			return resp(http.StatusOK, `{"embeddings":[[0,1,2]]}`), nil
		default:
			return resp(http.StatusNotFound, ""), nil
		}
	}))
	require.NoError(t, err)
	_, err = c.Embed(context.Background(), "hello")
	require.NoError(t, err)
	require.Equal(t, "nomic-embed-text", seen["model"])
}
