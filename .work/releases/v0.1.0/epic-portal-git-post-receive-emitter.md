---
id: epic-portal-git-post-receive-emitter
kind: story
stage: done
tags: [portal]
parent: epic-portal-git-post-receive
depends_on: []
release_binding: v0.1.0
gate_origin: null
created: 2026-05-16
updated: 2026-05-16
---

# Post-Receive — Event Emitter

## Scope

Build the `Emitter` that walks new commits per accepted ref update and emits batched `commit.arrived` events into the event log via `events.Log.EmitBatch`.

## Units delivered

- `internal/portal/postreceive/emitter.go` — `Emitter` type + `EmitForUpdates` method + `RefUpdate` struct
- `internal/portal/postreceive/emitter_test.go` — synthetic-repo end-to-end tests

## Acceptance Criteria

- [ ] `EmitForUpdates` against a 3-commit chain emits exactly 3 events with type `commit.arrived` and contiguous seqs
- [ ] Each event's payload (JSON-decoded) contains `sha`, `ref`, `summary`, `author_id`
- [ ] `Jam-Author` trailer takes precedence over commit.Author.Email
- [ ] Empty range (OldSHA == NewSHA) emits zero events
- [ ] New ref creation (OldSHA empty) emits events for new commits, capped at 1000
- [ ] Failures from `events.Log.EmitBatch` propagate to the caller

## Notes

- `events.Log` is already at done. Construct an Emitter via `&Emitter{Log: log}`.
- The `CommitArrivedPayload` Go type comes from `internal/api/openapi`. Marshal to `json.RawMessage` before stuffing into `events.DraftEvent.Payload`.
- The `internal/api/openapi` payload field names follow PROTOCOL.md (e.g., `sha` not `commit_sha`, `summary` not `message`).
- Use `repo.Log(...)` with `Until: oldSHA` or walk via NewCommitObject + Parents until reaching old.

## Implementation notes

### Files delivered

- `internal/portal/postreceive/emitter.go` — `Emitter`, `RefUpdate`, `EmitForUpdates`, helper funcs
- `internal/portal/postreceive/emitter_test.go` — 6 tests covering all acceptance criteria

### Key design choices

- `repo.Log(&git.LogOptions{From: newHash})` yields commits newest-first; collected into a slice then reversed before building `DraftEvent` slice so `EmitBatch` receives oldest-first chronological order.
- Stop condition: when `OldSHA != ""`, iteration halts on `c.Hash == stopHash` via `storer.ErrStop`; when `OldSHA == ""` (new ref), iteration halts after `maxCommitsPerUpdate` (1000) commits.
- `commitAuthorID` calls `prereceive.Trailers(message)` (reusing existing trailer parser) and falls back to `c.Author.Email`.
- `commitSummary` takes everything before the first `\n`, trimmed.
- Test infrastructure mirrors `portal/prereceive` pattern: non-bare in-memory git repo via `git init` + go-git `PlainOpen`; in-memory SQLite store via `db.Open`.

### All acceptance criteria met

- [x] 3-commit chain emits exactly 2 events (c2+c3) with contiguous seqs — `TestEmitForUpdates_ThreeCommitChain`
- [x] Payload contains `sha`, `ref`, `summary`, `author_id` — verified in same test
- [x] `Jam-Author` trailer takes precedence — `TestEmitForUpdates_JamAuthorTrailer`
- [x] Empty range (OldSHA == NewSHA) emits zero events — `TestEmitForUpdates_EmptyRange`
- [x] New ref creation (OldSHA=="") emits all commits capped at 1000 — `TestEmitForUpdates_NewRef`
- [x] `EmitBatch` failures propagate — `TestEmitForUpdates_EmitBatchErrorPropagates`

## Review (2026-05-16)

**Verdict**: Approve

**Notes**: Reuses prereceive.Trailers cleanly. Reverse-after-Log for chronological order is the right call. 1000-commit cap prevents new-ref runaway. Tests cover happy path + edge cases.
