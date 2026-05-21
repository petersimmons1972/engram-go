> **SUPERSEDED** — Implemented in PR #755 (merged 2026-05-20). This document retained for design context only.

# Issue #750 — lme: GenerateForModel retries on invalid-model error instead of short-circuiting

**Severity:** nice-to-have
**Area:** lme-tooling
**Status:** Design only — not yet implemented

## Root cause

`internal/longmemeval/claude.go:66-86` — the `generate()` function runs a retry loop with backoffs of 30s, 60s, 120s. `runClaude()` at line 104-106 returns a static validation error `"claude: refusing to invoke with disallowed model %q"` for any model not in `validClaudeModels`. This error is permanent — no amount of retrying will resolve it — but `generate()` does not distinguish it from a transient API error and sleeps the full backoff before each retry.

The test `TestGenerateForModel_InvalidModel_NoRetry` in `internal/longmemeval/claude_additions_test.go` is currently `t.Skip`'d with the comment "subprocess retry blocks 60s timeout; functionality covered by sibling test." The sibling test only validates that the error is returned, not that retries are skipped.

## Repro

```bash
# With retries=2 and model="gpt-4o" (not in allowlist), the call blocks ~90s
# before returning: 0s first attempt + 30s sleep + 60s sleep = ~90s wasted
go test ./internal/longmemeval/ -run TestGenerateForModel_InvalidModel_NoRetry -v -timeout 120s
# (currently skipped; if unskipped, takes 90s+ before returning the error)
```

## Proposed patch

```diff
--- a/internal/longmemeval/claude.go
+++ b/internal/longmemeval/claude.go
@@ -1,4 +1,11 @@
 package longmemeval
 
+import "errors"
+
+// ErrDisallowedModel is a permanent error returned when the model name is not
+// in the allowlist. The retry loop must not sleep on this error.
+var ErrDisallowedModel = errors.New("disallowed model")
+
 // generateTimeout is the hard cap for one OAI generation call.
@@ -66,10 +73,15 @@ func generate(ctx context.Context, prompt, model string, retries int) (string, error) {
 	var lastErr error
 	backoffs := []time.Duration{30 * time.Second, 60 * time.Second, 120 * time.Second}
 	for attempt := 0; attempt <= retries; attempt++ {
 		out, err := runClaude(ctx, prompt, model)
 		if err == nil {
 			return out, nil
 		}
 		lastErr = err
+		// Permanent validation errors must not be retried.
+		if errors.Is(err, ErrDisallowedModel) {
+			break
+		}
 		if attempt >= retries {
 			break
 		}
@@ -104,7 +116,7 @@ func runClaude(ctx context.Context, prompt, model string) (string, error) {
 	if !isValidClaudeModel(model) {
-		return "", fmt.Errorf("claude: refusing to invoke with disallowed model %q (allowed: opus, sonnet, haiku) (#678)", model)
+		return "", fmt.Errorf("%w: %q (allowed: opus, sonnet, haiku) (#678)", ErrDisallowedModel, model)
 	}
```

Then un-skip the test:

```diff
--- a/internal/longmemeval/claude_additions_test.go
+++ b/internal/longmemeval/claude_additions_test.go
@@ -59,7 +59,6 @@ func TestGenerateForModel_InvalidModel(t *testing.T) {
 func TestGenerateForModel_InvalidModel_NoRetry(t *testing.T) {
-	t.Skip("subprocess retry blocks 60s timeout; functionality covered by sibling test")
 	// Even with retries > 0 the model-rejection error should be returned
 	// immediately (no point sleeping and retrying a static validation failure).
 	ctx := context.Background()
```

Note: the test should also add a timing assertion — the call must complete in <1s, not 60s+:

```diff
+	start := time.Now()
 	_, err := longmemeval.GenerateForModel(ctx, "prompt", "claude-3-opus-20240229", 2)
+	elapsed := time.Since(start)
+	if elapsed > 5*time.Second {
+		t.Errorf("GenerateForModel with invalid model took %v — retry not short-circuited", elapsed)
+	}
```

## TDD scenarios

1. **invalid_model_no_retry_immediate** — Given `retries=2` and model `"gpt-4o"` (not in allowlist), when `GenerateForModel` is called, then it returns within 1s with an error wrapping `ErrDisallowedModel`.
2. **invalid_model_error_message_preserved** — Given an invalid model, when the error is returned, then `errors.Is(err, ErrDisallowedModel)` is true and the error string still contains the model name for diagnostics.
3. **valid_model_retries_on_transient_error** — Given a valid model `"sonnet"` and a transient error from `runClaude` (simulated), when `generate` is called with `retries=1`, then the retry sleep fires and the second attempt is made (no short-circuit).
4. **regression: sibling test still passes** — `TestGenerateForModel_InvalidModel` (zero retries) continues to pass and returns `ErrDisallowedModel`-wrapping error.

## Risk notes

- `ErrDisallowedModel` is a new exported sentinel; callers using `errors.Is` will work correctly; callers matching the string may need updating (search for `"disallowed model"`).
- The OAI path (`GenerateOAI`) does not call `runClaude`; its model validation (if any) is separate. Not affected by this change.
- The `Generate` function used by `GenerateOpus` calls `generate()` with a valid model — no behavior change for production use.

## Rollout

Rebuild binary. No infra changes. Un-skip the test and add it to CI.

## Out of scope (followups)

- Apply the same permanent-error short-circuit pattern to `ScoreOAI` retries if the scorer URL is unreachable at connection time (connection refused vs. transient timeout).
