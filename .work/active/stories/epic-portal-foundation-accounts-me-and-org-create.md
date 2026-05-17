---
id: epic-portal-foundation-accounts-me-and-org-create
kind: story
stage: done
tags: [portal, security]
parent: epic-portal-foundation-accounts
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-16
updated: 2026-05-16
---

# Accounts — /api/me + POST /api/orgs + Role Middleware

## Scope

Stand up the role-checking middleware, the `GET /api/me` endpoint
(used by the portal SPA on every page load), and the `POST /api/orgs`
manual-org-creation endpoint.

## Units delivered

- `internal/portal/auth/middleware.go` — `RequireOrgRole(store, roles ...string) func(http.Handler) http.Handler`
- `internal/portal/accounts/handlers.go` — `Handler` with `Me` + `CreateOrg` methods matching the strict-server interface
- `docs/openapi.yaml` (edit) — `GET /api/me` and `POST /api/orgs`; schemas `MeResponse`, `OrgRef`, `CreateOrgBody`, `MeOrgMembership`
- Regen `internal/api/openapi/server.gen.go` + `frontend/src/lib/api/types.gen.ts`
- `cmd/portal/main.go` (edit) — register the new authenticated routes, extend `combinedHandler` to embed the accounts handler
- Tests

## Acceptance Criteria

- [ ] `GET /api/me` returns `{id, email, display_name, orgs: [{id, name, slug, role}]}` for the authenticated account
- [ ] `POST /api/orgs` accepts `{name}` and returns the created org; the authenticated account becomes a `creator` member
- [ ] `RequireOrgRole` middleware rejects non-members with 403 (`auth.insufficient_permission`) and wrong-role with 403; allows the right role through
- [ ] `make generate && git diff --exit-code` green
- [ ] All tests green

## Notes

- Reuse the slug-generation helper from `internal/portal/auth/provision.go` if it's already callable as a public function; otherwise factor it out into a shared util.
- The `/api/me` endpoint is consumed by the SPA's `auth.loadCurrentUser` (currently a no-op). After this story, the SPA can populate the avatar + breadcrumb chips.
- Routes go in the authenticated group (BearerMiddleware applied).

## Implementation notes

### Files created
- `internal/portal/auth/slug.go` — `SlugFromName`, `CreateOrgWithSlug`, `randomSuffix`, `alphanumChars` extracted from `provision.go` into a shared helper (same `auth` package)
- `internal/portal/auth/middleware.go` — `RequireOrgRole(store, roles...)` middleware; injects resolved `*store.OrgMember` via `OrgMemberFromContext` for downstream handler convenience
- `internal/portal/accounts/handlers.go` — `Handler` with `GetMe` and `CreateOrg` satisfying `openapi.StrictServerInterface`
- `internal/portal/auth/middleware_test.go` — 5 middleware tests: not-member→403, wrong-role→403, correct-role→200 (×2), context injection
- `internal/portal/accounts/handlers_test.go` — 6 handler tests: GetMe happy path, multiple orgs, no-auth 401; CreateOrg happy path, slug-collision suffix, no-auth 401

### Files modified
- `docs/openapi.yaml` — added `MeOrgMembership`, `MeResponse`, `OrgRef`, `CreateOrgBody` schemas; `GET /api/me` and `POST /api/orgs` paths
- `internal/portal/auth/provision.go` — refactored to call `CreateOrgWithSlug` (shared), removed duplicate `createOrgWithSlug`, `slugFromEmail`, `randomSuffix`
- `internal/api/openapi/server.gen.go` — regenerated (added `GetMe`, `CreateOrg` to `StrictServerInterface`)
- `frontend/src/lib/api/types.gen.ts` — regenerated with new account/org types
- `cmd/portal/main.go` — added `accounts.Handler` to `combinedHandler`, delegate methods, authenticated routes for `/me` and `/orgs`
- `internal/portal/auth/magic_link_test.go`, `oauth_test.go`, `internal/portal/tokens/handlers_test.go` — added `GetMe` and `CreateOrg` stub methods to satisfy updated interface

### Design decisions
- `RequireOrgRole` errors are always `auth.insufficient_permission` 403; no distinction between "not a member" and "wrong role" (prevents membership enumeration)
- `OrgMemberFromContext` is provided as a convenience for future org-scoped handlers that want to skip a redundant store lookup
- `accounts.Handler` uses named field (`AccountsHandler`) in `combinedHandler` to avoid Go embedded-field name collision with `tokens.Handler`; delegate methods are 2 lines each

## Review (2026-05-16)

**Verdict**: Approve

**Blockers**: none
**Important**: none
**Nits**: none

**Notes**: RequireOrgRole middleware injecting OrgMember into context is a thoughtful detail — saves a redundant lookup in handlers. Same 403 for non-member and wrong-role correctly prevents membership enumeration. Slug helper factoring into auth/slug.go is clean refactor. /api/me now feeds the SPA's loadCurrentUser.
