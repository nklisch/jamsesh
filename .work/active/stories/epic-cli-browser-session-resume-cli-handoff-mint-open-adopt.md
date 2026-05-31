---
id: epic-cli-browser-session-resume-cli-handoff-mint-open-adopt
kind: story
stage: implementing
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
