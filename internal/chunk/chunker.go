// Package chunk provides text chunking, deduplication, and hashing utilities.
// All algorithms are ported from Python engram/src/engram/chunker.py and must
// produce bit-identical output for ChunkHash (SHA-256 compatibility).
package chunk

import (
	"crypto/sha256"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
)

// LazyChunkThreshold: content shorter than this is returned as a single chunk.
// Mirrors Python LAZY_CHUNK_THRESHOLD = 8000.
const LazyChunkThreshold = 8000

// DefaultTargetChunkChars is the default chunk size used by ChunkDocument
// when the caller passes targetChunkChars <= 0.
const DefaultTargetChunkChars = 2000

// headingRE matches level-1 and level-2 Markdown headings at the start of a line.
var headingRE = regexp.MustCompile(`(?m)^#{1,2}\s+(.+)$`)

// whitespaceRE collapses runs of whitespace for normalization.
var whitespaceRE = regexp.MustCompile(`\s+`)

// sentenceSplitRE splits on sentence-ending punctuation followed by whitespace.
var sentenceSplitRE = regexp.MustCompile(`(?:[.!?])\s+`)

// paragraphSplitRE splits on two or more consecutive newlines.
var paragraphSplitRE = regexp.MustCompile(`\n{2,}`)

// ChunkCandidate is a candidate chunk produced by ChunkDocument.
type ChunkCandidate struct {
	// Text is the chunk content, possibly including overlap from the previous chunk.
	Text string
	// SectionHeading is the nearest level-1/2 Markdown heading ancestor, or empty
	// when the document has no headings.
	SectionHeading string
	// HasHeading is true when a SectionHeading was found (distinguishes empty heading
	// from no heading).
	HasHeading bool
	// ChunkType is one of "section", "paragraph", or "sentence_window".
	ChunkType string
}

// ChunkHash returns a 32-character hex string that is SHA-256 of the whitespace-
// normalized, lowercased text. Must be bit-identical to Python:
//
//	hashlib.sha256(re.sub(r"\s+", " ", text.strip().lower()).encode()).hexdigest()[:32]
func ChunkHash(text string) string {
	normalized := whitespaceRE.ReplaceAllString(strings.TrimSpace(strings.ToLower(text)), " ")
	sum := sha256.Sum256([]byte(normalized))
	return fmt.Sprintf("%x", sum)[:32]
}

// JaccardSimilarity computes word-level Jaccard similarity between two strings.
// Mirrors Python jaccard_similarity().
func JaccardSimilarity(a, b string) float64 {
	wordsA := wordSet(a)
	wordsB := wordSet(b)
	if len(wordsA) == 0 || len(wordsB) == 0 {
		return 0.0
	}
	intersection := 0
	for w := range wordsA {
		if wordsB[w] {
			intersection++
		}
	}
	union := len(wordsA) + len(wordsB) - intersection
	return float64(intersection) / float64(union)
}

// IsDuplicate returns true when newText is too similar to any of existingTexts.
// Threshold is read from ENGRAM_DEDUP_THRESHOLD env var, defaulting to 0.75.
// An explicit threshold argument (-1 signals "use env/default").
// Mirrors Python is_duplicate().
func IsDuplicate(newText string, existingTexts []string, threshold float64) bool {
	if threshold < 0 {
		threshold = defaultDedupThreshold()
	}
	for _, existing := range existingTexts {
		if JaccardSimilarity(newText, existing) >= threshold {
			return true
		}
	}
	return false
}

// ChunkText splits text into overlapping sentence-window chunks.
// Content shorter than LazyChunkThreshold is returned as a single chunk.
// Uses ~4 chars/token approximation to avoid a tokenizer dependency.
// Mirrors Python chunk_text().
func ChunkText(text string, maxTokens, overlapTokens int) []string {
	if strings.TrimSpace(text) == "" {
		return nil
	}
	if len(text) <= LazyChunkThreshold {
		return []string{text}
	}

	const charsPerToken = 4
	maxChars := maxTokens * charsPerToken
	overlapChars := overlapTokens * charsPerToken

	sentences := splitSentences(text)
	if len(sentences) == 0 {
		return []string{text}
	}

	var chunks []string
	var current []string
	currentLen := 0

	for _, sentence := range sentences {
		slen := len(sentence)
		sep := 0
		if len(current) > 0 {
			sep = 1
		}
		addedLen := slen + sep

		if currentLen+addedLen > maxChars && len(current) > 0 {
			chunks = append(chunks, strings.Join(current, " "))

			// Build overlap from the tail of the current chunk
			var overlap []string
			overlapLen := 0
			for i := len(current) - 1; i >= 0; i-- {
				s := current[i]
				sepO := 0
				if len(overlap) > 0 {
					sepO = 1
				}
				if overlapLen+len(s)+sepO > overlapChars {
					break
				}
				overlap = append([]string{s}, overlap...)
				overlapLen += len(s) + sepO
			}
			current = overlap
			currentLen = overlapLen
			sep = 0
			if len(current) > 0 {
				sep = 1
			}
			addedLen = slen + sep
		}

		current = append(current, sentence)
		currentLen += addedLen
	}

	if len(current) > 0 {
		chunks = append(chunks, strings.Join(current, " "))
	}

	if len(chunks) == 0 {
		return []string{text}
	}
	return chunks
}

// ChunkDocument performs semantic chunking: headings → paragraphs → sentence-window fallback.
// Mirrors Python chunk_document().
func ChunkDocument(text string, targetChunkChars int) []ChunkCandidate {
	if strings.TrimSpace(text) == "" {
		return nil
	}
	if targetChunkChars <= 0 {
		targetChunkChars = DefaultTargetChunkChars
	}

	type headingPos struct {
		pos     int
		heading string
	}

	matches := headingRE.FindAllStringSubmatchIndex(text, -1)
	var headings []headingPos
	for _, m := range matches {
		headings = append(headings, headingPos{
			pos:     m[0],
			heading: strings.TrimSpace(text[m[2]:m[3]]),
		})
	}

	if len(headings) == 0 {
		return chunkSection(strings.TrimSpace(text), "", false, targetChunkChars)
	}

	// Build (heading, body) pairs
	type section struct {
		heading string
		body    string
	}
	var sections []section
	for i, h := range headings {
		// Find end of heading line
		lineEnd := strings.Index(text[h.pos:], "\n")
		var bodyStart int
		if lineEnd < 0 {
			bodyStart = len(text)
		} else {
			bodyStart = h.pos + lineEnd + 1
		}

		var bodyEnd int
		if i+1 < len(headings) {
			bodyEnd = headings[i+1].pos
		} else {
			bodyEnd = len(text)
		}

		body := ""
		if bodyStart < bodyEnd {
			body = strings.TrimSpace(text[bodyStart:bodyEnd])
		}
		sections = append(sections, section{heading: h.heading, body: body})
	}

	var results []ChunkCandidate
	var prevLastSentence string

	for _, sec := range sections {
		sectionChunks := chunkSection(sec.body, sec.heading, true, targetChunkChars)

		if prevLastSentence != "" && len(sectionChunks) > 0 {
			first := sectionChunks[0]
			if !strings.Contains(first.Text, prevLastSentence) {
				sectionChunks[0] = ChunkCandidate{
					Text:           prevLastSentence + " " + first.Text,
					SectionHeading: first.SectionHeading,
					HasHeading:     first.HasHeading,
					ChunkType:      first.ChunkType,
				}
			}
		}

		results = append(results, sectionChunks...)

		if len(results) > 0 {
			prevLastSentence = lastSentence(results[len(results)-1].Text)
		}
	}

	return results
}

// ── Internal helpers ─────────────────────────────────────────────────────────

func chunkSection(text, heading string, hasHeading bool, target int) []ChunkCandidate {
	if strings.TrimSpace(text) == "" {
		return nil
	}

	rawParas := paragraphSplitRE.Split(strings.TrimSpace(text), -1)
	var paras []string
	for _, p := range rawParas {
		if p = strings.TrimSpace(p); p != "" {
			paras = append(paras, p)
		}
	}
	if len(paras) == 0 {
		return nil
	}

	var chunks []ChunkCandidate
	var currentTexts []string
	currentLen := 0
	var prevLast string

	flush := func(ctype string) {
		if len(currentTexts) == 0 {
			return
		}
		merged := strings.Join(currentTexts, "\n\n")
		if prevLast != "" && !strings.Contains(merged, prevLast) {
			merged = prevLast + " " + merged
		}
		chunks = append(chunks, ChunkCandidate{
			Text:           merged,
			SectionHeading: heading,
			HasHeading:     hasHeading,
			ChunkType:      ctype,
		})
		currentTexts = nil
		currentLen = 0
	}

	for _, para := range paras {
		if len(para) > target {
			flush("paragraph")
			swChunks := sentenceWindow(para, target)
			for i, swText := range swChunks {
				if i == 0 && prevLast != "" && !strings.Contains(swText, prevLast) {
					swText = prevLast + " " + swText
				}
				chunks = append(chunks, ChunkCandidate{
					Text:           swText,
					SectionHeading: heading,
					HasHeading:     hasHeading,
					ChunkType:      "sentence_window",
				})
			}
			if len(chunks) > 0 {
				prevLast = lastSentence(chunks[len(chunks)-1].Text)
			}
			continue
		}

		sep := 2
		if len(currentTexts) == 0 {
			sep = 0
		}
		if currentLen+len(para)+sep > target && len(currentTexts) > 0 {
			flush("paragraph")
			if len(chunks) > 0 {
				prevLast = lastSentence(chunks[len(chunks)-1].Text)
			}
		}

		currentTexts = append(currentTexts, para)
		currentLen += len(para)
		if len(currentTexts) > 1 {
			currentLen += 2
		}
	}

	flush("paragraph")
	return chunks
}

func sentenceWindow(text string, target int) []string {
	sentences := splitSentences(text)
	if len(sentences) == 0 {
		return []string{text}
	}

	var chunks []string
	var current []string
	currentLen := 0

	for _, sentence := range sentences {
		slen := len(sentence)
		sep := 0
		if len(current) > 0 {
			sep = 1
		}
		addedLen := slen + sep

		if currentLen+addedLen > target && len(current) > 0 {
			chunks = append(chunks, strings.Join(current, " "))
			// Overlap: carry last sentence
			var overlap []string
			if len(current) > 0 {
				overlap = []string{current[len(current)-1]}
			}
			overlapLen := 0
			if len(overlap) > 0 {
				overlapLen = len(overlap[0])
			}
			current = overlap
			currentLen = overlapLen
			sep = 0
			if len(current) > 0 {
				sep = 1
			}
			addedLen = slen + sep
		}

		current = append(current, sentence)
		currentLen += addedLen
	}

	if len(current) > 0 {
		chunks = append(chunks, strings.Join(current, " "))
	}

	if len(chunks) == 0 {
		return []string{text}
	}
	return chunks
}

func lastSentence(text string) string {
	sentences := splitSentences(text)
	if len(sentences) == 0 {
		return ""
	}
	return sentences[len(sentences)-1]
}

// splitSentences splits on sentence-ending punctuation followed by whitespace,
// keeping the delimiter attached to the preceding sentence.
// Mirrors Python _split_sentences().
func splitSentences(text string) []string {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}

	// Find all split positions
	locs := sentenceSplitRE.FindAllStringIndex(text, -1)
	if len(locs) == 0 {
		return []string{text}
	}

	var parts []string
	prev := 0
	for _, loc := range locs {
		// The match includes the punctuation char and the trailing whitespace.
		// We want to keep the punctuation with the preceding sentence.
		// loc[0] is the start of the punctuation char (which is part of the sentence),
		// but our regex starts AFTER the punctuation (lookbehind equivalent: match the
		// whitespace after [.!?]). So the split point is loc[0] for the sentence end.
		// Actually sentenceSplitRE matches the whitespace AFTER [.!?], so loc[0] is
		// the space. The punctuation at loc[0]-1 should stay with the sentence.
		part := strings.TrimSpace(text[prev:loc[0]])
		if part != "" {
			parts = append(parts, part)
		}
		prev = loc[1]
	}
	if tail := strings.TrimSpace(text[prev:]); tail != "" {
		parts = append(parts, tail)
	}
	return parts
}

func wordSet(text string) map[string]bool {
	normalized := whitespaceRE.ReplaceAllString(strings.TrimSpace(strings.ToLower(text)), " ")
	words := strings.Split(normalized, " ")
	set := make(map[string]bool, len(words))
	for _, w := range words {
		if w != "" {
			set[w] = true
		}
	}
	return set
}

func defaultDedupThreshold() float64 {
	if v := os.Getenv("ENGRAM_DEDUP_THRESHOLD"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
	}
	return 0.75
}
