---
id: gate-tests-stale-fencing-token-unskip
kind: story
stage: review
tags: [testing, portal, infra]
parent: null
depends_on: []
release_binding: v0.1.0
gate_origin: tests
created: 2026-05-18
updated: 2026-05-18
---

# `TestStaleFencingTokenRejected` skips on three independent paths — invariant under-asserted

## Priority
High

## Spec reference
Item: `epic-cloud-native-deploy-lease-fencing-postgres` +
`epic-cloud-native-deploy-object-storage-sync-manifest`.
Acceptance criterion: `Save` with fencing token < on-disk token returns
`ErrFenced`; at e2e tier this is the manifest-layer guard.

## Gap type
test-integrity. Three `t.Skipf` calls at
`tests/e2e/failure/stale_fencing_token_rejected_test.go:186, 201, 226`
each shift blame to a not-yet-filed follow-on story
`stale-token-injection-needs-manifest-format-exposure`. The unit test
exists (`manifest_test.go`), but the spec-level "manifest-layer fencing
guard works against a real MinIO" is bypassed at three different
conditions.

## Suggested test
Either (a) file the
`stale-token-injection-needs-manifest-format-exposure` story OR (b)
re-architect the test to (1) trigger an actual push that creates a real
manifest first, (2) parse the manifest using `objectstore.Manifest`'s
production types (no shadow `staleManifest` struct), (3) use
`Backend.Put` unconditional overwrite.

## Test location (suggested)
`tests/e2e/failure/stale_fencing_token_rejected_test.go` and new
`.work/backlog/stale-token-injection-needs-manifest-format-exposure.md`
if approach (a).

## Implementation notes

**Decision: took option (a) — file the follow-on story to backlog.**

Rationale: option (b) is a deep test-architecture refactor (re-wiring
fixture types, exposing production manifest internals to tests). That's more
work than this story's scope. Filing the follow-on preserves the audit trail
of WHY the skips exist and gives a discoverable target for future work.

**New backlog story filed:**
`.work/backlog/stale-token-injection-needs-manifest-format-exposure.md`
(id: `stale-token-injection-needs-manifest-format-exposure`)

**Three skip locations updated** in
`tests/e2e/failure/stale_fencing_token_rejected_test.go` — each `t.Skipf`
now opens with:

```
"blocked on stale-token-injection-needs-manifest-format-exposure (backlog); ..."
```

Lines (post-edit, approximately):
- Line 186: missing manifest from MinIO
- Line 201: manifest not parseable JSON
- Line 226: PutObject unconditional overwrite failure

The backlog story `id:` field and the skip message prefix are an exact
string match — grep will find all three skips from the story id.
