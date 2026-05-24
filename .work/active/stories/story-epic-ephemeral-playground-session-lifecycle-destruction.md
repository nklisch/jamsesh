---
id: story-epic-ephemeral-playground-session-lifecycle-destruction
kind: story
stage: implementing
tags: [portal, playground]
parent: feature-epic-ephemeral-playground-session-lifecycle
depends_on: [story-epic-ephemeral-playground-session-lifecycle-rest-endpoints]
release_binding: null
gate_origin: null
created: 2026-05-23
updated: 2026-05-23
---

# Destruction worker + cascade routine

## Scope

Story 2 of the parent feature. Owns the background subsystem that
walks active playground sessions every N seconds, identifies expired
ones (idle past `idle_timeout_at` OR wall-clock past `hard_cap_at`),
and runs the destruction cascade: revoke bearers → delete comments &
conflict_events → delete session row (FK cascade handles
session_members + events + presence + oauth_tokens via session_id FK)
→ delete anonymous accounts → remove bare repo from disk.

The tombstone insertion happens BEFORE the session row is deleted (so
the summary stats are computed while everything is still queryable).

Full design including code skeletons and per-step destruction logic
is in the parent feature body's "Story 2" section.

## Files delivered

- `internal/portal/playground/worker.go` — Run + sweep + reasonFor
- `internal/portal/playground/destruction.go` — Destroy cascade
- `internal/portal/playground/worker_test.go`
- `internal/portal/playground/destruction_test.go`
- `cmd/portal/main.go` (modify) — start worker on boot when enabled;
  participate in graceful shutdown via the existing WaitGroup pattern
- `db/queries/{sqlite,postgres}/sessions.sql` (extend) —
  `ListExpiredPlaygroundSessions`, `DeleteAnonymousAccountsByIDs`,
  `PurgeExpiredTombstones`

## Acceptance criteria

See the parent feature body's "Story 2 acceptance criteria" section.
Summary: worker ticks at configured interval, destruction cascade is
correct + idempotent, partial-failure resilience (next sweep retries),
graceful shutdown stops worker within one tick, tombstone TTL purge
runs.

## Notes for the implementing agent

- Destruction is NOT transactional across all steps — the bare-repo
  delete is a filesystem op outside the DB. Each step must be
  idempotent so partial failures are safely retried by the next sweep.
- Step ordering matters: tombstone insert BEFORE session row delete
  (need the session row to compute summary stats); bearer revoke
  BEFORE session row delete (defense in depth, even though
  oauth_tokens.session_id FK has ON DELETE CASCADE); anonymous account
  delete AFTER session_members cascade (need the account_ids captured
  BEFORE the cascade removes the join link).
- Worker integration with `cmd/portal/main.go` follows the existing
  `wg.Add(1); go func() { defer wg.Done(); ... }()` pattern. Use the
  shutdown context that's already wired into the WaitGroup.
- For clustered-mode safety, the destruction routine should wrap the
  per-session work in a PG advisory lock acquired via the existing
  LeaseManager infrastructure. Under single-instance (default),
  LeaseManager is `NoopManager` so the lock is a no-op. See the
  `Risks` section of the parent feature body for the rationale.
- Tombstone TTL: 30 days default; purge sub-routine can run as part
  of the main sweep (every Nth tick, e.g.) or as its own goroutine.
  Implementer's call.

## Cross-story notes

- This story **depends on Story 1** (REST endpoints) because the
  destruction worker's sweep query reads schema columns
  (`hard_cap_at`, `idle_timeout_at`, `last_substantive_activity_at`)
  AND inserts into the `tombstones` table — both of which are
  introduced by Story 1's migration. The dep is declared in this
  story's frontmatter.
- The parent feature body's "Implementation order" originally said
  Stories 1 + 2 are independent (wave 2a parallel); the dep declared
  here makes Story 2 land in wave 2b alongside Stories 3 and 4. The
  feature body's order section is approximate; the substrate (this
  story's `depends_on`) is authoritative.
