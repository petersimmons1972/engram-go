package runner

// Sequential by design — do not parallelise. Models share GPU.

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/petersimmons1972/engram/internal/manifest"
	"github.com/petersimmons1972/engram/internal/ollama"
	"github.com/petersimmons1972/engram/internal/types"
)

const SystemPrompt = `You are a pattern detection system analyzing Claude Code tool call sequences.

Analyze the tool call events and identify recurring patterns of these types:

1. CORRECTION: Evidence the user corrected the AI — re-do after rollback, same action reversed within 3 steps.
2. ERROR_RESOLUTION: The same error (exit_status=1 + similar output) followed by the same fix, 2+ times.
3. WORKFLOW: A sequence of 3+ tool calls that recurs within or across sessions.

Return a JSON object with a single key "patterns" containing an array. Each pattern must have:
- "type": "correction" | "error_resolution" | "workflow"
- "description": one sentence, present tense
- "domain": one word (testing|git|editing|bash|agent|memory|general)
- "evidence": brief explanation, max 100 chars
- "tag_signature": lowercase slug e.g. "sig-edit-bash-fail"
- "confidence": 0.0 to 1.0

If no patterns found, return {"patterns":[]}.
Return ONLY valid JSON — no prose, no markdown fences.`

func loadEvents(fixturePath string) (string, error) {
	f, err := os.Open(fixturePath)
	if err != nil {
		return "", fmt.Errorf("opening fixture %s: %w", fixturePath, err)
	}
	defer func() { _ = f.Close() }()

	var lines []string
	skipped := 0
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024) // 1MB max line
	for sc.Scan() {
		var event map[string]any
		if err := json.Unmarshal(sc.Bytes(), &event); err != nil {
			skipped++
			continue
		}
		toolName, _ := event["tool_name"].(string)
		summary, _ := event["tool_output_summary"].(string)
		ts, _ := event["timestamp"].(string)
		exitStatus, _ := event["exit_status"].(float64)
		lines = append(lines, fmt.Sprintf("[%s] %s | exit=%.0f | %s", ts, toolName, exitStatus, truncate(summary, 80)))
	}
	if err := sc.Err(); err != nil {
		return "", fmt.Errorf("reading fixture: %w", err)
	}
	if len(lines) == 0 {
		return "", fmt.Errorf("fixture %s: no valid events (skipped %d malformed lines)", fixturePath, skipped)
	}
	header := fmt.Sprintf("Tool call events (%d total, %d skipped):\n", len(lines), skipped)
	return header + strings.Join(lines, "\n") + "\n", nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

// Run executes numRuns inference calls against model and returns raw results.
func Run(ctx context.Context, client *ollama.Client, model manifest.Model, fixturePath string, numRuns int) (types.RunResult, error) {
	result := types.RunResult{Model: model.Name}

	available, digest, err := client.IsAvailable(ctx, model.Name)
	if err != nil {
		return result, fmt.Errorf("checking availability: %w", err)
	}
	if !available {
		pullStart := time.Now()
		d, err := client.Pull(ctx, model.Name, os.Stdout)
		result.PullDuration = types.Duration(time.Since(pullStart))
		if err != nil {
			result.Skipped = true
			result.SkipReason = fmt.Sprintf("pull failed: %v", err)
			return result, nil
		}
		digest = d
	}
	result.ModelDigest = digest

	userMsg, err := loadEvents(fixturePath)
	if err != nil {
		return result, err
	}

	formatJSON := json.RawMessage(`"json"`)

	for i := 0; i < numRuns; i++ {
		runCtx, cancel := context.WithTimeout(ctx, 300*time.Second)
		attempt := types.RunAttempt{}
		start := time.Now()

		resp, err := client.Chat(runCtx, ollama.ChatRequest{
			Model: model.Name,
			Messages: []ollama.Message{
				{Role: "system", Content: SystemPrompt},
				{Role: "user", Content: userMsg},
			},
			Format:  formatJSON,
			Options: map[string]any{"temperature": 0.1, "num_predict": 1024},
		})
		attempt.Duration = types.Duration(time.Since(start))
		// Capture context state before cancel clears it.
		timedOut := runCtx.Err() != nil
		cancel()

		if err != nil {
			if timedOut {
				attempt.TimedOut = true
			} else {
				attempt.Error = err.Error()
			}
		} else {
			attempt.RawContent = resp.Message.Content
			attempt.ThinkingText = resp.Message.Thinking
		}
		result.Runs = append(result.Runs, attempt)
	}

	evictCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	if err := client.Evict(evictCtx, model.Name); err != nil {
		fmt.Fprintf(os.Stderr, "  evict %s: %v\n", model.Name, err)
	}
	cancel()

	return result, nil
}
