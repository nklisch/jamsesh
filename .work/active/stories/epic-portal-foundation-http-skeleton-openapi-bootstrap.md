---
id: epic-portal-foundation-http-skeleton-openapi-bootstrap
kind: story
stage: implementing
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
