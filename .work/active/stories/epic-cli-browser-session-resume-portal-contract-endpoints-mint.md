---
id: epic-cli-browser-session-resume-portal-contract-endpoints-mint
kind: story
stage: done
tags: [portal, security]
parent: epic-cli-browser-session-resume-portal-contract
depends_on: [epic-cli-browser-session-resume-portal-contract-token-store]
release_binding: null
gate_origin: null
created: 2026-05-30
updated: 2026-05-30
---

# Resume contract (openapi) + mint endpoint

Implements **Unit 2** of `epic-cli-browser-session-resume-portal-contract`. See
the feature body.

## Scope

- `docs/openapi.yaml`: add BOTH operations (`POST /api/session-resumes` and
  `POST /api/session-resumes/exchange`) + their request/response schemas, so the
  generated `StrictServerInterface` types exist for the exchange story too.
  Regenerate Go (and TS if the SPA story needs the types). (This is the
  foundation-doc roll-forward for openapi at implementation time.)
- New handler package `internal/portal/sessionresume/` implementing the mint op
  via the `StrictServerInterface` (chi-server) pattern.
- Mint: `AccountFromContext` + `checkSessionMembership(org_id, session_id,
  account)` (mirror `internal/portal/finalize/fetch_token.go`); 401/403/404
  matrix. Generate a random token; store `sha256(token)` + binding via Unit 1;
  return ONLY `{ resume_url, expires_in: 60, session_id }` where `resume_url` is
  the canonical route with the `rt` fragment (single source of truth — see
  feature body Design decisions). Do NOT return a standalone `resume_token`
  field (the raw token appears once, in the fragment).
- Wire `POST /api/session-resumes` under the **bearer-middleware** route group;
  add a rate limit via `internal/portal/ratelimit`.

## Acceptance criteria

- [ ] No bearer → 401; non-org-member → 403; non-session-member → 403; unknown
      session → 404 (mirror fetch-token).
- [ ] Success stores a HASHED token bound to account+session; raw token never
      logged.
- [ ] Response `resume_url` carries the `rt` fragment + the canonical path
      (playground vs durable per session kind); no standalone token field.
- [ ] Mounted under bearer middleware (not the public group) and rate-limited.
- [ ] `expires_in` reflects the 60s TTL.
- [ ] `go build ./...`, `go vet`, sqlc/oapi generate clean; handler tests pass.

## Implementation notes

- Added `POST /api/session-resumes` to `docs/openapi.yaml` with `operationId: createSessionResume`.
  Added `SessionResumeRequest` and `SessionResumeResponse` schemas. Exchange op deliberately omitted
  from this story (per guardrail) to keep the StrictServerInterface implementation complete.
- Regenerated `internal/api/openapi/server.gen.go` via `make generate-api-go`.
- New package `internal/portal/sessionresume/` with three files:
  - `handler.go`: `Handler` struct + `New`/`NewWithClock` constructors, `sessionResumeStore` narrow
    interface composed from `store.SessionStore + SessionMemberStore + OrgMemberStore + ResumeTokenStore`.
  - `membership.go`: package-private `checkSessionMembership` replicating finalize's helper (cannot
    import finalize's private function).
  - `mint.go`: `CreateSessionResume` implementation — 401/403/404 guard, crypto/rand token generation,
    SHA-256 hash stored, raw token embedded ONLY in `resume_url` fragment (`#rt=<token>`). Canonical
    paths: playground → `/playground/s/{sessionID}/resume`; durable → `/orgs/{orgID}/sessions/{sessionID}/resume`.
    `playgroundOrgID` mirrored as local const per `reserved-org-id-local-const-mirror` pattern.
- `cmd/portal/main.go`: added `SessionResumeHandler` field to `combinedHandler`, `CreateSessionResume`
  delegation method, `sessionresume.New(dbStore, tokenSvc, cfg.PortalURL)` construction, `sessionResumeRL`
  rate limiter (10/min), route `POST /api/session-resumes` in the bearer-authenticated group.
- All existing strict-server partial shims in 10 test files updated with `CreateSessionResume` panic stub.
- `internal/portal/sessionresume/mint_test.go`: 7 tests covering 401/403/404/success-durable/
  success-playground/no-standalone-token/hash-stored-not-raw. All pass.
- `go build ./...`, `go vet ./cmd/... ./internal/portal/...`, finalize tests all clean.
