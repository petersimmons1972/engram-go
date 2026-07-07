package mcp

// atoms_handler.go — Milestone 1 (#938) /atoms REST endpoint.
//
// POST /atoms dispatches on the "action" field:
//   - action="store"  → insert atom + embedding (called by atom-build)
//   - (omitted/empty) → fetch active atoms for a project (called by --atom-mode via FetchAtoms)
//
// Authorization: Bearer <token> (handled by applyMiddleware before this handler runs).
//
// The handler type-asserts to *db.PostgresBackend for atom methods because they
// are not yet in the db.Backend interface (Milestone 1 minimal footprint).

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/petersimmons1972/engram/internal/atom"
	"github.com/petersimmons1972/engram/internal/db"
)

const maxAtomsRequestBodyBytes = 2 * 1024 * 1024

// atomsRequest is the unified request body for POST /atoms.
type atomsRequest struct {
	Action    string     `json:"action"`
	Project   string     `json:"project"`
	AtomType  string     `json:"atom_type"`
	TopK      int        `json:"top_k"`
	Atom      *atom.Atom `json:"atom"`
	Embedding []float32  `json:"embedding"`
}

// handleAtoms dispatches POST /atoms requests for Milestone 1 atom recall and storage.
func (s *Server) handleAtoms(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req atomsRequest
	if err := decodeJSONBodyBounded(w, r, maxAtomsRequestBodyBytes, &req); err != nil {
		if errors.Is(err, errRequestBodyTooLarge) {
			writeJSONError(w, http.StatusRequestEntityTooLarge, "request body too large")
			return
		}
		writeJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Project == "" {
		writeJSONError(w, http.StatusBadRequest, "project is required")
		return
	}
	if req.Action == "store" && s.cfg.EmbedDimensions > 0 && len(req.Embedding) > s.cfg.EmbedDimensions {
		writeJSONError(w, http.StatusBadRequest,
			fmt.Sprintf("embedding length %d exceeds configured EmbedDimensions %d", len(req.Embedding), s.cfg.EmbedDimensions))
		return
	}

	// Get a pool handle to access the backend for this project.
	h, err := s.pool.Get(r.Context(), req.Project)
	if err != nil {
		slog.Error("atoms: pool.Get failed", "project", req.Project, "err", err)
		writeJSONError(w, http.StatusInternalServerError, "backend unavailable")
		return
	}

	// Assert to PostgresBackend for atom methods.
	// On non-Postgres backends (test stubs), we return 501 Not Implemented.
	pg, ok := h.Engine.Backend().(*db.PostgresBackend)
	if !ok {
		writeJSONError(w, http.StatusNotImplemented, "atom operations require PostgresBackend")
		return
	}

	switch req.Action {
	case "store":
		s.handleAtomStore(w, r, pg, &req)
	default:
		// No action or unrecognised → fetch (GET-like via POST for FetchAtoms compatibility).
		s.handleAtomFetch(w, r, pg, req.Project, req.AtomType, req.TopK)
	}
}

// handleAtomStore inserts one atom and its embedding into the database.
func (s *Server) handleAtomStore(w http.ResponseWriter, r *http.Request, pg *db.PostgresBackend, req *atomsRequest) {
	if req.Atom == nil {
		writeJSONError(w, http.StatusBadRequest, "atom is required for action=store")
		return
	}
	req.Atom.Project = req.Project

	if insErr := pg.InsertAtom(r.Context(), req.Atom); insErr != nil {
		slog.Error("atoms store: InsertAtom failed", "project", req.Project, "err", insErr)
		writeJSONError(w, http.StatusInternalServerError, "InsertAtom failed")
		return
	}

	if len(req.Embedding) > 0 {
		if embErr := pg.InsertAtomEmbedding(r.Context(), req.Atom.ID, req.Embedding); embErr != nil {
			// Non-fatal: atom row is stored; embedding miss is recoverable on re-run.
			slog.Warn("atoms store: InsertAtomEmbedding failed (non-fatal)",
				"atom_id", req.Atom.ID, "err", embErr)
		}
	}

	writeJSON(w, http.StatusCreated, map[string]string{"id": req.Atom.ID})
}

// handleAtomFetch returns active atoms for project, filtered by atomType, capped to topK.
func (s *Server) handleAtomFetch(w http.ResponseWriter, r *http.Request, pg *db.PostgresBackend, project, atomType string, topK int) {
	atoms, err := pg.GetActiveAtoms(r.Context(), project, atomType)
	if err != nil {
		slog.Error("atoms fetch: GetActiveAtoms failed", "project", project, "err", err)
		writeJSONError(w, http.StatusInternalServerError, "GetActiveAtoms failed")
		return
	}

	// Apply topK cap (safety: never return more than 1000 atoms in one call).
	const hardCap = 1000
	if topK <= 0 || topK > hardCap {
		topK = hardCap
	}
	if len(atoms) > topK {
		atoms = atoms[:topK]
	}

	writeJSON(w, http.StatusOK, map[string]any{"atoms": atoms})
}
