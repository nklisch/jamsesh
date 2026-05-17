---
id: epic-finalize-flow-plan-generation-fetch-token-and-mark-shipped
kind: story
stage: implementing
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

- [ ] `make generate` succeeds
- [ ] `go build ./...` clean
- [ ] `go test ./internal/portal/finalize/...` green (all 3 stories'
      test suites pass together)
- [ ] `IssueFetchToken` returns a token that `tokens.Validate`
      immediately accepts and that expires 5 minutes after issuance
      (fake-clock test)
- [ ] `FetchTokenResponse.remote_url` carries the token in the
      `x-access-token:<token>@` userinfo segment
- [ ] `IssueFetchToken` from a non-session-member returns 403
- [ ] `MarkSessionShipped` on a `finalizing` session transitions to
      `ended` with `end_reason = "shipped"` and `ended_at` set
- [ ] `MarkSessionShipped` is idempotent when already
      `ended` + `shipped`
- [ ] `MarkSessionShipped` returns 409 `session.not_finalizing` when
      called on `active`
- [ ] `MarkSessionShipped` returns 409 `session.already_ended` when
      called on `ended` with a different `end_reason`
- [ ] `MarkSessionShipped` releases any held finalize lock
      (`released_at` set on the row, sessions pointer cleared)
- [ ] `MarkSessionShipped` emits `session.ended` event with
      `reason: "shipped"` and optional `final_branch_name`
- [ ] The OpenAPI `SessionEndedPayload.reason` enum lists
      `shipped` as an accepted value
- [ ] Migration 00011 widens the SQLite + Postgres CHECK
      constraint on `archived_sessions.end_reason` to accept
      `shipped`; `make generate-db` succeeds; existing tests still pass

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
