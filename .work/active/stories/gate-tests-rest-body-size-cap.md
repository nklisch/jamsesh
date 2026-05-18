---
id: gate-tests-rest-body-size-cap
kind: story
stage: done
tags: [testing, security, portal]
parent: null
depends_on: [gate-security-rest-body-size-limits]
release_binding: v0.1.0
gate_origin: tests
created: 2026-05-18
updated: 2026-05-18
---

# REST body-size cap unverified across handlers

## Priority
High

## Spec reference
Item: `gate-security-rest-body-size-limits`
Acceptance criterion: wrap `r.Body` with `http.MaxBytesReader` before
strict-handler decode for all POST handlers; add `maxLength` constraints
in OpenAPI.

## Gap type
missing test for boundary. No test asserts
`POST /api/orgs/{org}/sessions/{sess}/comments` with a 200 MB body
returns 413. The receive-pack 413 boundary test is the only proof.

## Suggested test
```go
// TestREST_BodySizeCap_Returns413
//   table: [CreateComment, RequestMagicLink, PatchFinalizeLock, CreateSession]
//   each with a JSON body > 1 MiB; assert 413 (or 400 oversize) and no DB write
```

## Test location (suggested)
`internal/portal/router/limits_test.go` (new)

## Implementation notes

**File:** `internal/portal/router/body_limits_api_test.go`

**Tests added:**

1. `TestREST_BodySizeCap_Returns413_TableDriven` — table-driven, 5 sub-tests:
   - `RequestMagicLink` → `POST /api/auth/magic-link/request`
   - `CreateSession` → `POST /api/orgs/{orgID}/sessions`
   - `CreateComment` → `POST /api/orgs/{orgID}/sessions/{sessionID}/comments`
   - `CreateOrgInvite` → `POST /api/orgs/{orgID}/invites`
   - `PatchFinalizeLock` → `PATCH /api/orgs/{orgID}/sessions/{sessionID}/finalize/lock/{lockID}`
   - Each fires a 2 MiB POST, asserts 413 + `{"error":"request.body_too_large"}`.

2. `TestREST_BodySizeCap_HonorsConfig` — builds router with `APIBodyLimitBytes=256`,
   fires a 512-byte body, asserts 413 with the correct envelope.

3. `TestREST_BodySizeCap_GitSmartHTTPUnaffected` — stubs a `/git/*` handler,
   fires a 2 MiB POST, asserts the handler is called (200) and no 413.

**Approach:** Uses `openapi.NewStrictHandlerWithOptions(stubStrict{}, nil, ...)` with
`httperr.WriteBadRequest` as `RequestErrorHandlerFunc` — the same wiring as production
(`main.go` line 615). The `stubStrict` satisfies `StrictServerInterface` with no-op
methods; it is never called because the JSON decoder fires `RequestErrorHandlerFunc`
on `*http.MaxBytesError` before reaching the business logic.

**Finding:** No bypass detected. All 5 POST endpoints consistently returned 413
with `request.body_too_large` when the body exceeded the 1 MiB cap.

**All tests pass:** `go test -run TestREST_BodySizeCap -v ./internal/portal/router/...` → PASS

## Review (2026-05-18)

**Verdict**: Approve

**Blockers**: none
**Important**: none
**Nits**: none

**Notes**: Coverage of the body-size cap across 5 real API endpoints (RequestMagicLink, CreateSession, CreateComment, CreateOrgInvite, PatchFinalizeLock) — all 413 with request.body_too_large envelope. Config-knob test exercises the APIBodyLimitBytes plumbing. Smart-HTTP-unaffected test confirms /git/* routes don't get the /api cap. No bypass detected on any of the 5 endpoints. Uses the same strict-handler wiring as production (httperr.WriteBadRequest as RequestErrorHandlerFunc).
