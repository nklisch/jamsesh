---
id: epic-portal-git-smart-http
kind: feature
stage: drafting
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

<!-- Feature-design will fill in interfaces, signatures, and implementation
units when /agile-workflow:feature-design runs on this. -->
