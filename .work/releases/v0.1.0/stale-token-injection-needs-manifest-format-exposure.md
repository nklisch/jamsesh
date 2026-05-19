---
id: stale-token-injection-needs-manifest-format-exposure
kind: story
stage: done
tags: [testing, infra, portal]
parent: null
depends_on: []
release_binding: v0.1.0
gate_origin: null
created: 2026-05-18
updated: 2026-05-18
---

# Expose objectstore manifest format for e2e fencing tests

## Context

The e2e test `TestStaleFencingTokenRejected` (tests/e2e/failure/stale_fencing_token_rejected_test.go)
skips three subtests because the test fixture can't easily inject a stale
fencing token into a real MinIO bucket. The skips were filed against a
placeholder story name that didn't actually exist — this is that story,
filed for real.

## Required work

Re-architect TestStaleFencingTokenRejected to (1) trigger an actual git push
that creates a real manifest in MinIO, (2) parse the manifest using
`objectstore.Manifest`'s production types rather than a shadow
`staleManifest` struct, (3) use `Backend.Put` (unconditional overwrite) to
inject a stale-token version, (4) verify the manifest-layer guard rejects
the subsequent push.

May require exposing `objectstore.Manifest` (or a parse helper) as a public
API for tests. Evaluate whether that breaks the package boundary discipline
first.

## Three subtests currently skipped

At tests/e2e/failure/stale_fencing_token_rejected_test.go:186, 201, 226 —
each handles a different precondition (missing manifest, unparseable JSON,
PutObject failure).

## Implementation notes

### Package-boundary decision

`objectstore.Manifest` and `objectstore.PackEntry` are already exported
(capitalized) types. However, the e2e test suite lives in a separate Go module
(`jamsesh/tests/e2e`) with no `replace` directive pointing to the main
`jamsesh` module and no go.work workspace. Importing `jamsesh/internal/...`
from this module fails at `go test -c` time.

**Resolution**: use `map[string]interface{}` for manifest JSON manipulation.
Two helpers replace the shadow struct and the production-type import:

- `manifestFencingToken(t, label, data []byte) int64` — extracts
  `fencing_token` from raw manifest JSON without a typed struct.
- `forgeManifestToken(t, data []byte, newToken int64) []byte` — decodes the
  manifest into a generic map, overwrites only `fencing_token`, and
  re-encodes. All other fields (including `updated_at` as raw JSON) are
  preserved byte-exactly from the production manifest.

This approach is strictly safer than a typed shadow struct: it round-trips the
production JSON without re-encoding any field whose type might diverge
(e.g. `time.Time` in production vs `string` in the shadow struct — both
serialize to a JSON string, but a future production change could add a new
field that the shadow struct would silently drop).

### Three skip → fatal conversions

1. **Skip 1** (manifest not found): replaced with `staleFencingWaitForManifest`
   — polls MinIO for up to 10 s, then `t.Fatal`. The manifest is written
   synchronously by `SyncPushPath` before returning 200 OK; the poll absorbs
   sub-second container timing only.

2. **Skip 2** (unparseable JSON): replaced with `t.Fatalf`. If the manifest
   is unreadable JSON that is a production schema bug, not a test gap.

3. **Skip 3** (PutObject failure): replaced with `t.Fatalf`. `mn.PutObject`
   is an unconditional MinIO write; it cannot fail unless the container is
   unreachable, which would have failed earlier steps already.

### Verification status

- `go build ./...` — clean.
- `go test ./internal/portal/storage/objectstore/...` — pass.
- `cd tests/e2e && go test -c -o /dev/null ./failure/` — compiles cleanly.
- `cd tests/e2e && go test ./failure/ -run TestStaleFencingTokenRejected` —
  BLOCKED by a pre-existing concurrent DB migration race in the clustered-mode
  e2e fixture. Both `TestStaleFencingTokenRejected` and `TestLeaseAlreadyHeld`
  fail with the same error:
  `ERROR: duplicate key value violates unique constraint "pg_type_typname_nsp_index"`
  This is an environment-level bug in portalcluster.Start that races two portal
  pods both running migrations concurrently. It is NOT introduced by this story.
  Filed as a separate backlog item (see Implementation discovery below).

### Implementation discovery

Pre-existing bug found while exercising the test: when portalcluster.Start
spins up two portal pods in parallel against a fresh Postgres database, both
pods attempt to run SQL migrations concurrently. Postgres reports a duplicate-key
violation on a pg_type constraint. This causes pod startup to fail with exit
code 1, blocking all clustered-mode e2e tests. See backlog item
`clustered-portal-concurrent-migration-race` (to be filed via
/agile-workflow:park after this commit).

## Review (2026-05-18)

**Verdict**: Approve

**Blockers**: none
**Important**: none
**Nits**:
- End-to-end execution of `TestStaleFencingTokenRejected` is gated on the
  parked `clustered-portal-concurrent-migration-race` fix. Implementer of
  that follow-up should run this test as part of their verification —
  structural correctness is in place, but a successful run-through has not
  been observed locally yet.
- The map-based JSON manipulation is the right call here, but consider
  whether the e2e module should grow a `go.work` workspace or a `replace`
  directive at some point so future tests can use production types
  (compile-time safety on schema drift). Not for this story.

**Notes**: The `map[string]interface{}` approach is actually safer than a
shadow struct — it round-trips the production manifest byte-for-byte and only
mutates the one field under test. The three `t.Skipf` calls are replaced with
real assertions (with a 10s manifest poll for the timing-dependent case).
Discovery of the migration race during execution was handled correctly:
parked as a separate item rather than bundled, noted in the implementation
notes, and the story's structural scope was completed honestly.
