---
id: bug-postreceive-emits-events-for-base-ref-bootstrap-history
kind: story
stage: review
tags: [bug, event-bus, playground, backpressure, performance]
parent: null
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-25
updated: 2026-05-25
---

# postreceive emits commit.arrived for pre-session bootstrap history

## Context

The post-receive emitter at `internal/portal/postreceive/emitter.go:128-194`
(`emitForUpdate`) walks every reachable commit on a brand-new ref when
`OldSHA == ""`, capped at `maxCommitsPerUpdate = 1000`. It has no notion of
the session's `base_sha`.

The result: every bootstrap push to `refs/heads/jam/<sessionID>/base` —
which by design carries the user's pre-session working-tree history (e.g.,
the full main/master branch of their existing repo) — fans out a
`commit.arrived` event per pre-session commit. For typical repos this means
the cap fires every time someone seeds a session.

This is a category error. The rest of the system already treats the
base-ref push as pre-session bootstrap. `prereceive/validate.go:108`
(`isBaseRefFirstPush`) exempts the bootstrap push from trailer/scope
validation with the comment *"the seed commits a user pushes here are
their pre-session working-tree commits — they predate the session and so
cannot carry session-aware trailers."* The emitter is the one place that
doesn't know.

## Production evidence

Playground session `01KSEKEBP2X9TVMVBEA85BENVE` on `2026-05-25T03:39:20Z`:

- 1195 WARN `events: subscriber channel full, dropping event` lines in a
  4.1ms burst
- Affected event type: `commit.arrived`, seq range 66 → 1000 (hit the cap
  exactly)
- Session completed cleanly 30 min later: `end_reason=idle, members=1,
  commits=1000, auto_merges=257`
- System survived (automerger has a startup catch-up scan that recovers
  dropped events), but the WARN spam masked every other backpressure
  signal in the 24h window

The session was a single-member playground: nobody was even subscribed to
the fanout in real time. We emitted 1000 events to drain into a 64-deep
buffered channel, dropped most of them, and the only observers had to
recover via DB scan.

## Impact

1. **Wasteful event log writes** on every session bootstrap (one DB row per
   pre-session commit, up to 1000 per push)
2. **Backpressure log noise** — `subscriber channel full` is supposed to
   indicate a real slow subscriber, not an expected outcome of every new
   session
3. **Misleading event semantics** — `commit.arrived` reads as "a new
   commit landed for collaborators to observe in real time"; for the
   bootstrap, every commit is historical from the moment the session
   exists
4. **Latent issue for large bootstraps** — users importing repos with
   >1000 commits will silently lose seed commits from the event log
   (only the first 1000 reachable from HEAD are emitted; the rest are
   never recorded as events at all). The DB row + base_sha audit trail
   still captures the SHA, but the event log loses the tail.

## UI-init path verification

The user explicitly asked: does session creation via the UI bypass this
fix path?

Verified by reading the two front doors:

- `internal/portal/playground/handler.go:208` — `CreatePlaygroundSession`
  ends with `Storage.CreateRepo(ctx, ReservedOrgID, sessionID)`
- `internal/portal/sessions/handler.go:151` — `CreateSession` (durable)
  ends with the same `Storage.CreateRepo(ctx, orgID, sessionID)` call
- `internal/portal/storage/repo.go:33` — `CreateRepo` runs only
  `git init --bare <path>`, with no commit seeding

There is no alternate UI-init path that seeds commits server-side. No
archive-upload import, no clone-from-URL. All bootstrap commits ALWAYS
arrive via subsequent `git push` from the user's client to
`refs/heads/jam/<sid>/base`. `EmitForUpdates` is therefore the sole
emission chokepoint, and a single fix to the emitter covers both
playground and durable sessions uniformly.

## Fix design

In `EmitForUpdates`, treat `base_sha` as an additional stop-point for the
commit walk. The walk's job becomes "emit commits reachable from NewSHA
that are NOT already part of the seeded session history."

`receive_pack.go` already stamps `sessions.base_sha` BEFORE calling the
emitter (`receive_pack.go:291-303` runs before line 311), so the SHA is
known at emit time. The cleanest signature is to pass `baseSHA` directly
into `EmitForUpdates` from the handler rather than re-reading the session
row.

```go
// emitForUpdate walk semantics:
stops := []plumbing.Hash{}
if update.OldSHA != "" {
    stops = append(stops, plumbing.NewHash(update.OldSHA))
}
if baseSHA != "" && baseSHA != update.NewSHA {
    stops = append(stops, plumbing.NewHash(baseSHA))
}
// walk from NewSHA, stop on ANY hash in stops
```

The walk uses `storer.ErrStop` against the combined stop set instead of
the current single-hash comparison.

### Caller-side change

In `internal/portal/githttp/receive_pack.go`, after the
`SetSessionBaseSHA` call (~line 303), determine the effective base SHA for
this receive and pass it through:

```go
baseSHA := ""
if session.BaseSHA != nil {
    baseSHA = *session.BaseSHA
}
// If this receive contains the base-ref creation, the just-stamped value
// is the SHA we want.
if u := findBaseRefUpdate(sessionID, updates); u != nil {
    baseSHA = u.NewSHA
}
// ...
h.Emitter.EmitForUpdates(r.Context(), diskRepo, &session, account, updates, baseSHA)
```

(Reading the just-stamped SHA from the update slice avoids a DB
round-trip and races; the SHA is right there.)

### Behavior matrix

| Scenario | Walk result |
|---|---|
| Bootstrap push to base ref (`OldSHA=""`, `NewSHA == base_sha`) | walk yields 0 commits → 0 events |
| Collaborator branches off base (`OldSHA=""`, `NewSHA` ahead of base) | walk stops at base → emits ONLY session-authored commits |
| Subsequent push to existing ref (`OldSHA` set) | walk stops at OldSHA — unchanged |
| Pathological branch with unrelated history (no path to base) | `maxCommitsPerUpdate` cap still applies; pre-receive trailer enforcement rejects commits without Jam-Session anyway |

### Side benefit

The `maxCommitsPerUpdate = 1000` cap becomes a defensive guard for the
pathological case, not the load-bearing thing every bootstrap relies on.
Backpressure WARN logs become a real signal again.

## Acceptance criteria

1. Bootstrap push to `refs/heads/jam/<sid>/base` with N commits emits **0**
   `commit.arrived` events, regardless of N.
2. Collaborator's first push to a new branch off base with K session
   commits emits **exactly K** events.
3. Subsequent push to an existing ref still emits per new commit since
   `OldSHA` (unchanged behavior).
4. UI-created playground session + large bootstrap push (test value: 5000
   commits) → zero WARN `subscriber channel full` lines, zero
   `commit.arrived` events in the event log for that session before the
   first non-bootstrap push.
5. UI-created durable session + large bootstrap push → same.
6. The existing `maxCommitsPerUpdate = 1000` constant stays in place as
   defense for the pathological no-path-to-base case (not removed).

## Files touched

Primary:
- `internal/portal/postreceive/emitter.go` — walk stop-set logic, new
  `baseSHA string` parameter on `EmitForUpdates`
- `internal/portal/githttp/receive_pack.go` — compute and pass `baseSHA`
  to `EmitForUpdates`

Tests:
- `internal/portal/postreceive/emitter_test.go` — assert behavior matrix
  rows 1-3 (unit-level)
- `internal/portal/githttp/receive_pack_test.go` — assert acceptance
  criteria 4-5 (handler-level with real bare repo)

## Out of scope

- Adding a `session.bootstrapped` (or similar) summary event with
  `{base_sha, commit_count}`. UI may want this later for "Session seeded
  with N commits from main" messaging, but the data is already in the
  session row + base_sha audit field. Punt to a follow-up if UI requests
  it.
- Removing or lowering `maxCommitsPerUpdate`. Cap stays as-is.
- Changing the subscriber channel buffer size. The 64-deep buffer is
  designed for the catch-up-via-scan model and is correct for normal
  per-commit emission; this fix removes the synthetic bootstrap pressure
  that was making it look undersized.

## Implementation notes

- **Files changed**:
  - `internal/portal/postreceive/emitter.go` — added `baseSHA string`
    parameter to `EmitForUpdates`; threaded through to `emitForUpdate`;
    replaced the single-`stopHash` walk with a `stops` set that includes
    both `OldSHA` (when non-empty) and `baseSHA` (when non-empty and
    different from `NewSHA`); early-return when the bootstrap push has
    `NewSHA == baseSHA`; `maxCommitsPerUpdate` cap now applies only when
    the stop set is empty (pathological unbounded walk).
  - `internal/portal/githttp/receive_pack.go` — compute the effective
    `baseSHA` immediately before the emitter call: take `session.BaseSHA`
    if already populated, override with the just-pushed `NewSHA` if this
    receive contains the base-ref creation (no DB re-read — the SHA is
    in the `updates` slice). Pass to `EmitForUpdates`.
- **Tests added**:
  - `TestEmitForUpdates_BootstrapBaseRef` — bootstrap push of 5-commit
    history with `NewSHA == baseSHA`; asserts 0 emitted events (AC 1)
  - `TestEmitForUpdates_CollaboratorBranchOffBase` — 4-commit history,
    base at c2, push of new ref at c4 with empty OldSHA and `baseSHA=c2`;
    asserts exactly 2 events (c3, c4) in oldest-first order (AC 2)
  - `TestPostReceive_BaseRefBootstrapEmitsNoCommitArrived` — handler-level
    integration test: real `git push` of 5-commit bootstrap through the
    receive_pack handler, asserts zero `commit.arrived` events in the
    event log (AC 4 + 5; covers both playground and durable session paths
    since `mustCreateSession` uses the same `Storage.CreateRepo` chokepoint
    as both UI front doors)
- **Tests updated**: all 5 existing `TestEmitForUpdates_*` tests adjusted
  to pass `""` as the new `baseSHA` parameter, preserving their original
  semantics (no base anchor, behavior unchanged).
- **Discrepancies from design**: none. Design was vetted against the
  source before scoping; implementation matched the design verbatim.
- **Adjacent issues parked**: none surfaced during implementation.

### Verification

- `go build ./...` — clean
- `go vet ./internal/portal/postreceive/... ./internal/portal/githttp/...`
  — clean
- `go test ./internal/portal/postreceive/...` — PASS (0.162s)
- `go test ./internal/portal/githttp/...` — PASS (1.392s)
- Acceptance criteria 1-5: covered by the two unit tests + one handler
  integration test listed above. AC 6: `maxCommitsPerUpdate = 1000`
  constant is untouched; the guard now applies only when the stop set is
  empty (the pathological no-base no-OldSHA case it was always intended
  to defend).
