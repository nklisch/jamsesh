---
id: epic-portal-git-smart-http-auth-and-routing
kind: story
stage: implementing
tags: [portal, security]
parent: epic-portal-git-smart-http
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-16
updated: 2026-05-16
---

# Smart-HTTP — Auth + Routing Skeleton

## Scope

Build the `Handler` struct + route mount + three middlewares (Basic auth, session membership, archived-session check). After this, the routes return 401/410/403 correctly even though upload-pack and receive-pack are stub handlers.

## Units delivered

- `internal/portal/githttp/handler.go` — Handler struct + Mount
- `internal/portal/githttp/auth.go` — basicAuth + requireSessionMember + checkArchived middlewares
- Stub handlers for info/refs, upload-pack, receive-pack (return 501 Not Implemented for now — next stories replace)
- Tests
- cmd/portal/main.go (edit) — construct githttp.Handler; pass it as `router.Deps.MountGit`

## Acceptance Criteria

- [ ] `GET /git/<orgID>/<sessionID>.git/info/refs` with no auth → 401 + WWW-Authenticate header
- [ ] With invalid Basic password → 401
- [ ] With valid token but non-member → 401
- [ ] With archived session → 410 + JSON stub from storage.StubResponse
- [ ] With valid token + member → reaches the stub handler (501 for now)
- [ ] `go build ./...` clean; `go test ./internal/portal/githttp/...` green

## Notes

- The `auto-loaded` `git-smart-http` skill carries the verified streaming pattern. Consult it for the rest of the chain.
