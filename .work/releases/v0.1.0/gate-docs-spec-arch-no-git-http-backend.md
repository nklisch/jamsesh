---
id: gate-docs-spec-arch-no-git-http-backend
kind: story
stage: done
tags: [documentation]
parent: null
depends_on: []
release_binding: v0.1.0
gate_origin: docs
created: 2026-05-18
updated: 2026-05-18
---

# SPEC.md and ARCHITECTURE.md still claim portal can serve git via `git http-backend` CGI

## Drift category
foundation-doc-assertion

## Location
- Doc: `docs/SPEC.md:21-22` and `docs/ARCHITECTURE.md:63-67`
- Code: `internal/portal/githttp/handler.go:32-43`,
  `internal/portal/githttp/info_refs.go`,
  `internal/portal/githttp/upload_pack.go`. Skill
  `.claude/skills/git-smart-http/SKILL.md:9-12` explicitly says "NOT via
  `git http-backend` CGI".

## Current doc text
SPEC.md:
> System `git` binary for smart-HTTP serving (via subprocess or `git http-backend` CGI).

ARCHITECTURE.md:
> Wraps the canonical `git http-backend` CGI (or invokes
> `git-upload-pack` / `git-receive-pack` as subprocesses) with
> Go-implemented HTTP Basic auth …

## Reality
The portal directly spawns `git-upload-pack` and `git-receive-pack` with
`--stateless-rpc`. `git http-backend` is never invoked. Pre-receive runs
in-process in Go before subprocess spawn.

## Required edit
Replace both passages with a single, unambiguous statement: smart-HTTP
is served by spawning `git-upload-pack` and `git-receive-pack` as
subprocesses with `--stateless-rpc`; pre-receive validates in-process
before spawn. Drop the `git http-backend` mention.

## Implementation notes

- `docs/SPEC.md` line 20-21: replaced "via subprocess or `git http-backend` CGI" with "`git-upload-pack` and `git-receive-pack` spawned as subprocesses with `--stateless-rpc`".
- `docs/ARCHITECTURE.md` lines 64-71: replaced the "Wraps the canonical `git http-backend` CGI..." passage with text describing direct subprocess spawning with `--stateless-rpc`, in-process pre-receive validation, and chi-router auth enforcement — consistent with `internal/portal/githttp/` code.
- Final grep confirms zero remaining `http-backend` or `CGI` references in both files.

## Review (2026-05-18)

**Verdict**: Approve

**Blockers**: none
**Important**: none
**Nits**: none

**Notes**: Mechanical change matching the gate finding spec. Implementation notes accurately describe what was changed. Global `go build ./...` and `go test ./internal/portal/...` pass after the wave landed.
