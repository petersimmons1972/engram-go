package main

// atom_mode.go — Milestone 1 (#938) atom-mode harness integration.
//
// fetchAtomContextBlock retrieves active preference atoms for a project and
// formats them as a labeled context block for injection into the generation
// prompt. This is the Milestone 1 code path; it is OFF by default (--atom-mode
// flag required) and should only be enabled after the post-reset atom
// extraction pass has been completed.
//
// The atom recall goes via the MCP memory_query tool (an atom-typed query).
// In Milestone 1 the server does not yet expose a dedicated atom endpoint;
// we use the client's existing FetchAtoms helper which will be upgraded in M2.

import (
	"context"
	"log"

	"github.com/petersimmons1972/engram/internal/atom"
	"github.com/petersimmons1972/engram/internal/longmemeval"
	"github.com/petersimmons1972/engram/internal/search"
)

// atomFetcher is the narrow interface fetchAtomContextBlock needs from the MCP
// client. Satisfied by *longmemeval.Client; declared here so the function is
// unit-testable with a stub.
type atomFetcher interface {
	FetchAtoms(ctx context.Context, project string, atomType string, topK int) ([]atom.Atom, error)
}

// fetchAtomContextBlock fetches active preference atoms for the project and
// returns them formatted as a prompt-ready string. Returns empty string on error
// (non-fatal — the run continues with memory-only context) or when no atoms are
// found. Logs a warning on error so the issue is visible in run output.
func fetchAtomContextBlock(ctx context.Context, client *longmemeval.Client, project, questionID string) string {
	const atomTopK = 50 // cap for Milestone 1 context budget

	atoms, err := client.FetchAtoms(ctx, project, atom.TypePreference, atomTopK)
	if err != nil {
		// Non-fatal: missing atom endpoint, connection error, etc.
		log.Printf("WARN run [%s] atom-mode fetch failed (non-fatal): %v", questionID, err)
		return ""
	}
	if len(atoms) == 0 {
		return ""
	}
	return search.FormatAtomsAsContext(atoms)
}
