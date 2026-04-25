package runner_test

import (
	"testing"

	"github.com/petersimmons1972/engram/internal/runner"
)

func TestDetectThinkingLeak_Clean(t *testing.T) {
	if runner.DetectThinkingLeak(`{"patterns":[]}`) {
		t.Error("clean JSON should not be flagged as thinking leak")
	}
}

func TestDetectThinkingLeak_ThinkTag(t *testing.T) {
	if !runner.DetectThinkingLeak(`<think>Let me analyze...</think>{"patterns":[]}`) {
		t.Error("content with <think> tag should be flagged")
	}
}

func TestDetectThinkingLeak_ThinkingTag(t *testing.T) {
	if !runner.DetectThinkingLeak(`<thinking>reasoning here</thinking>{"patterns":[]}`) {
		t.Error("content with <thinking> tag should be flagged")
	}
}

func TestDetectThinkingLeak_Thought(t *testing.T) {
	if !runner.DetectThinkingLeak(` Thought: I should analyze the patterns...`) {
		t.Error("content with ' Thought:' should be flagged")
	}
}

func TestDetectThinkingLeak_ClosingThinkTag(t *testing.T) {
	if !runner.DetectThinkingLeak(`</think>{"patterns":[]}`) {
		t.Error("closing </think> tag alone should be flagged")
	}
}

func TestDetectThinkingLeak_ClosingThinkingTag(t *testing.T) {
	if !runner.DetectThinkingLeak(`</thinking>{"patterns":[]}`) {
		t.Error("closing </thinking> tag alone should be flagged")
	}
}

func TestDetectThinkingLeak_HarmonyFormat(t *testing.T) {
	if !runner.DetectThinkingLeak(`<|channel|>analysis<|message|>{"patterns":[]}`) {
		t.Error("GPT-OSS Harmony format should be flagged")
	}
}
