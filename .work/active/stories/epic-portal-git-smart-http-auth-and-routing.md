---
id: epic-portal-git-smart-http-auth-and-routing
kind: story
stage: done
tags: [portal, security]
parent: epic-portal-git-smart-http
depends_on: []
release_binding: v0.1.0
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

## Implementation notes

**Files delivered:**

- `internal/portal/githttp/handler.go` — `Handler` struct + `Mount(r chi.Router)` + stub handlers (501)
- `internal/portal/githttp/auth.go` — `basicAuth`, `requireSessionMember`, `checkArchived` middlewares + `AccountFromContext` helper
- `internal/portal/githttp/handler_test.go` — 6 tests covering all AC cases
- `cmd/portal/main.go` — wired `storage.New`, `events.New`, `prereceive.Validator`, `postreceive.Emitter`, and `githttp.Handler`; set `router.Deps.MountGit = gitHandler.Mount`

**Key decisions:**

- `basicAuth` uses `r.BasicAuth()` (stdlib) then `tokens.BasicAuthValidator(svc)`. Uses exact same `accountCtxKey{}` pattern as `tokens.BearerMiddleware` but in a separate type so the two context keys don't collide. `AccountFromContext` exported for downstream handlers.
- `requireSessionMember` returns 401 (not 403/404) on missing membership to avoid session-existence disclosure. Any store error other than ErrNotFound → 500.
- `checkArchived` checks `storage.LookupArchived`; ErrNotFound → pass through; found → 410 + JSON-encoded `ArchivedStub`. Runs after auth+membership so non-members can't probe archived status either.
- Route registration: `Mount` uses `/{orgID}/{sessionID}.git` relative paths; the router already mounts at `/git` so the final paths are `/git/{orgID}/{sessionID}.git/...`.
- `cmd/portal/main.go` constructs `gitHandler` between accountsHandler and the router build. Uses `cfg.Git.MaxPackBytes` for the validator limit.

## Review (2026-05-16)

**Verdict**: Approve

**Notes**: Three middlewares clean. Same-401 for no-auth/invalid-token/non-member prevents session-existence disclosure. Separate accountCtxKey type avoids collision with tokens middleware.
