package longmemeval

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestNewChronoLedgerAtomsRequest_IsProjectScopedAndExplicit(t *testing.T) {
	req, err := newChronoLedgerAtomsRequest(
		context.Background(),
		"http://engram.test/",
		"secret",
		"project-a",
		41,
	)
	if err != nil {
		t.Fatalf("newChronoLedgerAtomsRequest: %v", err)
	}
	if req.Method != http.MethodPost || req.URL.String() != "http://engram.test/atoms" {
		t.Fatalf("request = %s %s", req.Method, req.URL)
	}
	if got := req.Header.Get("Authorization"); got != "Bearer secret" {
		t.Fatalf("Authorization = %q", got)
	}
	var body struct {
		Action  string `json:"action"`
		Project string `json:"project"`
		TopK    int    `json:"top_k"`
	}
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		t.Fatalf("decode request: %v", err)
	}
	if body.Action != "fetch" || body.Project != "project-a" || body.TopK != 41 {
		t.Fatalf("request body = %+v", body)
	}
}

func TestDecodeChronoLedgerAtomsResponse_DistinguishesEmptyAndUnavailable(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		body    string
		wantLen int
		wantErr string
	}{
		{name: "empty project", status: http.StatusOK, body: `{"atoms":[]}`},
		{name: "missing endpoint", status: http.StatusNotFound, wantErr: "unexpected status 404"},
		{name: "unsupported backend", status: http.StatusNotImplemented, wantErr: "unexpected status 501"},
		{name: "one atom", status: http.StatusOK, body: `{"atoms":[{"id":"event-1"}]}`, wantLen: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := &http.Response{
				StatusCode: tt.status,
				Body:       io.NopCloser(strings.NewReader(tt.body)),
			}
			atoms, err := decodeChronoLedgerAtomsResponse(resp)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error = %v, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("decodeChronoLedgerAtomsResponse: %v", err)
			}
			if len(atoms) != tt.wantLen {
				t.Fatalf("atoms len = %d, want %d", len(atoms), tt.wantLen)
			}
		})
	}
}
