---
id: story-portal-visitor-entry-pages-info-endpoint
kind: story
stage: done
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

## Implementation notes

**Config (`internal/portal/config/config.go`):**
- Added `LandingConfig` sub-struct with `Variant string` field (YAML tag
  `landing.variant`, matching the nested key path used by TLS, DB, etc).
- Added `Landing LandingConfig` field to `Config` (YAML tag `landing`).
- Default: `"auto"` set in `defaults()`.
- Env-var overlay: `readEnvString("JAMSESH_LANDING_VARIANT", &c.Landing.Variant)` in `applyEnv()`.
- Validation: `switch c.Landing.Variant { case "auto", "project", "login": ... }` in `validate()`.
- Package-level doc comment updated with new YAML key and env var.

**Handler (`internal/portal/portalinfo/handler.go`):**
- New package `portalinfo` with `Handler` struct holding `PlaygroundEnabled bool`
  and `LandingVariant string` captured at construction time (immutable snapshot).
- `GetPortalInfo` returns `openapi.GetPortalInfo200JSONResponse` with the config
  snapshot. Zero deps beyond the openapi package — no store, no clock.

**Tests:**
- `internal/portal/portalinfo/handler_test.go` — 6-case table-driven test
  covering all 3 landing variants × 2 playground states, plus a
  `TestGetPortalInfo_NoAuthRequired` confirming no auth header is needed.
  Uses the `strict-server-partial-handler-shim` pattern with `portalInfoOnlyStrict`.
- `internal/portal/config/config_test.go` — 6 new tests: default value, env
  override, YAML parsing, env-over-YAML precedence, invalid-value error, all-valid-values.

**OpenAPI (`docs/openapi.yaml`):**
- Added `PortalInfo` schema to `components.schemas` with `playground_enabled:
  boolean` and `landing_variant: string enum [auto, project, login]`.
- Added `GET /api/portal/info` path with `security: []` (no auth), 200 returns
  `PortalInfo`. Inserted before `/api/me`.
- Ran `go generate ./internal/api/openapi` — generator produced
  `PortalInfoLandingVariant` enum type with constants `Auto`, `Login`, `Project`,
  `GetPortalInfo` on `StrictServerInterface`, and corresponding request/response
  object types.

**Wiring (`cmd/portal/main.go`):**
- Added `PortalInfoHandler *portalinfo.Handler` field to `combinedHandler`.
- Added `GetPortalInfo` delegation method on `combinedHandler`.
- Constructs `portalInfoHandler` from `cfg.PlaygroundEnabled` and `cfg.Landing.Variant`.
- Registered `r.Get("/portal/info", apiWrapper.GetPortalInfo)` in the
  unauthenticated route group, alongside playground public routes.

**Shim updates:**
- Added `GetPortalInfo` stub to the 8 existing `StrictServerInterface` shims
  across `playground`, `tokens`, `accounts`, `auth` (magic_link, oauth),
  `wsgateway`, `comments`, `sessions`, and `router` test files to keep compile.

**Docs (`docs/SELF_HOST.md`):**
- Added `JAMSESH_LANDING_VARIANT` row to the §2 reference table (same column
  shape as adjacent rows).
- Added one-paragraph `**On JAMSESH_LANDING_VARIANT.**` explanation of the three
  modes and the playground-discoverability rationale immediately after the row.

**Verification:** `go generate ./internal/api/openapi && go build ./... && go vet ./... && go test ./...` — all clean, 57 packages pass.

## Review (2026-05-24)

**Verdict**: Approve

**Blockers**: none
**Important**: none
**Nits**:
- The new default `landing.variant: auto` will change behavior on upgrade for
  existing deploys with `JAMSESH_PLAYGROUND_ENABLED=true` (anonymous `/` will
  start redirecting to `/playground` once the SPA side ships). The behavior
  change is intentional discoverability per the feature design, but worth a
  release-notes call-out when next version ships.

**Notes**: Implementation matches design exactly. Snapshot-at-construction
pattern correctly honors config-is-immutable-post-startup. Public-route
placement alongside playground routes matches the established pattern.
SELF_HOST.md roll-forward complete; UX.md roll-forward is correctly assigned
to the sibling spa-landing story. Tests are real (no skips/silenced asserts);
the strict-server-partial-handler-shim updates across 9 test files are
mechanical-correct.

**Next**: Unblocks `story-portal-visitor-entry-pages-spa-landing` (which had
`depends_on: [story-portal-visitor-entry-pages-info-endpoint]`).
