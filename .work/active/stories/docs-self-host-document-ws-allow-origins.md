---
id: docs-self-host-document-ws-allow-origins
kind: story
stage: done
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

## Implementation notes

Added one row to the config table in `docs/SELF_HOST.md` (between the
`JAMSESH_OAUTH_GITHUB_BASE_URL` row and the YAML example) plus a
short paragraph immediately below the table explaining the value
pattern. The paragraph covers:

- Same-origin deployments leave the var unset.
- The exact format (scheme + host + port, verbatim, comma-separated).
- Two concrete examples (single CDN host; localhost dev compose with
  Vite + portal on separate ports).
- The "no wildcards, no trailing slash" gotcha — operators commonly
  trip on this when copy-pasting browser URLs into env files.

YAML-key column is `_(env-only)_` to match the existing convention for
env-only knobs.

## Review findings — nits

- "Origins are compared verbatim (scheme + host + port)" slightly understates
  the actual matching. The portal hands `AllowOrigins` to coder/websocket's
  `OriginPatterns`, which runs `path.Match` (case-insensitive) against either
  `host:port` (no scheme in pattern) or `scheme://host:port` (scheme present).
  Operator-facing impact is nil — copying the exact origin still works — but
  the wording could be tightened to "Patterns are matched case-insensitively
  against the request Origin's `scheme://host:port` (path ignored)."
- "Wildcards are not supported" is technically false — `path.Match` glob is
  honored. The upstream library explicitly warns against `*` (use
  `InsecureSkipVerify` instead). The doc steers operators correctly away from
  wildcards, but could phrase it as "Wildcard patterns are discouraged — list
  each origin exactly."
- Implementation notes describe the `_(env-only)_` YAML-key marker as matching
  "the existing convention for env-only knobs"; in fact this row establishes
  the convention (no prior env-only row exists in the table). Reasonable
  default; just noting the precedent is set here, not followed.

None of the above is load-bearing; approving as-is. If a future operator hits
the matching-semantics phrasing, file a follow-up parked item to tighten the
language.
