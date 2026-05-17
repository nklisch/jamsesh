---
id: epic-finalize-flow-plan-generation-fetch-token-and-mark-shipped
kind: story
stage: done
tags: [portal]
parent: epic-finalize-flow-plan-generation
depends_on: [epic-finalize-flow-plan-generation-locks-schema-and-rest]
release_binding: null
gate_origin: null
created: 2026-05-17
updated: 2026-05-17
---

# Finalize Fetch-Token and Mark-Shipped

## Scope

Land the two terminal endpoints in the finalize surface: the
ephemeral fetch-token endpoint that backs the plugin's HTTPS-fallback
path, and the manual mark-shipped transition that moves the session
from `finalizing → ended` with `end_reason: "shipped"`. Also extends
the `session.ended` event payload enum to include `shipped`.

Together with stories 1 and 2 this completes the feature: the
portal-API surface that backs both the curation UI and the plugin's
finalize-run command, end to end.

## Units delivered

- **`internal/portal/finalize/fetch_token.go`** —
  `func (h *Handler) IssueFetchToken(ctx, req) (resp, error)`.
  Verifies org membership + session membership for the caller.
  Calls `h.tokens.IssueShortLived(ctx, callerAccountID,
  5*time.Minute)` (added in story 1). Returns `FetchTokenResponse`:
  - `token`: raw access token string
  - `remote_url`: `"https://x-access-token:" + token + "@" +
    portalHost + "/git/" + orgID + "/" + sessionID + ".git"`
    (portalHost derived from `h.portalURL` with scheme stripped;
    if `portalURL` is already an `https://` URL, splice the token
    into its authority component cleanly using `url.Parse`).
  - `expires_at`: the token pair's `AccessExpiresAt`
- **`internal/portal/finalize/fetch_token_test.go`** —
  Cases: happy path returns a token that
  `tokens.Service.Validate` accepts immediately, the token expires
  after 5 min (test uses an injected fake clock — reuse
  `tokens.NewWithClock` in test setup), `remote_url` carries the
  token in the userinfo segment, non-member returns 403.
- **`internal/portal/finalize/mark_shipped.go`** —
  `func (h *Handler) MarkSessionShipped(ctx, req) (resp, error)`.
  Logic:
  1. Membership check on caller (any session member can mark
     shipped — not creator-only; the user kicking off the ship
     might not be the creator).
  2. Load session. If `status == "ended"` with `end_reason ==
     "shipped"`: idempotent — return 200 with the session row.
  3. If `status == "ended"` with a different `end_reason`: 409
     `session.already_ended` with `details.end_reason`.
  4. If `status == "active"`: 409 `session.not_finalizing` (must
     finalize first).
  5. Otherwise (`finalizing`): transition `status = "ended"`,
     `end_reason = "shipped"`, `ended_at = now()` via existing
     `UpdateSessionStatus` + `SetSessionEndReason` queries.
  6. If `req.Body.FinalBranchName` is non-nil, cache it on the
     session row for the eventual archive step. (The archive
     itself happens via the existing archive scheduler /
     post-finalize sweep in `storage.ArchiveSession`; this story
     does NOT immediately archive — it only flips status. The
     archival sweep that runs after the retention window picks up
     the `final_branch_name` from a small new column on `sessions`
     OR — simpler — passes it through into `storage.ArchiveSession`
     when the sweep runs, by reading from a column we ALREADY
     have. Since `sessions` has no `final_branch_name` column,
     story scope: pass `final_branch_name` straight into a new
     `ArchiveInfo.FinalBranchName` via the archive path. If
     the archive scheduler reads from a column, add that column
     in this migration. Otherwise store in-memory in `events`
     payload of `session.ended` — and let the archive sweep
     read the latest event. Implementation choice: cache on
     the `archived_sessions.final_branch_name` column DIRECTLY
     when the archive sweep runs; this story only emits the
     value in the `session.ended` event payload, and the archive
     sweep reads from there. No new column needed.)
  7. If a finalize lock is still held for the session, release it
     as part of the transition (set `released_at` on the active
     lock row). Mark-shipped means the run completed — the lock is
     no longer needed.
  8. Emit `session.ended` event with payload
     `{reason: "shipped", final_branch_name: <optional>}`.
- **`internal/portal/finalize/mark_shipped_test.go`** —
  Cases: happy transition `finalizing → ended/shipped`; idempotent
  re-call returns 200 with same row; 409 when active (not
  finalizing); 409 when already ended with `abandoned`; emits
  `session.ended` with `reason: "shipped"`; releases held
  finalize lock on success; non-member returns 403.
- **OpenAPI additions** — `docs/openapi.yaml`:
  - Schemas: `FetchTokenResponse`, `MarkShippedRequest`.
  - **`SessionEndedPayload.reason` enum extension**: add `"shipped"`
    to the existing `enum: [finalize, abandon, timeout]` —
    becomes `[finalize, abandon, timeout, shipped]`. (Note: enum
    is also extended on `archived_sessions.end_reason` CHECK
    constraint via a small follow-on migration **00011** —
    `00011_end_reason_shipped.sql` — to add `'shipped'` to the
    accepted values; sqlc regenerates models, no Go change
    needed.)
  - Paths:
    `/api/orgs/{orgID}/sessions/{sessionID}/finalize/fetch-token`
    POST → 201 `FetchTokenResponse`; standard 401/403/404.
    `/api/orgs/{orgID}/sessions/{sessionID}/mark-shipped`
    POST with body `MarkShippedRequest` → 200 `Session`;
    409 `session.not_finalizing` / `session.already_ended`;
    standard 401/403/404.
- **Mark-shipped + fetch-token wired into the handler** — replaces
  the `501 not_implemented` stubs from story 1.
- **Migration 00011** —
  `internal/db/migrations/{sqlite,postgres}/00011_end_reason_shipped.sql`.
  Postgres: drop and re-add the `archived_sessions_end_reason_check`
  CHECK constraint with `IN ('finalize','abandon','timeout','shipped')`.
  SQLite: table-rebuild dance (like 00006) to widen the CHECK.

## Acceptance Criteria

- [x] `make generate` succeeds
- [x] `go build ./...` clean
- [x] `go test ./internal/portal/finalize/...` green (all 3 stories'
      test suites pass together)
- [x] `IssueFetchToken` returns a token that `tokens.Validate`
      immediately accepts and that expires 5 minutes after issuance
      (fake-clock test)
- [x] `FetchTokenResponse.remote_url` carries the token in the
      `x-access-token:<token>@` userinfo segment
- [x] `IssueFetchToken` from a non-session-member returns 403
- [x] `MarkSessionShipped` on a `finalizing` session transitions to
      `ended` with `end_reason = "shipped"` and `ended_at` set
- [x] `MarkSessionShipped` is idempotent when already
      `ended` + `shipped`
- [x] `MarkSessionShipped` returns 409 `session.not_finalizing` when
      called on `active`
- [x] `MarkSessionShipped` returns 409 `session.already_ended` when
      called on `ended` with a different `end_reason`
- [x] `MarkSessionShipped` releases any held finalize lock
      (`released_at` set on the row, sessions pointer cleared)
- [x] `MarkSessionShipped` emits `session.ended` event with
      `reason: "shipped"` and optional `final_branch_name`
- [x] The OpenAPI `SessionEndedPayload.reason` enum lists
      `shipped` as an accepted value
- [x] Migration 00011 widens the SQLite + Postgres CHECK
      constraint on `archived_sessions.end_reason` to accept
      `shipped`; `make generate-db` succeeds; existing tests still pass

## Implementation notes

- **OpenAPI extensions** — `docs/openapi.yaml` gained:
  - `SessionEndedPayload.reason` enum extended to `[finalize, abandon,
    timeout, shipped]`, plus an optional `final_branch_name` property
    carried only when reason=shipped.
  - `FetchTokenResponse` and `MarkShippedRequest` schemas.
  - Two new paths under `/api/orgs/{orgID}/sessions/{sessionID}/`:
    `finalize/fetch-token` (POST → 201 FetchTokenResponse) and
    `mark-shipped` (POST → 200 Session).

- **Handlers** — `internal/portal/finalize/fetch_token.go` and
  `internal/portal/finalize/mark_shipped.go` follow the existing
  strict-server-method pattern. Wired into `cmd/portal/main.go`'s
  `combinedHandler` as two new delegations.

- **Token issuance** — `IssueFetchToken` uses the
  `tokens.Service.IssueShortLived` method added in story 1 with a
  package-local `fetchTokenTTL = 5*time.Minute`. The remote URL is
  composed via `url.Parse` so scheme/host/port from `cfg.PortalURL`
  flow through unchanged for both `https://portal.example.com` and
  `http://localhost:8080` configurations; userinfo is set via
  `url.UserPassword("x-access-token", rawToken)` so URL-unsafe token
  bytes are correctly encoded.

- **Mark-shipped semantics** — Any session member (not just creator)
  may mark a session shipped, since the user kicking off the ship may
  not have created the session. State machine:
  - `finalizing` → `ended/shipped`: status updated via
    `UpdateSessionStatus`, end_reason + ended_at via
    `SetSessionEndReason`, any active lock released via
    `GetActiveFinalizeLockForSession` + `ReleaseFinalizeLock`
    (idempotent on missing-lock case via `errors.Is(err,
    store.ErrNotFound)`), sessions pointer cleared via
    `ClearFinalizeLock` (only when it still points at the lock holder,
    mirroring `lock_release.go`).
  - `ended/shipped` → 200 idempotent: same row returned, no event
    emitted (verified by post-subscribe test).
  - `ended/<other>` → 409 `session.already_ended` with
    `details.end_reason` set.
  - `active` → 409 `session.not_finalizing`.

- **Event emission** — `session.ended` payload uses
  `{reason, final_branch_name?}` matching the openapi schema. JSON
  uses `omitempty` so the field is absent (rather than null) when not
  supplied — consumers see exactly the shape the schema defines.
  Best-effort emission via `h.events.Emit` (return value ignored,
  matches sessions.AbandonSession).

- **sessionToOpenAPI** — duplicated locally in `mark_shipped.go`
  rather than imported from `sessions`. Keeps `finalize` from
  depending on `sessions` (which would create a circular-ish import
  cluster); the helper is small and the byte-for-byte mirror is
  intentional. If a third package ever needs the same projection,
  promote it to a shared `internal/portal/sessionsdto` package.

- **Migration 00011** — Postgres uses `DROP CONSTRAINT IF EXISTS`
  + `ADD CONSTRAINT` (one-statement-each) since Postgres supports
  CHECK alteration directly. SQLite uses the table-rebuild pattern
  from 00006_sessions_lifecycle: `PRAGMA foreign_keys = OFF`, create
  `archived_sessions_new`, copy rows, drop old, rename, recreate
  `archived_sessions_org_idx`, `PRAGMA foreign_keys = ON`. No other
  tables FK into `archived_sessions` so the rebuild is safe (no
  escape-hatch trigger). Down migration is symmetric.

- **sqlc generated code** — no SQL changes needed; `sqlc generate`
  ran cleanly. The CHECK widening is a schema-level concern, not a
  query shape.

- **Tests** — 7 fetch-token tests + 10 mark-shipped tests = 17 new
  test functions. Tests reuse the `newFinalizeEnv` helper from
  `testhelpers_test.go` (story 1); fetch-token tests use a private
  `newFetchTokenEnv` because they need a `tokens.Service` with an
  injected `fakeClock` to assert TTL expiry deterministically. The
  no-event-on-reship test subscribes AFTER the first mark-shipped to
  avoid racing the first emission.

- **Cross-package interface updates** — adding `IssueFetchToken` and
  `MarkSessionShipped` to `openapi.StrictServerInterface` broke five
  test-only stubs (`magicLinkOnlyStrict`, `oauthOnlyStrict`,
  `sessionsOnlyStrict`, `accountsOnlyStrict`, `tokensOnlyHandler`,
  `commentsOnlyStrict`). Added `panic("not wired")` stubs to each so
  the compile-time `var _ StrictServerInterface = (*X)(nil)`
  assertions still hold.

- **No design-flaw escape hatches triggered.** The SQLite rebuild was
  clean; no FK conflicts; no surprises from the strict-server
  interface; no semantics-gap between the parent feature's design and
  what the implementation needed.

## Files touched

- `internal/portal/finalize/{fetch_token,mark_shipped}.go` (new)
- `internal/portal/finalize/{fetch_token,mark_shipped}_test.go` (new)
- `internal/db/migrations/{sqlite,postgres}/00011_end_reason_shipped.sql` (new)
- `internal/db/sqlitestore/*` (regenerated by sqlc — no query changes,
  models may regenerate)
- `internal/db/pgstore/*` (regenerated by sqlc)
- `docs/openapi.yaml` (add schemas + 2 paths + 1 enum value)
- `internal/api/openapi/server.gen.go` (regenerated)
- `frontend/src/lib/api/schema.d.ts` (regenerated)
- `cmd/portal/main.go` (no further changes — story 1 wired the handler)

## Review (2026-05-17)

**Verdict**: Approve

**Notes**: SQLite table-rebuild dance for CHECK widening matches 00006 pattern. Mark-shipped releases held lock on transition. 17 tests covering happy + idempotent + 5 error paths each.
