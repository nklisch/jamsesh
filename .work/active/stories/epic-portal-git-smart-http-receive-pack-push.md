---
id: epic-portal-git-smart-http-receive-pack-push
kind: story
stage: implementing
tags: [portal, security]
parent: epic-portal-git-smart-http
depends_on: [epic-portal-git-smart-http-auth-and-routing]
release_binding: null
gate_origin: null
created: 2026-05-16
updated: 2026-05-16
---

# Smart-HTTP — receive-pack (Push) + Pre/Post-Receive Integration

## Scope

Implement the push side: parse the receive-pack command list, call prereceive.Validator, on accept invoke receive-pack subprocess + on success call postreceive.Emitter.

## Units delivered

- `internal/portal/githttp/receive_pack.go` — receive-pack handler
- `internal/portal/githttp/pktline.go` — pkt-line helpers (read 4-hex-len framed records, write report-status)
- Tests

## Acceptance Criteria

- [ ] Request body size limited to `config.GitConfig.MaxPackBytes`; over-size → 413 + push.size_limit rejection
- [ ] Command list parsed correctly (extracted RefUpdate slice)
- [ ] Pre-receive rejections render as git-protocol `unpack ok\nng <ref> <reason>` lines so `git push` displays them inline
- [ ] On clean validation: receive-pack subprocess spawns; request body streamed to stdin; subprocess stdout streamed to client
- [ ] After receive-pack exits 0: postreceive.Emitter.EmitForUpdates called with the updates; events written to log
- [ ] A real `git push http://127.0.0.1:<port>/...` succeeds end-to-end against a test session with valid pre-receive scope

## Notes

- pkt-line format: 4-hex-digit length prefix followed by payload. `0000` is flush. Length 0001-0003 are reserved (not used here).
- The receive-pack body starts with command list, then `0000` flush, then the actual pack data. The command list lines are parsed BEFORE handing the full body to the subprocess.
- For pre-receive rejection responses: write the smart-HTTP `unpack` line + per-ref `ng` lines, then `0000` flush, then DO NOT invoke receive-pack.
- Emitter errors after successful receive-pack: log loudly via slog.Error, but return 200 to the client (push succeeded; missed event is a downstream lag concern, not a push failure).
