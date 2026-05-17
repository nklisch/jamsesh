---
id: epic-portal-foundation-http-skeleton-openapi-bootstrap
kind: story
stage: review
tags: [portal]
parent: epic-portal-foundation-http-skeleton
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-16
updated: 2026-05-16
---

# HTTP Skeleton — OpenAPI Bootstrap

## Scope

Stand up the spec-first generated-contracts pipeline locked in
`docs/SPEC.md > Generated contracts`. After this story:

- `docs/openapi.yaml` exists with the OpenAPI 3.0.3 skeleton
- `oapi-codegen` generates `internal/api/openapi/server.gen.go`
- `openapi-typescript` generates `frontend/src/lib/api/types.gen.ts`
- `make generate` reproduces both outputs; CI's
  `make generate && git diff --exit-code` is green
- Subsequent REST features have a place to hang their endpoints

## Units delivered

- **Unit 5**: `docs/openapi.yaml` — 3.0.3 skeleton with security
  scheme, `ErrorEnvelope` schema, empty `paths`
- **Unit 6**: `oapi-codegen.yaml`, `internal/api/openapi/doc.go`
  (with `//go:generate` directive),
  `internal/api/openapi/server.gen.go` (generated, committed)
- **Unit 7**: `frontend/package.json` (pinning `openapi-typescript`
  ^7.5.0 and `typescript` ^5.4.0), `frontend/tsconfig.json`,
  `frontend/.gitignore`, `frontend/src/lib/api/types.gen.ts`
  (generated, committed)
- **Unit 8**: `Makefile` — `.PHONY` targets `generate`, `generate-db`,
  `generate-api`, `generate-api-go`, `generate-api-ts`

## go.mod / module additions

`go.mod` gains `github.com/oapi-codegen/oapi-codegen/v2` (tool
dependency via `go run`) and `github.com/oapi-codegen/runtime`.
If the `go.mod` doesn't exist when this story lands, create it
(`module jamsesh`, `go 1.22`).

## Makefile coordination

The `data-layer-queries-and-codegen` story also touches the root
`Makefile` to add `generate-db`. Whichever story lands first creates
the file with the targets it owns; the second uses `Edit` to add its
targets. Implementer should `cat Makefile` defensively before deciding
between `Write` and `Edit`.

## Acceptance Criteria

- [ ] `npx @redocly/cli lint docs/openapi.yaml` passes (or equivalent
      3.0.3 validator)
- [ ] `make generate-api-go` regenerates `server.gen.go` cleanly
- [ ] `make generate-api-ts` regenerates `types.gen.ts` cleanly
- [ ] `make generate && git diff --exit-code` is green from a fresh
      checkout (after `frontend && npm install`)
- [ ] `internal/api/openapi/server.gen.go` exports
      `StrictServerInterface` (currently empty interface body) and
      the `ErrorEnvelope` Go model
- [ ] `frontend/src/lib/api/types.gen.ts` exports `paths`,
      `components`, `operations` (empty but well-typed)
- [ ] Generated files include a header noting they are generated and
      should not be hand-edited

## Notes

- Spec stays at `openapi: 3.0.3` per the locked decision until
  oapi-codegen mainline supports 3.1 (see SPEC.md migration trigger).
- The frontend skeleton in this story is the bare minimum to make
  codegen targets land. `epic-portal-ui-foundation` adds the full
  Svelte 5 + Vite app structure on top.
- Do NOT add path definitions — those belong to the features that own
  the routes.

## Implementation notes

### Landed files

- `docs/openapi.yaml` — OpenAPI 3.0.3 skeleton with `bearerAuth`
  security scheme, `ErrorEnvelope` schema, `Unauthorized`/`Forbidden`/
  `NotFound` reusable responses, empty `paths: {}`
- `oapi-codegen.yaml` — chi-strict server config with `skip-prune: true`
  under `output-options` (required to force `ErrorEnvelope` model
  generation despite no paths referencing it)
- `internal/api/openapi/doc.go` — package declaration + `//go:generate`
  directive (paths relative to package directory: `../../../oapi-codegen.yaml`)
- `internal/api/openapi/server.gen.go` — generated; exports
  `StrictServerInterface` (empty interface, expected with empty paths)
  and `ErrorEnvelope` Go model plus response type aliases
- `frontend/package.json` — pins `openapi-typescript@~7.13.0`,
  `typescript@^5.4.0`; generate script invokes `openapi-typescript`
- `frontend/tsconfig.json` — minimal ES2022/ESNext/Bundler config
- `frontend/.gitignore` — excludes `node_modules/`, `dist/`
- `frontend/src/lib/api/types.gen.ts` — generated; exports `paths`,
  `components` (with `ErrorEnvelope` schema + response types),
  `operations` (all empty, well-typed)
- `Makefile` — `.PHONY` targets: `generate`, `generate-db`,
  `generate-api`, `generate-api-go`, `generate-api-ts`
- `tools/tools.go` — `//go:build tools` file anchoring the
  `oapi-codegen/v2/cmd/oapi-codegen` tool dependency so `go mod tidy`
  does not remove it
- `go.mod` / `go.sum` — added `github.com/oapi-codegen/runtime@v1.4.0`
  and `github.com/oapi-codegen/oapi-codegen/v2@v2.7.0` (plus transitive
  deps including `getkin/kin-openapi@v0.135.0`)

### Dependency versions chosen

- `oapi-codegen/oapi-codegen/v2`: v2.7.0 (latest as of 2026-05-16)
- `oapi-codegen/runtime`: v1.4.0
- `openapi-typescript`: 7.13.0 (resolved from `~7.13.0`)

### Key discovery: skip-prune required

With `paths: {}`, oapi-codegen's default pruning removes unused schemas
including `ErrorEnvelope`. Adding `output-options.skip-prune: true` forces
all `components.schemas` to be emitted regardless of path references.
This is the correct approach for a bootstrap that pre-declares shared
error types before any paths exist.

### Verification results

- `make generate-api-go` — clean (no output, no error)
- `make generate-api-ts` — clean (openapi-typescript 7.13.0 emits
  `types.gen.ts` in 15ms)
- `make generate-api && git diff --exit-code` — green (exit 0)
- `go build ./...` — clean
- `npx @redocly/cli lint docs/openapi.yaml` — valid, 4 warnings (all
  expected: missing `license` field, 3 unused components — components
  will be used once paths are added by subsequent features)
