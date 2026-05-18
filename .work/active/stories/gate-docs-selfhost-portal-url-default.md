---
id: gate-docs-selfhost-portal-url-default
kind: story
stage: implementing
tags: [documentation]
parent: null
depends_on: []
release_binding: v0.1.0
gate_origin: docs
created: 2026-05-18
updated: 2026-05-18
---

# SELF_HOST.md §2 reference table shows `JAMSESH_PORTAL_URL` default as "_(none)_" but the actual default is `http://localhost:8443`

## Drift category
foundation-doc-assertion

## Location
- Doc: `docs/SELF_HOST.md:116`
- Code: `internal/portal/config/config.go:351`
  (`PortalURL: "http://localhost:8443"`)

## Current doc text
> | `JAMSESH_PORTAL_URL` | `portal_url` | _(none)_ | Public base URL of
> the portal, e.g. `https://jamsesh.example.com`. Required when running
> behind a reverse proxy that does not forward `Host` and
> `X-Forwarded-Proto`. …

## Reality
When unset, `Config.PortalURL` defaults to `http://localhost:8443`.

## Required edit
Change the default cell to `http://localhost:8443`, keep the "Required
when … reverse proxy" qualification.
