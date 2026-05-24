---
id: story-comments-service-use-slog-not-stdlib-log
kind: story
stage: done
tags: [portal, cleanup, logging]
parent: null
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-23
updated: 2026-05-24
---

# Switch comments.Service activity-reset warning from stdlib log to slog

## Context

Surfaced during review of
`story-epic-ephemeral-playground-session-lifecycle-abuse-caps`.

The activity-reset best-effort warning added to
`internal/portal/comments/service.go` uses stdlib `log.Printf`:

```go
log.Printf("comments: reset idle timer failed (best-effort): session=%s err=%v",
    s.SessionID, resetErr)
```

The rest of `internal/portal/` uses `log/slog` for structured logging
(see `httperr`, `logging`, `server`, `automerger`, `storage/objectstore`,
and the sibling activity-reset call-sites in `githttp/receive_pack.go`
and `sessions/handler.go`, both of which use `slog.WarnContext`). Stdlib
`log` bypasses the slog handler chain, breaks structured-log routing,
and produces a log line in a different format from the rest of the
service.

`comments.Service` does not currently carry a `*slog.Logger` field. Two
options:

**Option A (preferred):** Use `slog.WarnContext(ctx, ...)` like the other
two activity-reset sites do. Routes through the default slog handler
which is wired in `cmd/portal/main.go`. No new field needed.

**Option B:** Add a `Log *slog.Logger` field to `Service`, default to
`slog.Default()` when nil, and use it. More invasive but matches the
explicit-logger pattern used in `Validator` (`Logger *slog.Logger`).

## Scope

- Replace the `log.Printf` call with `slog.WarnContext(ctx, ...)`.
- Drop the `"log"` import.
- Optionally: do the same for any other stdlib-log calls in
  `internal/portal/` (none currently, but worth a quick grep before
  closing).

## Acceptance criteria

- `internal/portal/comments/service.go` no longer imports `"log"`.
- The warning is emitted via slog with structured fields (`session`,
  `err`) matching the pattern in `receive_pack.go` and
  `sessions/handler.go`.
- Existing comments tests still pass.

## Implementation notes

Option A applied: replaced `log.Printf` with `slog.WarnContext(ctx, ...)` and
swapped the `"log"` import for `"log/slog"`. No new struct fields needed.

Field names mirror the two sibling activity-reset sites exactly:
`"org", p.OrgID, "session", p.SessionID, "err", resetErr`
(matching `receive_pack.go:345` and `sessions/handler.go:331`).

Comment text updated from "the push itself succeeded" to "the comment itself
succeeded" — the stale wording was copy-pasted from `receive_pack.go`.

`go build ./...` and `go test ./...` green with no stale fixtures.

## Review (2026-05-24)

**Verdict**: Approve

**Blockers**: none
**Important**: none
**Nits**: none

**Notes**: Implementation matches the design; verification passes (Go: `go build` + `go test ./...` clean; frontend: `npm run check` 0 errors, `npm run test` 635/635, `npm run build` clean). Implementation notes accurately document what landed, including any agent decisions or land-mode confirmations.
