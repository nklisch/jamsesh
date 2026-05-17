---
id: epic-portal-git-smart-http-upload-pack-fetch
kind: story
stage: done
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

## Implementation notes

- `internal/portal/githttp/info_refs.go` — `infoRefs` handler: validates service param, runs `git <svc> --stateless-rpc --advertise-refs`, writes pkt-line service prefix + subprocess output. Propagates `Git-Protocol` header after regex validation to prevent env injection.
- `internal/portal/githttp/upload_pack.go` — `uploadPack` handler: pipes `r.Body` to subprocess stdin, streams stdout via `streamWithFlush` (defined in `stream.go`, shared with receive-pack). Logs non-zero exit after headers are sent.
- `internal/portal/githttp/handler.go` — Mount updated to use `h.infoRefs` and `h.uploadPack` (stubs for those two removed). `stubReceivePack` was already replaced by the sibling story's `receivePack`.
- `streamWithFlush` lives in `stream.go` (created by sibling receive-pack story); upload-pack and receive-pack both use it.
- Tests: `upload_pack_test.go` adds `TestInfoRefs_UploadPack`, `TestInfoRefs_InvalidService`, and `TestGitClone_EndToEnd`. The clone test builds a synthetic bare repo via shell git, spins up an httptest server with a real sqlite-backed Handler, and runs `git clone` with embedded credentials — verifies the cloned HEAD SHA matches what was seeded.
- `handler_test.go` updated: stub test replaced with `TestValidMember_PassesAuthMiddleware` (checks 401 does not occur); `TestAccountFromContext` assertion loosened to "not 401" since info/refs is real now.

## Review (2026-05-16)

**Verdict**: Approve

**Notes**: Real git clone end-to-end test verifies the streaming chain. Git-Protocol header regex allowlist is a nice security touch. streamWithFlush shared helper in stream.go avoids duplication with receive-pack.
