---
id: epic-auto-merger-outcomes-apply
kind: story
stage: implementing
tags: [portal]
parent: epic-auto-merger-outcomes
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-17
updated: 2026-05-17
---

# Auto-Merger Outcomes — Apply

## Scope

Single story: ship the `conflict_events` schema + Store extension + `Apply` entrypoint that turns a MergeResult into side effects (merge commit + ref advance + event emission, or conflict_events row + event emission).

## Units delivered

- `internal/db/migrations/{sqlite,postgres}/00008_conflict_events.sql`
- `db/schema/{sqlite,postgres}.sql` (edit)
- `db/queries/{sqlite,postgres}/conflict_events.sql` — Insert, GetByID, MarkResolved, ListOpenForSession
- Regen sqlitestore + pgstore
- `internal/db/store/store.go` (edit) — ConflictEventStore sub-interface + domain type
- Both adapters
- `internal/portal/automerger/outcomes.go` — Apply entrypoint + helpers
- `internal/portal/automerger/addressing.go` — computeAddressedTo
- Tests

## Acceptance Criteria

- [ ] Clean merge: Apply creates merge commit with author=source-author, committer=auto-merger, trailers (Auto-Merger:true, Source-Commit, Source-Ref), advances draft, emits merge.succeeded
- [ ] Safe-auto-resolve: same + Auto-Resolved:<heuristic> trailer
- [ ] Hard-conflict: inserts conflict_events row, emits conflict.detected, leaves draft unchanged
- [ ] Resolves-Conflict trailer on source: marks matching open event resolved, emits conflict.resolved
- [ ] Mismatch (unknown event-id): silent no-op
- [ ] computeAddressedTo: walks back up to 100 draft commits, includes source-ref owner + each conflicted-file's last-modifier
- [ ] `go test ./internal/portal/automerger/...` green; `go build ./...` clean

## Notes

- Auto-merger identity: `Name: "jamsesh auto-merger", Email: "auto-merger@<portalHost>"`. portalHost is parsed from cfg.PortalURL.
- The Apply function takes an `events.Log` + `store.Store` via constructor.
- For draft ref advance use `repo.Storer.SetReference(plumbing.NewHashReference(draftRefName, newSHA))`.
