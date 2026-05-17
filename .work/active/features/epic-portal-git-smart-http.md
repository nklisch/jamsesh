---
id: epic-portal-git-smart-http
kind: feature
stage: done
tags: [portal, security]
parent: epic-portal-git
depends_on: [epic-portal-git-storage, epic-portal-git-pre-receive, epic-portal-git-post-receive, epic-portal-foundation-tokens]
release_binding: null
gate_origin: null
created: 2026-05-16
updated: 2026-05-16
---

# Portal Git — Smart-HTTP Handlers

## Brief

The HTTP handler trio that exposes session bare repos to git clients over
smart-HTTP, plus the HTTP Basic auth integration that maps "git push" credentials
to portal tokens. This feature is the assembly point: it composes storage,
pre-receive, post-receive, and the foundation tokens helper into a working
git server.

**Routes** (per `docs/PROTOCOL.md > Git smart-HTTP`):

- `GET /git/<org_id>/<session_id>.git/info/refs?service=...` — capability
  advertisement
- `POST /git/<org_id>/<session_id>.git/git-upload-pack` — fetch
- `POST /git/<org_id>/<session_id>.git/git-receive-pack` — push

**Auth** (HTTP Basic):

- Decode the `Authorization: Basic ...` header. Username is ignored
  (convention: `x-access-token` or anything); password is the user's portal
  OAuth token.
- Hand the token to the foundation tokens feature's validator
  (`epic-portal-foundation-tokens`). On success: `account_id` is known.
- Verify the account is a member of `session_id` (DB lookup via
  data-layer query package).
- On failure: respond `401 Unauthorized` with the
  `WWW-Authenticate: Basic realm="jamsesh"` header so git prompts for
  credentials.

**Subprocess invocation** (locked at epic-design):

- Spawn `git-upload-pack --stateless-rpc` or `git-receive-pack
  --stateless-rpc` as a subprocess, with `GIT_DIR=<bare-repo-path>` in env
- Pipe the request body to the subprocess's stdin
- Stream the subprocess's stdout to the response body with the appropriate
  `Content-Type` (`application/x-git-upload-pack-result` or
  `application/x-git-receive-pack-result`)
- Set `Cache-Control: no-cache` per the smart-HTTP protocol

**Push request flow** (the assembled pre-receive + post-receive
choreography):

1. Authenticate via HTTP Basic (token validator).
2. Verify session membership.
3. Read the pack into a temp area (or stream into a temporary objects
   dir — design pass decides) and parse the proposed ref updates.
4. Call into pre-receive validator. On reject: return the
   git-protocol report with rejection messages; do NOT invoke
   `git-receive-pack`.
5. On accept: invoke `git-receive-pack --stateless-rpc` and pipe
   through.
6. After `git-receive-pack` returns success: call into post-receive
   event emitter with the accepted ref updates.

**Archived-session handling**: before the auth step, look up the session in
the data layer. If the session is archived, the storage feature's archived
stub formatter produces a 410 Gone response — handler returns it without
proceeding to auth or subprocess invocation.

**Streaming discipline**: use `io.Pipe` and `http.Flusher` to stream large
fetch responses without buffering the entire repo in memory. Bound the
write-side temp area for pushes by the 50 MB cap from pre-receive.

Does NOT implement any policy logic (lives in pre-receive). Does NOT
emit events directly (delegated to post-receive). Does NOT cover the
session base-push permit — that's pre-receive's `base` exception path,
this handler just routes the request like any other receive-pack.

## Epic context

- Parent epic: `epic-portal-git`
- Position in epic: assembly point; the only feature in the epic with a
  cross-epic dep (`epic-portal-foundation-tokens`).

## Foundation references

- `docs/PROTOCOL.md` — Git smart-HTTP routes
- `docs/ARCHITECTURE.md` — Git smart-HTTP component; Data flow: a turn >
  steps 5-7 (PostToolUse push → pre-receive → post-receive)
- `docs/SECURITY.md` — Git push authorization (HTTP Basic w/ portal token
  as password); What a single-user-token compromise exposes

## Inherited epic design decisions

- **Smart-HTTP serving mechanism**: subprocess invocation of
  `git-upload-pack` / `git-receive-pack` with stdin/stdout piping. Not
  CGI wrapping of `git-http-backend`.
- **Token reuse across transports**: the foundation tokens feature's
  validator accepts the token from HTTP Basic password the same way the
  Bearer middleware does.
- **Concurrent push handling**: rely on git's native ref locking; no
  portal-level lock layer.

## Decomposition risks

- Streaming gigabyte fetches without memory blowup requires careful
  `io.Pipe` + `http.Flusher` management. Mitigation: feature design
  references the well-trodden Gitea/Forgejo streaming pattern.
- Subprocess error reporting (when `git-receive-pack` returns non-zero
  mid-stream) needs to map cleanly to the git-protocol report-status
  format the client expects. Design pass produces a fault-injection test
  plan.

## Design decisions

- **Package location**: `internal/portal/githttp/`. Public surface: `Handler` struct + `func (h *Handler) Mount(r chi.Router)` to wire `/git/{orgID}/{sessionID}.git/*` routes.
- **Subprocess invocation**: `exec.CommandContext(ctx, "git", "upload-pack", "--stateless-rpc", repoPath)` and `"git", "receive-pack", "--stateless-rpc", repoPath`. The auto-loaded `git-smart-http` skill carries verified patterns; use it.
- **Streaming**: `io.Pipe` for receive-pack stdin, `http.Flusher` after each write to stdout. Bound write-side at `MaxPackBytes` (50 MB default from `config.GitConfig`).
- **Auth gate**: middleware chain on the `/git` route group:
  1. Basic-auth decode → call `tokens.BasicAuthValidator(svc)` → attach `*store.Account` to ctx
  2. Resolve session from URL path → check membership via `store.GetSessionMember`
  3. Archived-session check: if `GetSession` returns archived (or the session is in `archived_sessions`), return 410 Gone via `storage.StubResponse`
- **Pre-receive integration** (receive-pack only):
  1. Stream the pack into a temp file (sized-limited by MaxPackBytes)
  2. Use `go-git` to open the bare repo + walk the proposed refs/objects
  3. Call `prereceive.Validator.Validate(in ValidateInput)` 
  4. On reject: write a git-protocol report-status with rejection messages; return
  5. On accept: spawn `git-receive-pack` with the temp pack on stdin; pipe stdout to response
  6. After receive-pack exits 0: call `postreceive.Emitter.EmitForUpdates(...)` with the accepted updates
- **Story decomposition**: 3 stories.
  1. `auth-and-routing` — Handler skeleton, route mount, Basic-auth, session-membership, archived-session 410. depends_on: []
  2. `upload-pack-fetch` — info/refs + upload-pack subprocess streaming. depends_on: [auth-and-routing]
  3. `receive-pack-push` — receive-pack subprocess with pre/post-receive integration. depends_on: [auth-and-routing]

## Implementation Units

### Unit 1: Handler struct + auth middleware

**File**: `internal/portal/githttp/handler.go`
**Story**: `epic-portal-git-smart-http-auth-and-routing`

```go
package githttp

import (
    "net/http"

    "github.com/go-chi/chi/v5"
    "jamsesh/internal/db/store"
    "jamsesh/internal/portal/postreceive"
    "jamsesh/internal/portal/prereceive"
    "jamsesh/internal/portal/storage"
    "jamsesh/internal/portal/tokens"
)

type Handler struct {
    Store     store.Store
    Tokens    tokens.Service
    Storage   storage.Service
    Validator *prereceive.Validator
    Emitter   *postreceive.Emitter
}

// Mount registers /git/{orgID}/{sessionID}.git/* routes on r.
func (h *Handler) Mount(r chi.Router) {
    r.Route("/git/{orgID}/{sessionID}.git", func(r chi.Router) {
        r.Use(h.basicAuth, h.requireSessionMember, h.checkArchived)
        r.Get("/info/refs", h.infoRefs)
        r.Post("/git-upload-pack", h.uploadPack)
        r.Post("/git-receive-pack", h.receivePack)
    })
}
```

### Unit 2: Auth + session-member middleware

**File**: `internal/portal/githttp/auth.go`

- `basicAuth(next http.Handler) http.Handler` — parses `Authorization: Basic ...`, calls `tokens.BasicAuthValidator(svc)`, attaches `*store.Account` to ctx. On failure: 401 with `WWW-Authenticate: Basic realm="jamsesh"`.
- `requireSessionMember(next http.Handler) http.Handler` — reads orgID + sessionID from URL, looks up session, verifies membership via `GetSessionMember`. On failure: 401 (don't reveal session existence to non-members).
- `checkArchived(next http.Handler) http.Handler` — looks up archived session; if present, return 410 + stub.

### Unit 3: info/refs handler

**File**: `internal/portal/githttp/info_refs.go`
**Story**: `epic-portal-git-smart-http-upload-pack-fetch`

`GET /git/<orgID>/<sessionID>.git/info/refs?service=git-upload-pack|git-receive-pack`:

1. Reject non-allowed services with 400
2. Set Content-Type: `application/x-git-<service>-advertisement`, Cache-Control: no-cache
3. Write the smart-HTTP prelude: `001e# service=git-upload-pack\n0000` (pkt-line)
4. Spawn `git <service> --stateless-rpc --advertise-refs <repoPath>` (the `--advertise-refs` form prints the capability advertisement)
5. Pipe stdout to response

### Unit 4: upload-pack handler

**File**: `internal/portal/githttp/upload_pack.go`

`POST /git/<orgID>/<sessionID>.git/git-upload-pack`:

1. Content-Type: `application/x-git-upload-pack-result`, Cache-Control: no-cache
2. Spawn `git upload-pack --stateless-rpc <repoPath>`
3. Pipe request body to subprocess stdin (via io.Pipe)
4. Pipe subprocess stdout to response (via http.Flusher to keep client alive on long fetches)
5. Wait for subprocess exit; if non-zero, log but don't fail (response is already streaming)

### Unit 5: receive-pack handler

**File**: `internal/portal/githttp/receive_pack.go`
**Story**: `epic-portal-git-smart-http-receive-pack-push`

`POST /git/<orgID>/<sessionID>.git/git-receive-pack`:

1. Content-Type: `application/x-git-receive-pack-result`, Cache-Control: no-cache
2. Read request body, capped at MaxPackBytes, into a temp file
3. Parse the proposed ref updates from the body's command-list section (pkt-line format `<old-sha> <new-sha> <ref-name>`)
4. Open the bare repo via go-git
5. Build `prereceive.ValidateInput`; call `Validator.Validate`
6. If rejected: write the git-protocol report-status with `unpack ok\nng <ref> <reason>\n0000` pkt-lines; return without invoking receive-pack
7. If accepted: spawn `git receive-pack --stateless-rpc <repoPath>`; pipe the temp pack to stdin; pipe stdout to response
8. On receive-pack exit 0: call `postreceive.Emitter.EmitForUpdates(ctx, repo, session, account, updates)` — log emit errors but don't fail (push already succeeded)

### Unit 6: pkt-line parser

**File**: `internal/portal/githttp/pktline.go`

A small helper for parsing receive-pack's command-list (each line is `<4-hex-len><payload>`, terminated by `0000`). Used by Unit 5.

```go
func readCommandList(r io.Reader) (updates []prereceive.RefUpdate, err error)
```

## Implementation Order

1. auth-and-routing — handler skeleton + middleware
2. (parallel) upload-pack-fetch + receive-pack-push

## go.mod additions

- None new (go-git, chi, etc. all present)

## Testing

- Synthetic bare repo + go-git client: full clone via `git clone http://...` against a `httptest.NewServer` running the Handler
- Push via real `git push` (CLI driving the test) — verify pre-receive rejections AND successful pushes
- 401 for missing/invalid auth
- 401 for non-member
- 410 for archived session
- Pack size > MaxPackBytes → 413

## Risks

- **Streaming subtleties**: the smart-http skill is the reference; rely on it.
- **Concurrent pushes to same ref**: git's native ref locking handles correctness; concurrent test verifies.

## Implementation summary

All 3 child stories at done. The complete smart-HTTP server is live: clone via upload-pack, push via receive-pack with full pre/post-receive choreography (validation → subprocess → event emission). Auth via tokens.BasicAuthValidator; archived sessions return 410 stub.

## Review

**Verdict**: Approve. Capability complete.

After this lands, epic-portal-git has all 4 child features done (storage, pre-receive, post-receive, smart-http).
