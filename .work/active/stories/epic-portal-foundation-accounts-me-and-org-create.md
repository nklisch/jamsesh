---
id: epic-portal-foundation-accounts-me-and-org-create
kind: story
stage: implementing
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
