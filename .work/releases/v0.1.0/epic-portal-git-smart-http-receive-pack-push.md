---
id: epic-portal-git-smart-http-receive-pack-push
kind: story
stage: done
tags: [portal, security]
parent: epic-portal-git-smart-http
depends_on: [epic-portal-git-smart-http-auth-and-routing]
release_binding: v0.1.0
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

## Implementation notes

### Files delivered

- `internal/portal/githttp/pktline.go` — pkt-line reader (`readCommandList`) and report-status writer (`writeReportStatusRejection`). Wire-protocol zero-SHA (40 zeros) is normalized to empty OldSHA so prereceive treats new-ref creation correctly.
- `internal/portal/githttp/receive_pack.go` — `receivePack` handler and `buildValidationRepo` helper. `layeredStorer` embeds `*memory.Storage` and overrides object/ref methods to fall through to the disk storer.
- `internal/portal/githttp/stream.go` — shared `streamWithFlush` helper (moved here to avoid redeclaration conflict with sibling agent writing upload_pack.go).
- `internal/portal/githttp/handler.go` — `stubReceivePack` removed; `Mount` now registers `h.receivePack`.
- `internal/portal/githttp/pktline_test.go` — unit tests for pkt-line parsing and report-status formatting.
- `internal/portal/githttp/receive_pack_test.go` — integration tests driving real `git push` against httptest.Server.

### Key design decisions

**Quarantine bypass via in-memory pack parsing**: `git-receive-pack` quarantines incoming objects in a temp dir that go-git's `dotgit` storage cannot see. Pre-receive validation parses the pushed pack body with `packfile.NewParserWithStorage` into `memory.NewStorage()`, then wraps it in a `layeredStorer` that falls through to the disk repo for existing objects. This makes the prereceive validator see the full object graph without any subprocess cooperation.

**layeredStorer**: Embeds `*memory.Storage` (satisfies `storage.Storer` for Config/Index/Shallow/Module methods), overrides `EncodedObject`, `HasEncodedObject`, `EncodedObjectSize`, `IterEncodedObjects` to try memory first then disk, and overrides all reference methods to always read from disk (refs are on disk; the push hasn't landed yet).

**Post-receive uses a fresh disk repo**: After `git receive-pack --stateless-rpc` exits 0, we re-open the bare repo from disk. By that point the pack has been applied and go-git can walk the new commits for event emission.

**OldSHA zero normalization**: The git wire protocol encodes "new ref creation" as 40 ASCII zeros for OldSHA. `readCommandList` maps this to empty string so `prereceive.ValidateRef` skips the force-push check (which would otherwise fail trying to resolve the zero hash).

### Test coverage

- `TestReadCommandList_*` — pkt-line unit tests
- `TestWriteReportStatusRejection_*` — report-status format unit tests
- `TestReceivePack_WrongContentType` — 400 for wrong Content-Type
- `TestReceivePack_SuccessfulPush` — real `git push` succeeds; ref updated; commit.arrived event emitted
- `TestReceivePack_RejectedMissingTrailers` — missing trailers → rejected; no ref created; no events
- `TestReceivePack_MultipleCommits` — 3 commits in one push → 3 events emitted in order
- `TestReceivePack_PackSizeLimitExceeded` — oversized body → 413

## Review (2026-05-16)

**Verdict**: Approve

**Notes**: layeredStorer pattern is the right answer to the quarantine problem — clean abstraction. OldSHA zero normalization for new-ref creation handled correctly. Full git push end-to-end test covers happy path + rejection + multi-commit ordering + size limit.
