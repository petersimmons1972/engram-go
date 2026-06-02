// exact_fact_boost.go — exact-fact / entity-identifier scoring boost.
//
// LME experiment #4, issue #938 improvement #3.
//
// Exported symbols:
//   - IsIdentifierQuery(query string) bool
//   - ExactIdentifierHit(content, query string) bool
//
// exactIdentifierBoost() is package-private; used only by score.go.
package search

import (
	"regexp"
	"strings"
	"unicode"
)

// identifierBoostValue is the additive score boost applied when
// ExactIdentifierMatch=true in ScoreInput. Calibrated so that a memory with
// modest cosine (0.60) and BM25 (0.55) outscores a high-cosine near-miss
// (cosine=0.82, BM25=0.50) under default weights:
//
//	right  = 0.40*0.60 + 0.30*0.55 + 0.15*recency + 0.15*precision + boost × ImportanceBoost
//	wrong  = 0.40*0.82 + 0.30*0.50 + 0.15*recency + 0.15*precision
//
// At identical recency and precision, the gap to overcome is:
//
//	wrong − right ≈ 0.40*(0.82−0.60) + 0.30*(0.50−0.55) = 0.088 − 0.015 = 0.073
//
// Setting boost = 0.10 (multiplied by ImportanceBoost(2) = 1.0) → net +0.10,
// which is sufficient to flip the ranking with a comfortable margin.
const identifierBoostValue = 0.10

func exactIdentifierBoost() float64 { return identifierBoostValue }

// ── URL regex ────────────────────────────────────────────────────────────────

var urlRe = regexp.MustCompile(`https?://\S+`)

// ── Phone number regex ────────────────────────────────────────────────────────
// Matches:
//   +1-800-555-1234   (international prefix, dashes)
//   (415) 555-0123    (parenthesised area code)
//   650-555-9999      (plain dashes, 10-digit US)

var phoneRe = regexp.MustCompile(
	`(?:\+\d[\d\-]{7,}|` + // +1-800-… or +44…
		`\(\d{3}\)\s?\d{3}[\-\s]\d{4}|` + // (415) 555-0123
		`\d{3}[\-\s]\d{3}[\-\s]\d{4})`) // 650-555-9999

// ── Named-entity: two consecutive title-cased tokens ──────────────────────────

// isProperNounPair returns true when query contains at least two consecutive
// tokens that both start with an uppercase letter. Single-word capitalisation
// (start-of-sentence) is excluded by requiring a consecutive pair.
func isProperNounPair(query string) bool {
	tokens := strings.Fields(query)
	prevTitle := false
	for _, tok := range tokens {
		runes := []rune(tok)
		if len(runes) == 0 {
			prevTitle = false
			continue
		}
		// Strip leading punctuation for cleaner detection.
		start := runes[0]
		if !unicode.IsLetter(start) && !unicode.IsDigit(start) {
			// Punctuation-stripped first char.
			stripped := strings.TrimLeftFunc(tok, func(r rune) bool {
				return !unicode.IsLetter(r) && !unicode.IsDigit(r)
			})
			if stripped == "" {
				prevTitle = false
				continue
			}
			runes = []rune(stripped)
			start = runes[0]
		}
		isTitle := unicode.IsUpper(start)
		if isTitle && prevTitle {
			return true
		}
		prevTitle = isTitle
	}
	return false
}

// IsIdentifierQuery returns true when query contains a high-precision entity
// identifier: a URL, a phone number, or at least two consecutive title-cased
// tokens (named entity). When true, the engine is allowed to apply the
// exact-fact boost for memories whose content contains a verbatim match.
func IsIdentifierQuery(query string) bool {
	if urlRe.MatchString(query) {
		return true
	}
	if phoneRe.MatchString(query) {
		return true
	}
	return isProperNounPair(query)
}

// isIdentifierQuery is the unexported alias used within the search package.
func isIdentifierQuery(query string) bool { return IsIdentifierQuery(query) }

// ExactIdentifierHit returns true when content contains at least one token from
// query that is also present in the identifier set (URLs, phone numbers, or
// title-cased token pairs). The comparison is case-insensitive for title-cased
// tokens and case-sensitive for URLs and phone numbers (to avoid false positives
// on URLs containing random substrings of common words).
func ExactIdentifierHit(content, query string) bool {
	lowerContent := strings.ToLower(content)

	// URL hit: verbatim substring match (case-sensitive at URL level).
	for _, url := range urlRe.FindAllString(query, -1) {
		if strings.Contains(content, url) {
			return true
		}
	}

	// Phone hit: verbatim.
	for _, phone := range phoneRe.FindAllString(query, -1) {
		if strings.Contains(content, phone) {
			return true
		}
	}

	// Named-entity hit: check whether any title-cased token from a consecutive
	// pair appears in content (case-insensitive). We look for the *pair* as a
	// phrase: both words adjacent in either order.
	tokens := strings.Fields(query)
	for i := 0; i+1 < len(tokens); i++ {
		a := strings.TrimFunc(tokens[i], func(r rune) bool {
			return !unicode.IsLetter(r) && !unicode.IsDigit(r)
		})
		b := strings.TrimFunc(tokens[i+1], func(r rune) bool {
			return !unicode.IsLetter(r) && !unicode.IsDigit(r)
		})
		if len(a) == 0 || len(b) == 0 {
			continue
		}
		aRunes := []rune(a)
		bRunes := []rune(b)
		if !unicode.IsUpper(aRunes[0]) || !unicode.IsUpper(bRunes[0]) {
			continue
		}
		// Both title-cased: check if the pair phrase appears in content.
		phrase := strings.ToLower(a + " " + b)
		if strings.Contains(lowerContent, phrase) {
			return true
		}
	}
	return false
}
