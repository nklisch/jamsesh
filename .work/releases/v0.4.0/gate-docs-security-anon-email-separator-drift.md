---
id: gate-docs-security-anon-email-separator-drift
kind: story
stage: done
tags: [documentation]
parent: null
depends_on: []
release_binding: v0.4.0
gate_origin: docs
created: 2026-05-24
updated: 2026-05-24
---

# SECURITY.md uses wrong separator in anonymous account synthetic email format

## Drift category
foundation-doc-assertion

## Location
- Doc: `docs/SECURITY.md:263`
- Code: `internal/portal/tokens/service_impl.go:206-207`

## Current doc text
> a new `accounts` row marked `is_anonymous: true` (with a synthetic `anon-<random>@playground.local` email)

## Reality
The code constructs `accountID := "anon_" + idSuffix` then `email := accountID + "@playground.local"`, producing `anon_<random>@playground.local` (underscore separator). `docs/SPEC.md:213` correctly uses `anon_`; SECURITY.md uses `anon-`.

## Required edit
Replace `anon-<random>@playground.local` with `anon_<random>@playground.local`.

## Implementation notes

Replaced `anon-<random>@playground.local` with `anon_<random>@playground.local` at `docs/SECURITY.md:263` to match the actual `accountID := "anon_" + idSuffix` construction in `internal/portal/tokens/service_impl.go:206-207`.

Verified: Foundation docs are markdown — no build/test step. Edits preserve the rolling-foundation discipline (no "previously" prose, no "in v1.x" notes; assertions replaced in place).
