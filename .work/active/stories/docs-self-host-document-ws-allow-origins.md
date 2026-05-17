---
id: docs-self-host-document-ws-allow-origins
kind: story
stage: implementing
tags: [documentation, portal]
parent: null
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-17
updated: 2026-05-17
---

# Document `JAMSESH_WS_ALLOW_ORIGINS` in `docs/SELF_HOST.md`

The portal honors `JAMSESH_WS_ALLOW_ORIGINS` (comma-separated origins) to
allow cross-origin WebSocket upgrades to `/ws/sessions/{sessionID}`. The
env var is referenced from `cmd/portal/main.go` which directs operators
to `docs/SELF_HOST.md` for the configuration table, but the table
(`docs/SELF_HOST.md:103-114`) does not list this env var.

Add a row to the configuration table with:

- env var: `JAMSESH_WS_ALLOW_ORIGINS`
- YAML key: _(none yet — currently env-only)_
- default: _(none — empty means deny all cross-origin)_
- description: Comma-separated list of allowed Origin headers for
  cross-origin WebSocket upgrades. Empty (default) denies all
  cross-origin. Set per-deployment to the public origin of the SPA
  when it is served from a different origin than the portal.

Optionally, also document the value pattern in a short paragraph below
the table — operators commonly trip on this when serving the SPA from a
CDN or alternate hostname.

## History

Surfaced during review of `dev-docker-compose-setup` (commit `8d0e04e`).
The dev story made the env var newly functional (wiring landed in
`cmd/portal/main.go`); the operator-facing doc gap is now load-bearing.
