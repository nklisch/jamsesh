---
id: story-orgs-handler-missing-deperr-wrapdb-on-authfail-err
kind: story
stage: done
tags: [portal, security]
parent: null
depends_on: []
release_binding: null
gate_origin: refactor-design
created: 2026-05-23
updated: 2026-05-23
---

# accounts/orgs.go: wrap authfail-Err returns with deperr.WrapDBIfTransient

## Brief

`GetOrg` and `PatchOrg` in `internal/portal/accounts/orgs.go` deviate
from the documented `authfail-three-branch-guard` +
`deperr-translate-pipeline` patterns: when `handlerauth.RequireOrgMember`
returns a non-nil `fail.Err`, both handlers wrap it with a bare
`fmt.Errorf` rather than `deperr.WrapDBIfTransient`. Every other
handler in the same package follows the deperr wrap.

Effect: a transient DB-unavailability surfacing through the auth lookup
returns a generic 500 from this handler instead of the canonical
`dep.db_unavailable` envelope with `Retry-After`, so clients lose the
typed retry signal for these two endpoints.

Surfaced by a discovery-mode `/agile-workflow:refactor-design` scan.
**Behavior-changing — this is a bug fix, not a pure refactor.** Not
tagged `[refactor]` so the design pass routes through feature-design
classification if it grows.

## Current state

```go
// GetOrg, line 33-35
if fail.Err != nil {
    return nil, fmt.Errorf("accounts: get org: %w", fail.Err)
}

// PatchOrg, line 72-74
if fail.Err != nil {
    return nil, fmt.Errorf("accounts: patch org: %w", fail.Err)
}
```

## Target state

```go
if fail.Err != nil {
    return nil, deperr.WrapDBIfTransient(fmt.Errorf("accounts: get org: %w", fail.Err))
}
// (and matching for PatchOrg)
```

Same wording, same shape — only the `deperr.WrapDBIfTransient` wrap
added so `httperr.WriteFromError` can classify and emit the typed
envelope.

## Acceptance criteria

- `GetOrg` and `PatchOrg` both wrap `fail.Err` returns with
  `deperr.WrapDBIfTransient`.
- A new (or existing-extended) handler test exercises the
  authfail-Err transient path and asserts the typed
  `dep.db_unavailable` envelope is emitted.
- `go test ./internal/portal/accounts/...` clean.
- The change is consistent with sibling handlers in the same file
  (`CreateOrgInvite`, `AcceptOrgInvite`).

## Verification of scope

Grep `internal/portal/` for handlers that match the pattern
`if fail.Err != nil { return nil, fmt.Errorf(...) }` without an
adjacent `deperr.Wrap*`. If more sites surface during implementation,
extend this story rather than spinning new ones — the fix is
mechanical.

## Notes

This is the only handler-level deviation from the deperr pipeline
found in the discovery scan. The fix is surgical (~2 lines in one
file plus a test).

## Implementation notes

**Wrap sites fixed (2):**
- `GetOrg` line 34: `fmt.Errorf("accounts: get org: %w", fail.Err)` → wrapped with `deperr.WrapDBIfTransient`.
- `PatchOrg` line 73: `fmt.Errorf("accounts: patch org: %w", fail.Err)` → wrapped with `deperr.WrapDBIfTransient`.

**Tests added (`internal/portal/accounts/orgs_test.go`):**
- `TestGetOrg_DBTransientOnAuthLookup_Returns503DepDBUnavailable` — injects a `failingGetOrgMemberStore` that returns `fmt.Errorf("conn refused")` from `GetOrgMember`, drives `GET /api/orgs/{orgID}`, and asserts 503 + `Retry-After: 2` + `{"error":"dep.db_unavailable"}`.
- `TestPatchOrg_DBTransientOnAuthLookup_Returns503DepDBUnavailable` — same injection, drives `PATCH /api/orgs/{orgID}`, same assertions.
- The `failingGetOrgMemberStore` wraps the real store so token validation (via `GetAccount`) passes while only `GetOrgMember` fails, matching the exact transient-error path through `RequireOrgMember`.

**Cross-package scope scan:** All other `if fail.Err != nil` sites in `internal/portal/` (comments/handlers.go ×3, sessions/handler.go ×3, playground/handler.go ×1) already wrap with `deperr.WrapDBIfTransient`. No additional fixes required.

**Verification:** `go build ./...` and `go test ./...` both clean.

## Review (2026-05-23)

**Verdict**: Approve

**Blockers**: none
**Important**: none
**Nits**:
- `assertDepDBUnavailable` is local to `orgs_test.go`; if more dep-failure tests land in this package it could move to a shared helper file. Future-proofing only.

**Notes**: Two-line wrap matches sibling handlers exactly. `WrapDBIfTransient` preserves nil and non-transient sentinels (`ErrNotFound`/`ErrUniqueViolation`) so there's no over-classification risk. Test injection wraps the real store and overrides only `GetOrgMember` so token validation still flows through real auth — the request reaches the handler where the bug lived. Agent's cross-package scope scan confirmed every other `fail.Err != nil` site in `internal/portal/` was already correctly wrapped. `go test ./internal/portal/accounts/...` and full `go test ./...` clean.
