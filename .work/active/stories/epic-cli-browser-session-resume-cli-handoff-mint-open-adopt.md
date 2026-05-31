---
id: epic-cli-browser-session-resume-cli-handoff-mint-open-adopt
kind: story
stage: done
tags: [plugin]
parent: epic-cli-browser-session-resume-cli-handoff
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-30
updated: 2026-05-30
---

# Token-safe opener + mint helper + `--open` identity adoption

Implements **Unit 1** of `epic-cli-browser-session-resume-cli-handoff`. See the
feature body (Architectural choice, Unit 1, Risks, Other agent review).

## Scope

- `cmd/jamsesh/internal/osopen/osopen.go`: add `OpenSilent(rawURL string) error`
  — launches the browser but NEVER writes the URL anywhere (no print-on-failure,
  no unsupported-OS print). Returns an error on failure; the caller decides a
  token-free message. (For URLs carrying a secret in the fragment.) + test.
- `cmd/jamsesh/sessioncmd/resume.go` (new): `mintAndOpenResume(ctx, pc, orgID,
  sessionID)` — POST `/api/session-resumes` via `portalclient.PostJSON[openapi.
  SessionResumeResponse]`; validate `resp.SessionId == sessionID` AND
  `resp.ResumeUrl != ""` BEFORE opening (else error, open nothing); print a
  token-free line ("Opening your session in the browser (resume link expires in
  60s)…"); `osopen.OpenSilent(resp.ResumeUrl)`.
- Wire `--open` adoption at the existing `cmd.Bool("open")` sites in `new.go`
  (`newAction` durable + `newPlaygroundAction` playground) and `join.go`
  (`joinAction`): build `pc` with `SessionID` set (so the per-session bearer —
  anon for playground, OAuth for durable — authenticates the mint; do NOT use
  `buildPortalClient()` for playground), call `mintAndOpenResume`; on error,
  print a WARNING and fall back to the prior token-free
  `openInBrowser(sessionViewURL/playgroundJoinURL)`.

## Acceptance criteria

- [ ] `--open` (durable / playground / join) mints then opens the exact
      `resp.ResumeUrl`; playground uses the anonymous per-session bearer (NOT OAuth).
- [ ] No `#rt=` (or the resume_url) EVER reaches stdout/stderr — on success, on
      mint failure, and on browser-open failure (`OpenSilent` is silent).
- [ ] Empty `ResumeUrl` / `SessionId` mismatch → error before opening anything.
- [ ] `--open` mint failure → warning + token-free fallback open (old behavior).
- [ ] `go build ./...`, `go vet`, `go test ./cmd/jamsesh/...` pass.

## Notes

Generated field names are `OrgId`/`SessionId`/`ResumeUrl`. Build with
`GOTMPDIR=/home/nathan/.cache/jamsesh-gotmp` if `/tmp` (tmpfs) is full.

## Implementation notes

### Files changed
- `cmd/jamsesh/internal/osopen/osopen.go`: added `OpenSilent(rawURL string) error`
  — reuses `platformArgv`/`execCommand` seam but returns errors instead of printing.
  Unsupported OS → error; exec.Start failure → wrapped error. URL never emitted.
- `cmd/jamsesh/internal/osopen/osopen_test.go`: added `TestOpenSilent_ErrorOnStartFailure`
  and `TestOpenSilent_SuccessNoOutput` asserting no URL leakage even in error paths.
- `cmd/jamsesh/sessioncmd/resume.go` (new): package-level `openSilent` seam +
  `mintAndOpenResume(ctx, pc, orgID, sessionID)` helper. Validates
  `resp.SessionId == sessionID` and `resp.ResumeUrl != ""` before opening anything.
  Prints one token-free line to stdout; never prints the URL.
- `cmd/jamsesh/sessioncmd/new.go`: wired `--open` adoption at both `newAction`
  (durable) and `newPlaygroundAction` (playground) with mint-or-fallback pattern.
  Playground path builds `mintPC` with `SessionID` set but without `buildPortalClient()`
  so the anon per-session bearer (not OAuth) is used.
- `cmd/jamsesh/sessioncmd/join.go`: wired `--open` adoption in `joinAction` with
  mint-or-fallback; `WireRefresh` applied to the per-session mint client.
- `cmd/jamsesh/sessioncmd/new_test.go`: added `stubOpenSilent`, `captureStdoutAndStderr`,
  `assertNoTokenLeak`, `assertNoHashRT`, and 5 new tests covering durable/playground
  mint-success, mint-failure fallback, and openSilent-failure — all with SECURITY ACs.
- `cmd/jamsesh/sessioncmd/join_test.go`: added `buildJoinMuxWithResume`, and 2 new
  tests covering join mint-success and mint-failure fallback with SECURITY ACs.

### Token-leak prevention strategy
- `OpenSilent` never prints the URL on any path (failure → returned error, not print).
- `mintAndOpenResume` prints only a static token-free message; the `resp.ResumeUrl`
  is passed directly to `openSilent`, never interpolated into any string that goes
  to stdout/stderr.
- Warning messages on fallback use `%v` on the error (which doesn't contain the URL)
  not `%v` on the resume URL.
- Tests capture both stdout AND stderr and assert neither contains `#rt=` or the
  full resume URL — on success, on mint failure, and on browser-open failure.

### Acceptance criteria status
- [x] `--open` (durable / playground / join) mints then opens the exact `resp.ResumeUrl`
- [x] playground uses the anonymous per-session bearer (NOT OAuth)
- [x] No `#rt=` ever reaches stdout/stderr on any path
- [x] Empty `ResumeUrl` / `SessionId` mismatch → error before opening
- [x] `--open` mint failure → warning + token-free fallback open
- [x] `go build ./...`, `go vet`, `go test ./cmd/jamsesh/...` pass
