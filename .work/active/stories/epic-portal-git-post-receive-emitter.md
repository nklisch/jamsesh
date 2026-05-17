---
id: epic-portal-git-post-receive-emitter
kind: story
stage: implementing
tags: [portal]
parent: epic-portal-git-post-receive
depends_on: []
release_binding: null
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
