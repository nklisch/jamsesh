---
id: epic-portal-git-smart-http-upload-pack-fetch
kind: story
stage: implementing
tags: [portal]
parent: epic-portal-git-smart-http
depends_on: [epic-portal-git-smart-http-auth-and-routing]
release_binding: null
gate_origin: null
created: 2026-05-16
updated: 2026-05-16
---

# Smart-HTTP — info/refs + upload-pack (Fetch)

## Scope

Implement the fetch side: info/refs advertisement and upload-pack streaming subprocess.

## Units delivered

- `internal/portal/githttp/info_refs.go` — info/refs handler
- `internal/portal/githttp/upload_pack.go` — upload-pack handler
- Tests

## Acceptance Criteria

- [ ] `GET .git/info/refs?service=git-upload-pack` returns the smart-HTTP capability advertisement with correct Content-Type
- [ ] `POST .git/git-upload-pack` runs the subprocess and pipes stdout to client with Content-Type `application/x-git-upload-pack-result`
- [ ] A real `git clone http://127.0.0.1:<port>/git/<orgID>/<sessionID>.git <dir>` succeeds against an httptest server hosting a synthetic bare repo
- [ ] Cache-Control: no-cache on both routes
- [ ] No memory blow-up on large fetches (streaming via http.Flusher)

## Notes

- Subprocess: `git upload-pack --stateless-rpc <repoPath>`. For info/refs: `git upload-pack --stateless-rpc --advertise-refs <repoPath>` (prefixed with smart-HTTP service line).
- The smart-HTTP skill carries the verified pkt-line + streaming pattern.
