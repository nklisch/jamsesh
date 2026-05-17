---
id: epic-distribution-self-host-docs-quickstart-ci
kind: story
stage: implementing
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
