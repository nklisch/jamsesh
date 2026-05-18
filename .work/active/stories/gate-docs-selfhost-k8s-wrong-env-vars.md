---
id: gate-docs-selfhost-k8s-wrong-env-vars
kind: story
stage: implementing
tags: [documentation, infra]
parent: null
depends_on: []
release_binding: v0.1.0
gate_origin: docs
created: 2026-05-18
updated: 2026-05-18
---

# SELF_HOST.md §14 clustered-k8s YAML uses non-existent env vars

## Drift category
foundation-doc-assertion

## Location
- Doc: `docs/SELF_HOST.md:1139-1153`
- Code: `internal/portal/config/config.go` has no
  `JAMSESH_GITHUB_CLIENT_ID` or `JAMSESH_SESSION_SECRET` references;
  canonical names are `JAMSESH_OAUTH_GITHUB_CLIENT_ID` / `_SECRET` (see
  reference table at SELF_HOST.md:112-114)

## Current doc text
```yaml
- name: JAMSESH_GITHUB_CLIENT_ID
- name: JAMSESH_GITHUB_CLIENT_SECRET
- name: JAMSESH_SESSION_SECRET
```

## Reality
Operators copying §14 verbatim will boot a portal without OAuth wired
(the code reads `JAMSESH_OAUTH_GITHUB_*`, not `JAMSESH_GITHUB_*`);
`JAMSESH_SESSION_SECRET` is not read anywhere in the codebase.

## Required edit
Replace `JAMSESH_GITHUB_CLIENT_ID` and `JAMSESH_GITHUB_CLIENT_SECRET`
with `JAMSESH_OAUTH_GITHUB_CLIENT_ID` and
`JAMSESH_OAUTH_GITHUB_CLIENT_SECRET_FILE` (matching the §13 k8s recipe
at SELF_HOST.md:899-902, which uses the `_FILE` form). Delete the
`JAMSESH_SESSION_SECRET` block entirely or replace it with an actual
config knob if a session secret is required.
