---
id: gate-docs-architecture-security-org-protected-flag-not-documented
kind: story
stage: drafting
tags: [documentation]
parent: null
depends_on: []
release_binding: v0.4.0
gate_origin: docs
created: 2026-05-24
updated: 2026-05-24
---

# ARCHITECTURE.md / SECURITY.md do not document the `org_protected` flag introduced by migration 00017

## Drift category
foundation-doc-assertion

## Location
- Doc: `docs/ARCHITECTURE.md:550-557` (the "Reserved orgs" paragraph) and `docs/SECURITY.md` (no mention)
- Code: `internal/db/migrations/{sqlite,postgres}/00017_org_protected.sql`, `internal/portal/accounts/orgs.go` (`PatchOrg` guard per `story-extend-org-protected-guard-to-policy-mutations`)

## Current doc text
> ARCHITECTURE.md's "Reserved orgs" paragraph says the playground org is "auto-provisioned at startup" but does not mention that it carries `org_protected=true`, that delete / rename / policy-mutation operations are rejected with `409 org.protected`, or that the protection extends to `session_invite_policy` mutations.

## Reality
A new `org_protected` boolean column on `orgs` ships in this bundle (migrations 00017 in both dialects). `PatchOrg` returns `409 org.protected` when the target carries the flag, and the playground org is the sole protected row. Not documented.

## Required edit
Extend the "Reserved orgs" paragraph in `docs/ARCHITECTURE.md` to note the `org_protected` schema column and that the playground org is provisioned with `org_protected=true`, blocking delete / rename / policy mutations with `409 org.protected`. Add the `org.protected` error code to PROTOCOL.md's error-code list.
