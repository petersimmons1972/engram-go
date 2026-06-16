package longmemeval_test

import (
	"strings"
	"testing"

	"github.com/petersimmons1972/engram/internal/longmemeval"
)

func TestIsAggregationQuestion_DoesNotMatchTemporalQuantity(t *testing.T) {
	question := "How many weeks ago did I attend the baking class?"
	if longmemeval.IsAggregationQuestion(question) {
		t.Fatalf("IsAggregationQuestion(%q) = true, want false", question)
	}
}

func TestGenerationPromptForTypeEnumerate_PrependsInstructionToBaseline(t *testing.T) {
	question := "How many times did I call my sister?"
	questionType := "multi-session"
	questionDate := "2024-06-01"
	contextBlocks := []string{"Session date: 2024-05-10\nCalled my sister."}

	base := longmemeval.GenerationPromptForType(question, questionType, questionDate, contextBlocks)
	got := longmemeval.GenerationPromptForTypeEnumerate(question, questionType, questionDate, contextBlocks, true)

	if got == base {
		t.Fatal("enumerate-first prompt should differ from baseline for aggregation questions")
	}
	if !strings.HasSuffix(got, base) {
		t.Fatalf("enumerate-first prompt must preserve the baseline prompt as a suffix\nbase:\n%s\n\ngot:\n%s", base, got)
	}
	if !strings.Contains(got, "First,") {
		t.Fatalf("enumerate-first prompt missing prefixed instruction:\n%s", got)
	}
}
