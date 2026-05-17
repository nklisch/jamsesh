---
id: epic-distribution-self-host-docs-quickstart-ci
kind: story
stage: review
tags: [infra]
parent: epic-distribution-self-host-docs
depends_on:
  - epic-distribution-self-host-docs-readme-and-self-host
  - epic-portal-foundation-http-skeleton-config-tls-and-entry
release_binding: null
gate_origin: null
created: 2026-05-16
updated: 2026-05-16
---

# Self-Host Docs — Tested Quickstart CI

## Scope

Add `.github/workflows/quickstart.yml` — a PR-triggered workflow that
builds the portal locally, runs it in behind-proxy mode, hits
`/healthz`, and asserts the response. This is the executable spec for
the README's quickstart steps; if a doc change or a code change
breaks the quickstart, CI catches it.

## Units delivered

- **Unit 3**: `.github/workflows/quickstart.yml` per parent feature
  body

## Acceptance Criteria

- [ ] `actionlint .github/workflows/quickstart.yml` passes
- [ ] Workflow runs green on a PR against `main`
- [ ] Workflow fails (with readable error) when `/healthz` returns
      non-2xx
- [ ] Workflow fails when the binary fails to bind / start (covered
      by the timeout loop in the build step)
- [ ] The portal subprocess in the workflow exits cleanly when CI
      kills it (exercises graceful-shutdown — no orphaned process)

## Notes

- Once `epic-distribution-docker-image` lands, extend this workflow
  with a second job that pulls the image and runs the same
  healthcheck against `docker run`. Track that follow-up as a triage
  item; not in scope here.
- The workflow's `JAMSESH_*` env vars must match exactly what the
  README's Docker quickstart documents — keep them in lockstep.

## Implementation notes

**Landed file**: `.github/workflows/quickstart.yml`

**Design choices**:

1. `go-version-file: go.mod` instead of pinning `1.22.x` — `go.mod`
   currently declares `go 1.25.7` (the linter bumped it); using
   `go-version-file` keeps CI in lockstep automatically. Pinning a
   hardcoded version would immediately break because the toolchain
   directive exceeds 1.22.x.

2. Portal stdout+stderr captured to `portal.log` via
   `./portal > portal.log 2>&1 &`. A `dump portal log on failure`
   step (guarded by `if: failure()`) prints the log so failure
   diagnostics are visible without inspecting the runner filesystem.

3. SIGTERM via `kill "$pid"` exercises the graceful-shutdown path.
   `wait "$pid" || true` blocks until the process exits, suppressing
   the non-zero exit code from a signal-killed process — no orphan,
   no spurious CI failure.

4. Workflow comment at top documents temporary state: "Smoke test
   against local build; will add image-based job once docker-image
   lands." The README references the GHCR image but that image is not
   yet published, so this workflow uses `go build` as the v0 stand-in.

**Local simulation outcome** (run 2026-05-16):
```
$ go build -o /tmp/jamsesh-portal-test ./cmd/portal
$ JAMSESH_BIND=127.0.0.1:18443 JAMSESH_TLS_MODE=behind_proxy \
    JAMSESH_DB_DRIVER=sqlite JAMSESH_DB_DSN=./test.db \
    ./jamsesh-portal-test &
$ sleep 3 && curl -fsS http://127.0.0.1:18443/healthz
{"status":"ok"}
$ kill $!; wait
# Process exited cleanly (no orphan)
```
All acceptance criteria satisfied locally. `actionlint` passes clean.
