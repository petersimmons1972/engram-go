package internal

import (
	"encoding/json"
	"log/slog"
	"net/http"
)

type Server struct {
	k8s   *K8sClient
	store *Store
}

func NewServer(k8s *K8sClient, store *Store) *Server {
	return &Server{k8s: k8s, store: store}
}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", s.health)
	mux.HandleFunc("GET /policy/{hostname}", s.getPolicy)
	mux.HandleFunc("POST /status/{hostname}", s.postStatus)
	mux.HandleFunc("GET /registry", s.registry)
	return mux
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"status":"ok"}`))
}

func (s *Server) getPolicy(w http.ResponseWriter, r *http.Request) {
	hostname := r.PathValue("hostname")
	policy, err := s.k8s.GetPolicy(r.Context(), hostname)
	if err != nil {
		slog.Error("get policy", "hostname", hostname, "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if policy == nil {
		http.Error(w, "no policy for host", http.StatusNotFound)
		return
	}
	jsonOK(w, policy)
}

func (s *Server) postStatus(w http.ResponseWriter, r *http.Request) {
	hostname := r.PathValue("hostname")
	var report StatusReport
	if err := json.NewDecoder(r.Body).Decode(&report); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	report.Hostname = hostname // enforce from path
	s.store.Set(report)
	slog.Info("status received", "hostname", hostname, "policy_version", report.PolicyVersion, "containers", len(report.Containers))
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) registry(w http.ResponseWriter, r *http.Request) {
	policies, err := s.k8s.ListPolicies(r.Context())
	if err != nil {
		slog.Error("list policies", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	entries := make([]RegistryEntry, 0, len(policies))
	for _, p := range policies {
		entry := RegistryEntry{
			Host:          p.Spec.Host,
			PolicyVersion: p.PolicyVersion,
			Models:        p.Spec.Models,
		}
		if status, ok := s.store.Get(p.Spec.Host); ok {
			entry.LastSeen = status.ReportedAt
			entry.Containers = status.Containers
			entry.DegradedContainers = status.DegradedContainers
		}
		entries = append(entries, entry)
	}
	jsonOK(w, entries)
}

func jsonOK(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("json encode", "err", err)
	}
}
