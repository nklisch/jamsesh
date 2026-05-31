---
id: gate-docs-changelog-v0-5-0-entry-missing
kind: story
stage: implementing
tags: [documentation]
parent: null
depends_on: []
release_binding: v0.5.0
gate_origin: docs
created: 2026-05-31
updated: 2026-05-31
---

# `CHANGELOG.md` has no `v0.5.0` release entry

## Drift category
changelog-gap

## Location
- Doc: `CHANGELOG.md:3`

## Current doc text
> `## v0.4.1`

## Reality
Release `v0.5.0` has 68 bound items and 146 bundle files, but `CHANGELOG.md`
contains no `v0.5.0` section or entries for the bound work.

## Required edit
Add a top-level `## v0.5.0` section covering the release's CLI browser
handoff/session resume work, bug-squash fixes, e2e infrastructure fixes,
docs/security changes, and generated-contract changes.

