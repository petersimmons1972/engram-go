package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/petersimmons1972/engram/internal/llmclient"
)

// auditPrompt is the production-tuned prompt for pattern quality evaluation.
// Ported verbatim from instinct-python/cmd/audit/main.go:30-52.
// Do not rewrite without benchmarking — this prompt is tuned for KEEP/TUNE/REJECT verdicts.
const auditPrompt = `You are auditing an automatically-detected behavioural pattern for quality.

Pattern:
  type:        %s
  description: %s
  domain:      %s
  tag:         %s

Answer each question with one word:
IS_VALID: yes or no — is this a real, observable behaviour pattern?
IS_ACTIONABLE: yes or no — could an AI assistant use this to improve its behaviour?
IS_SPECIFIC: yes or no — is the description specific enough to act on?
FALSE_POSITIVE: yes or no — is this likely noise rather than a real pattern?
VERDICT: KEEP, TUNE (needs rewording), or REJECT
REASON: one sentence

Respond in exactly this format, nothing else:
IS_VALID: <yes/no>
IS_ACTIONABLE: <yes/no>
IS_SPECIFIC: <yes/no>
FALSE_POSITIVE: <yes/no>
VERDICT: <KEEP/TUNE/REJECT>
REASON: <one sentence>`

// Judge evaluates a single Engram memory record using the LLM client and
// returns an auditResult with the parsed KEEP/TUNE/REJECT verdict.
//
// The function mirrors the pattern established in consolidator/detect.go:
// the LLM client knows nothing about audit verdicts — all domain logic
// (prompt construction, line-by-line parsing) lives here.
//
// Ported from instinct-python/cmd/audit/main.go:163-219 with these changes:
//   - client is llmclient.LLMClient (generic) instead of *ollama.Client (Ollama-specific)
//   - model parameter is dropped; model selection is handled by the LLM factory
//   - timeout is applied via context.WithTimeout wrapping the Complete call
func Judge(ctx context.Context, client llmclient.LLMClient, timeout time.Duration, m engramMemory) auditResult {
	ptype, domain, sig := extractTags(m.Tags)
	content := m.Content
	if content == "" {
		content = m.Summary
	}

	userPrompt := fmt.Sprintf(auditPrompt, ptype, content, domain, sig)

	callCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	res := auditResult{
		ID:         m.ID,
		Content:    truncate(content, 150),
		Tags:       m.Tags,
		Confidence: m.Importance,
	}

	raw, err := client.Complete(callCtx, "", userPrompt)
	if err != nil {
		res.Verdict = "ERROR"
		res.Reason = err.Error()
		return res
	}

	raw = strings.TrimSpace(raw)
	res.Raw = raw

	// Parse the line-by-line verdict response.
	// Ported verbatim from instinct-python/cmd/audit/main.go:197-218.
	for _, line := range strings.Split(raw, "\n") {
		k, v, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		k = strings.TrimSpace(strings.ToLower(k))
		v = strings.TrimSpace(v)
		switch k {
		case "is_valid":
			res.IsValid = v
		case "is_actionable":
			res.IsActionable = v
		case "is_specific":
			res.IsSpecific = v
		case "false_positive":
			res.FalsePositive = v
		case "verdict":
			res.Verdict = strings.ToUpper(v)
		case "reason":
			res.Reason = v
		}
	}
	return res
}
