# Layer B Corruption Probe

Issue: `#1289`
Date: `2026-07-03`

This artifact is the additive-schema integrity probe for Layer B deterministic
aggregation. It is tied to concrete automated checks rather than an ad-hoc
manual replay.

## Probe Surface

1. Temporal inversion replay
   - Guard: `internal/layerb/layerb_test.go::TestBuildSummary_UsesValidFromForTemporalInversion`
   - Scenario: memory `created_at` is newer than `valid_from`; Layer B must use
     `valid_from` as the event timestamp so historical ordering survives late ingest.

2. Provenance-span roundtrip
   - Guard: `internal/db/postgres_layerb_test.go::TestLayerBEvents_RoundTripIdempotentAndCascade`
   - Scenario: exact `provenance_span` and verbatim `span_text` are written to
     the additive Layer B tables and read back without mutation.

3. No-orphan check
   - Guard: `internal/db/postgres_layerb_test.go::TestLayerBEvents_RoundTripIdempotentAndCascade`
   - Scenario: deleting the source memory removes dependent Layer B rows via
     `ON DELETE CASCADE`; post-delete lookup returns zero events.

4. Idempotent re-ingest
   - Guard: `internal/mcp/layerb_recall_test.go::TestHandleMemoryRecall_LayerBIndexingIsIdempotent`
   - Scenario: re-running the same aggregation-shaped recall against the same
     memory does not duplicate Layer B atoms or events.

## Verification Commands

```bash
PATH=/usr/local/go/bin:$PATH /usr/local/go/bin/go test ./internal/layerb ./internal/mcp -run 'Test(BuildSummary_|HandleMemoryRecall_)' -count=1
PATH=/usr/local/go/bin:$PATH /usr/local/go/bin/go test ./internal/db -run 'TestLayerBEvents_RoundTripIdempotentAndCascade$' -count=1
PATH=/usr/local/go/bin:$PATH /usr/local/go/bin/go test ./... -count=1 -race
PATH=/usr/local/go/bin:$PATH /usr/local/go/bin/go vet ./...
```

## Notes

- Layer B is lazy and additive: indexing happens only in the post-retrieval
  pass for single-project full recall responses.
- Existing recall ranking, existing memory/chunk schema, and the core search
  engine contract remain untouched.
