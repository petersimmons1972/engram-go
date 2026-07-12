package main

import (
	"fmt"
	"sort"
	"strings"

	"github.com/petersimmons1972/engram/internal/atom"
)

const chronoLedgerLineCap = 40

// prependChronoLedger adds a structured event chronology before every other
// context block. It is opt-in and scoped to temporal-reasoning questions.
func prependChronoLedger(
	enabled bool,
	questionType string,
	contextBlocks []string,
	atoms []atom.Atom,
) []string {
	if !enabled || questionType != "temporal-reasoning" {
		return contextBlocks
	}
	ledger := formatChronoLedger(atoms)
	if ledger == "" {
		return contextBlocks
	}
	combined := make([]string, 0, len(contextBlocks)+1)
	combined = append(combined, ledger)
	return append(combined, contextBlocks...)
}

// formatChronoLedger renders event atoms by date, then statement and
// supersession link for deterministic same-date ordering. Atoms without a
// valid_from cannot satisfy the explicitly-timestamped ledger contract and are
// omitted.
func formatChronoLedger(atoms []atom.Atom) string {
	dated := make([]atom.Atom, 0, len(atoms))
	for _, event := range atoms {
		if event.ValidFrom != nil {
			dated = append(dated, event)
		}
	}
	if len(dated) == 0 {
		return ""
	}

	sort.SliceStable(dated, func(i, j int) bool {
		leftDate := dated[i].ValidFrom.Format("2006-01-02")
		rightDate := dated[j].ValidFrom.Format("2006-01-02")
		if leftDate != rightDate {
			return leftDate < rightDate
		}
		if dated[i].Statement != dated[j].Statement {
			return dated[i].Statement < dated[j].Statement
		}
		return dated[i].Supersedes < dated[j].Supersedes
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
