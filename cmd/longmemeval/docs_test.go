package main

import (
	"os"
	"strings"
	"testing"
)

func TestLMEBenchmarkLearningsScratchTTLIngestExampleUsesCurrentFlags(t *testing.T) {
	data, err := os.ReadFile("../../docs/lme-benchmark-learnings.md")
	if err != nil {
		t.Fatalf("read docs/lme-benchmark-learnings.md: %v", err)
	}
	doc := string(data)
	section := between(t, doc, "### Stamping TTL at ingest time", "### Running the prune sweep")

	for _, stale := range []string{"--data-file", "--out-dir", "--database-url"} {
		if strings.Contains(section, stale) {
			t.Fatalf("scratch TTL ingest example contains stale flag %q:\n%s", stale, section)
		}
	}
	for _, current := range []string{"--data questions.json", "--out /tmp/lme-run-001", "--scratch-ttl 168h"} {
		if !strings.Contains(section, current) {
			t.Fatalf("scratch TTL ingest example missing current flag text %q:\n%s", current, section)
		}
	}
}

func TestLMEBenchmarkLearningsPruneExampleUsesSafeExecuteFlags(t *testing.T) {
	data, err := os.ReadFile("../../docs/lme-benchmark-learnings.md")
	if err != nil {
		t.Fatalf("read docs/lme-benchmark-learnings.md: %v", err)
	}
	doc := string(data)
	section := between(t, doc, "### Running the prune sweep", "### Backfilling existing runs")

	for _, current := range []string{"--execute", "--confirm-prefix lme-", "--limit 50"} {
		if !strings.Contains(section, current) {
			t.Fatalf("prune example missing safe execute flag text %q:\n%s", current, section)
		}
	}
}

func TestLMEPruneCronJobUsesSafeExecuteFlags(t *testing.T) {
	data, err := os.ReadFile("../../deploy/lme-prune-cronjob.yaml")
	if err != nil {
		t.Fatalf("read deploy/lme-prune-cronjob.yaml: %v", err)
	}
	manifest := string(data)
	for _, current := range []string{"/engram", "--execute", "--confirm-prefix=lme-", "--limit=200", "--use-default-token"} {
		if !strings.Contains(manifest, current) {
			t.Fatalf("prune CronJob missing safe execute flag text %q:\n%s", current, manifest)
		}
	}
}

func TestLMEPruneCronJobPinsReviewedImage(t *testing.T) {
	data, err := os.ReadFile("../../deploy/lme-prune-cronjob.yaml")
	if err != nil {
		t.Fatalf("read deploy/lme-prune-cronjob.yaml: %v", err)
	}
	manifest := string(data)
	const reviewedImage = "ghcr.io/petersimmons1972/engram-go/longmemeval@sha256:c51f11f15003768b965774669b753c885c40cfdf13e2bb8b7a42f652143161f3"

	if strings.Contains(manifest, "ghcr.io/petersimmons1972/engram-go/longmemeval:latest") || strings.Contains(manifest, "ghcr.io/petersimmons1972/engram-go/longmemeval:v3.2.0") {
		t.Fatalf("prune CronJob must not use mutable :latest image:\n%s", manifest)
	}
	for _, current := range []string{
		"#   - Image: " + reviewedImage,
		"image: " + reviewedImage,
		"Never use :latest for this CronJob.",
		"update both this comment and the image reference in the same reviewed change",
		"imagePullPolicy: Always",
	} {
		if !strings.Contains(manifest, current) {
			t.Fatalf("prune CronJob missing reviewed image pin guidance %q:\n%s", current, manifest)
		}
	}
	if !strings.Contains(manifest, "@sha256:") {
		t.Fatalf("prune CronJob image must be immutable by digest:\n%s", manifest)
	}
}

func TestLMEBenchmarkLearningsDocumentsPruneImageUpdateProcedure(t *testing.T) {
	data, err := os.ReadFile("../../docs/lme-benchmark-learnings.md")
	if err != nil {
		t.Fatalf("read docs/lme-benchmark-learnings.md: %v", err)
	}
	doc := string(data)
	section := between(t, doc, "### Updating the prune image", "### Backfilling existing runs")

	for _, current := range []string{
		"@sha256:c51f11f15003768b965774669b753c885c40cfdf13e2bb8b7a42f652143161f3",
		"kubectl patch cronjob lme-prune -n engram -p '{\"spec\":{\"suspend\":true}}'",
		"jsonpath='{.spec.suspend",
		"command: [\"/engram\"]",
		"envFrom:",
		"name: engram-lme",
		"replace it with the reviewed release tag or immutable digest",
		"Update `deploy/lme-prune-cronjob.yaml` in the same reviewed change",
		"kubectl apply -f deploy/lme-prune-cronjob.yaml",
		"#### Safe canary and rollout evidence",
		"IMAGE=$(kubectl -n engram get cronjob lme-prune -o jsonpath",
		"CANARY_JOB=lme-prune-canary-$(date +%s)",
		"CANARY_EXIT_CODE",
		"CANARY imageID",
		"kubectl apply -f <<EOF",
		"prune: DRY RUN — would delete",
		"imageID",
		"--timestamps",
		"summary status",
		"non-zero execute exit code",
		"If the canary is unexpected",
		"lme-prune-verify-$(date +%s)",
		"--dry-run",
		"--execute",
		"--limit=50",
		"--use-default-token",
		"kubectl patch cronjob lme-prune -n engram -p '{\"spec\":{\"suspend\":false}}'",
	} {
		if !strings.Contains(section, current) {
			t.Fatalf("prune image update procedure missing %q:\n%s", current, section)
		}
	}
}

func TestCommandCatalogDocumentsLongMemEval(t *testing.T) {
	data, err := os.ReadFile("../../cmd/README.md")
	if err != nil {
		t.Fatalf("read cmd/README.md: %v", err)
	}
	doc := string(data)
	for _, current := range []string{"## longmemeval", "score-efficient", "route-discover", "prune", "`longmemeval`"} {
		if !strings.Contains(doc, current) {
			t.Fatalf("command catalog missing %q", current)
		}
	}
	if strings.Contains(doc, "Engram ships six binaries") {
		t.Fatalf("command catalog still says Engram ships six binaries")
	}
}

func TestLMEBenchmarkLearningsQuickStartUsesCurrentFlags(t *testing.T) {
	data, err := os.ReadFile("../../docs/lme-benchmark-learnings.md")
	if err != nil {
		t.Fatalf("read docs/lme-benchmark-learnings.md: %v", err)
	}
	doc := string(data)
	section := between(t, doc, "## Quick Start for Future Benchmark Runs", "### Docker Compose Configuration")

	for _, stale := range []string{"--no-cleanup", "compile-time constants"} {
		if strings.Contains(section, stale) {
			t.Fatalf("quick start contains stale text %q:\n%s", stale, section)
		}
	}
	for _, current := range []string{"--cleanup-policy=never", "--recall-topk", "--context-topk", "score-efficient", "route-discover"} {
		if !strings.Contains(section, current) {
			t.Fatalf("quick start missing current text %q:\n%s", current, section)
		}
	}
}

func TestLMEBenchmarkLearningsSummaryUsesCurrentQuestionTypesAndPortableReference(t *testing.T) {
	data, err := os.ReadFile("../../docs/lme-benchmark-learnings.md")
	if err != nil {
		t.Fatalf("read docs/lme-benchmark-learnings.md: %v", err)
	}
	doc := string(data)

	for _, current := range []string{
		"LongMemEval-M (500 questions, 6 types)",
		"500 questions across six types: knowledge-update, multi-session, single-session-assistant, single-session-user, single-session-preference, and temporal-reasoning.",
		"`docs/architecture.md`",
	} {
		if !strings.Contains(doc, current) {
			t.Fatalf("benchmark learnings missing current summary/reference text %q", current)
		}
	}
	if strings.Contains(doc, "/home/psimmons/projects/engram-go/docs/architecture.md") {
		t.Fatalf("benchmark learnings still contains checkout-specific architecture path")
	}
}

func TestSSPrefModelRecommendationMemoTrackedAndComplete(t *testing.T) {
	readmeData, err := os.ReadFile("../../results/README.md")
	if err != nil {
		t.Fatalf("read results/README.md: %v", err)
	}
	readme := string(readmeData)
	const trackedFile = "wisdom/ss-pref-model-recommendations-2026-06-27.md"
	if !strings.Contains(readme, trackedFile) {
		t.Fatalf("results/README.md missing tracked wisdom artifact %q", trackedFile)
	}

	docData, err := os.ReadFile("../../results/" + trackedFile)
	if err != nil {
		t.Fatalf("read results/%s: %v", trackedFile, err)
	}
	doc := string(docData)
	for _, current := range []string{
		"## Ranked Candidates",
		"## Quantization Guidance",
		"## Config Levers to Prioritize",
		"Qwen3-32B BF16",
		"Llama 3.3 70B NVFP4",
		"Qwen2.5-72B NVFP4",
		"Gemma 3 27B BF16",
		"Qwen3-32B NVFP4",
		"dual-preference-recall",
		"topic-anchor-boost",
		"query-paraphrase-passes=3",
		"PreferenceMMR",
	} {
		if !strings.Contains(doc, current) {
			t.Fatalf("ss-pref model recommendation memo missing %q", current)
		}
	}
}

func TestRunbookW6800CanaryHasConcreteOllaChecks(t *testing.T) {
	data, err := os.ReadFile("../../docs/runbook.md")
	if err != nil {
		t.Fatalf("read docs/runbook.md: %v", err)
	}
	doc := string(data)
	section := between(t, doc, "## W6800 Canary", "## Common Issues")

	for _, current := range []string{"/v1/models", "/v1/chat/completions", "llama3.1:8b", "BAAI/bge-m3"} {
		if !strings.Contains(section, current) {
			t.Fatalf("W6800 canary section missing concrete check text %q:\n%s", current, section)
		}
	}
}

func TestDeploymentNotesW6800CanaryHasConcreteOllaChecks(t *testing.T) {
	data, err := os.ReadFile("../../docs/deployment-notes.md")
	if err != nil {
		t.Fatalf("read docs/deployment-notes.md: %v", err)
	}
	doc := string(data)
	section := between(t, doc, "## W6800 Canary Rollout", "Current verified split:")

	for _, current := range []string{"/v1/models", "/v1/chat/completions", "route-discover", "llama3.1:8b", "BAAI/bge-m3"} {
		if !strings.Contains(section, current) {
			t.Fatalf("deployment W6800 section missing concrete check text %q:\n%s", current, section)
		}
	}
}

func between(t *testing.T, s, start, end string) string {
	t.Helper()
	startIdx := strings.Index(s, start)
	if startIdx < 0 {
		t.Fatalf("missing start marker %q", start)
	}
	rest := s[startIdx:]
	endIdx := strings.Index(rest, end)
	if endIdx < 0 {
		t.Fatalf("missing end marker %q after %q", end, start)
	}
	return rest[:endIdx]
}
