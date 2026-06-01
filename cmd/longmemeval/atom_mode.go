package main

// atom_mode.go — Milestone 1 (#938) atom-mode harness integration.
//
// fetchAtomContextBlock retrieves active preference atoms for a project and
// formats them as a labeled context block for injection into the generation
// prompt. This is the Milestone 1 code path; it is OFF by default (--atom-mode
// flag required) and should only be enabled after the post-reset atom
// extraction pass has been completed.
//
// Primary path: FetchAtoms calls POST /atoms on the Engram server.
// Fallback (--atom-cache-dir): when the server returns no atoms (e.g. /atoms
// not yet deployed), reads from a local JSON cache written by atom-build.
// The fallback enables measurement even before the server is updated.

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"path/filepath"

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
//
// atomCacheDir (if non-empty) is used as a fallback when the server returns no
// atoms: reads <atomCacheDir>/<project>.json written by atom-build.
func fetchAtomContextBlock(ctx context.Context, client *longmemeval.Client, project, questionID, atomCacheDir string) string {
	const atomTopK = 50 // cap for Milestone 1 context budget

	atoms, err := client.FetchAtoms(ctx, project, atom.TypePreference, atomTopK)
	if err != nil {
		// Non-fatal: missing atom endpoint, connection error, etc.
		log.Printf("WARN run [%s] atom-mode fetch failed (non-fatal): %v", questionID, err)
	}

	// Fallback to local cache when the server returned no atoms.
	if len(atoms) == 0 && atomCacheDir != "" {
		cached, cacheErr := readAtomCache(atomCacheDir, project, atom.TypePreference, atomTopK)
		if cacheErr != nil {
			log.Printf("WARN run [%s] atom-cache read failed: %v", questionID, cacheErr)
		} else if len(cached) > 0 {
			log.Printf("INFO run [%s] using atom-cache (%d atoms)", questionID, len(cached))
			atoms = cached
		}
	}

	if len(atoms) == 0 {
		return ""
	}
	return search.FormatAtomsAsContext(atoms)
}

// readAtomCache reads atoms from <dir>/<project>.json, filters by atomType, and caps at topK.
func readAtomCache(dir, project, atomType string, topK int) ([]atom.Atom, error) {
	path := filepath.Join(dir, project+".json")
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	var all []atom.Atom
	if err := json.NewDecoder(f).Decode(&all); err != nil {
		return nil, err
	}

	// Filter by atomType if specified.
	var out []atom.Atom
	for _, a := range all {
		if atomType == "" || a.Type == atomType {
			out = append(out, a)
		}
	}
	if topK > 0 && len(out) > topK {
		out = out[:topK]
	}
	return out, nil
}
