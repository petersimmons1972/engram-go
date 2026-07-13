package main

import (
	"context"
	"fmt"
	"log"
	"sort"
	"strings"

	"github.com/petersimmons1972/engram/internal/atom"
)

const chronoLedgerLineCap = 40

// chronoLedgerAtomFetcher is the project chronology seam used by B4.
type chronoLedgerAtomFetcher interface {
	FetchChronoLedgerAtoms(ctx context.Context, project string, limit int) ([]atom.Atom, error)
}

type chronoLedgerRunRequest struct {
	enabled      bool
	questionType string
	project      string
	questionID   string
}

func injectChronoLedger(
	ctx context.Context,
	client chronoLedgerAtomFetcher,
	req chronoLedgerRunRequest,
	contextBlocks []string,
) ([]string, error) {
	ledger, err := loadChronoLedger(ctx, req.enabled, req.questionType, client, req.project)
	if err != nil {
		return nil, err
	}
	if shouldFetchChronoLedger(req.enabled, req.questionType) && ledger == "" {
		log.Printf("DEBUG run [%s] chronology ledger found no event atoms", req.questionID)
	}
	return prependChronoLedger(contextBlocks, ledger), nil
}

// loadChronoLedger fetches the full project's ordered chronology prefix for
// the opt-in B4 timeline. It deliberately does not use B3's question-windowed
// event recall.
func loadChronoLedger(
	ctx context.Context,
	enabled bool,
	questionType string,
	client chronoLedgerAtomFetcher,
	project string,
) (string, error) {
	if !shouldFetchChronoLedger(enabled, questionType) {
		return "", nil
	}

	atoms, err := client.FetchChronoLedgerAtoms(ctx, project, chronoLedgerLineCap+1)
	if err != nil {
		return "", fmt.Errorf("fetch project chronology: %w", err)
	}
	return formatChronoLedger(atoms), nil
}

func shouldFetchChronoLedger(enabled bool, questionType string) bool {
	return enabled && questionType == "temporal-reasoning"
}

// prependChronoLedger adds a non-empty timeline before every other context
// block without mutating the caller's slice.
func prependChronoLedger(contextBlocks []string, ledger string) []string {
	if ledger == "" {
		return contextBlocks
	}
	combined := make([]string, 0, len(contextBlocks)+1)
	combined = append(combined, ledger)
	return append(combined, contextBlocks...)
}

// formatChronoLedger renders dated event and status-change atoms in ascending
// valid_from order. Duplicate rendered entries are removed before the oldest
// 40 entries are selected so irrelevant or repeated atoms cannot consume the
// prompt budget.
func formatChronoLedger(atoms []atom.Atom) string {
	dated := make([]atom.Atom, 0, len(atoms))
	seen := make(map[chronoLedgerKey]struct{}, len(atoms))
	for _, candidate := range atoms {
		if candidate.ValidFrom == nil || !isChronoLedgerType(candidate.Type) {
			continue
		}
		key := newChronoLedgerKey(candidate)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		dated = append(dated, candidate)
	}
	if len(dated) == 0 {
		return ""
	}

	sort.SliceStable(dated, func(i, j int) bool {
		left := dated[i]
		right := dated[j]
		switch {
		case !left.ValidFrom.Equal(*right.ValidFrom):
			return left.ValidFrom.Before(*right.ValidFrom)
		case left.Statement != right.Statement:
			return left.Statement < right.Statement
		case left.Type != right.Type:
			return left.Type < right.Type
		case left.ID != right.ID:
			return left.ID < right.ID
		default:
			return left.Supersedes < right.Supersedes
		}
	})

	omitted := len(dated) - chronoLedgerLineCap
	if omitted > 0 {
		dated = dated[:chronoLedgerLineCap]
	}

	var b strings.Builder
	b.WriteString("=== Event Timeline (chronological) ===\n")
	for _, event := range dated {
		annotation := "[current]"
		if event.ValidTo != nil {
			annotation = fmt.Sprintf("[superseded %s]", event.ValidTo.Format("2006-01-02"))
		}
		fmt.Fprintf(
			&b,
			"%s: %s %s\n",
			event.ValidFrom.Format("2006-01-02"),
			event.Statement,
			annotation,
		)
	}
	if omitted > 0 {
		b.WriteString("(+more events truncated)\n")
	}
	return b.String()
}

func isChronoLedgerType(atomType string) bool {
	switch atomType {
	case atom.TypeEvent, atom.TypeStatusChange:
		return true
	default:
		return false
	}
}

type chronoLedgerKey struct {
	validFrom string
	validTo   string
	statement string
}

func newChronoLedgerKey(candidate atom.Atom) chronoLedgerKey {
	key := chronoLedgerKey{
		validFrom: candidate.ValidFrom.Format("2006-01-02"),
		statement: candidate.Statement,
	}
	if candidate.ValidTo != nil {
		key.validTo = candidate.ValidTo.Format("2006-01-02")
	}
	return key
}
