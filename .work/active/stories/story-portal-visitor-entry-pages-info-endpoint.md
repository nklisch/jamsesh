---
id: story-portal-visitor-entry-pages-info-endpoint
kind: story
stage: implementing
tags: [portal, infra]
parent: feature-portal-visitor-entry-pages
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-24
updated: 2026-05-24
---

# Portal config + public `/api/portal/info` endpoint

## Brief

Add the `JAMSESH_LANDING_VARIANT` config knob and a public anonymous
`GET /api/portal/info` endpoint so the SPA can fetch the deploy's
landing-variant + playground-enabled state on bootstrap (before auth).
See `feature-portal-visitor-entry-pages.md` Implementation Unit 1 for
the full design.

## Scope

- New `LandingVariant string` field on the portal Config struct.
  Default `auto`. Validate enum `auto|project|login`; fail startup on
  invalid value (Fail Fast).
- New `internal/portal/portalinfo/` package with a `Handler` that
  serves `GET /api/portal/info` per the strict-server convention.
- Wire the route into the public route group in `cmd/portal/main.go`
  (no auth middleware).
- Add `PortalInfo` schema and the `GET /api/portal/info` path to
  `docs/openapi.yaml`. Regenerate via `go generate
  ./internal/api/openapi`.
- Roll forward `docs/SELF_HOST.md` (add the env var to the §2
  reference table; one paragraph on the three modes).

## Acceptance

- `JAMSESH_LANDING_VARIANT` (env) and `landing.variant` (YAML) both
  set the field; env takes precedence per the established pattern.
- Default behaviour: unset → `auto`.
- Invalid value → portal fails to start with a clear error message
  naming the offending value and the valid options.
- `GET /api/portal/info` returns 200 JSON `{playground_enabled,
  landing_variant}` matching the running config.
- The endpoint is reachable without any Authorization header.
- Table-driven handler test covering 4-6 `(playground_enabled,
  landing_variant)` combinations.
- `docs/SELF_HOST.md` reference table has the new row, in the same
  shape as adjacent rows; no "previously" prose.

## References

- Parent feature design: `.work/active/features/feature-portal-visitor-entry-pages.md`
- Existing config patterns: `internal/portal/config/config.go`
- Existing public-route registration: `cmd/portal/main.go:966` (the
  `/api/playground/sessions` group)
- OpenAPI generation: `go generate ./internal/api/openapi` (see
  `internal/api/openapi/*.gen.go` headers)
- Testenv pattern: `.claude/skills/patterns/testenv-harness-struct.md`
