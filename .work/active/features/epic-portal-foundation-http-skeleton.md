---
id: epic-portal-foundation-http-skeleton
kind: feature
stage: drafting
tags: [portal]
parent: epic-portal-foundation
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-16
updated: 2026-05-16
---

# Portal Foundation — HTTP Skeleton

## Brief

The portal's HTTP server skeleton: process entry, chi router with the
per-subroute middleware shape that each auth mechanism needs, structured
logging, the standardized JSON error contract, configuration loading
(env vars + optional YAML), and TLS termination (native HTTPS with cert
paths AND HTTP-behind-trusted-proxy mode, config-selected).

This is the chassis every subsequent route group plugs into. Subroutes are
declared by route-group with their own middleware stacks: `/api/*` (Bearer
auth), `/git/*` (HTTP Basic, owned by `epic-portal-git`), `/mcp/*` (Bearer
auth via MCP headersHelper, owned by `epic-portal-api`), `/ws` (WebSocket
upgrade, owned by `epic-portal-api`). This feature stands up the chassis
and the `/api/*` Bearer-auth subroute scaffold; other epics mount their
groups against it.

The error contract from `docs/PROTOCOL.md > HTTP error contract` is enforced
by middleware that converts panics and recognized error types to the JSON
envelope (`error`, `message`, optional `details`). Structured logging
includes request ID, route, auth subject (when authenticated), and outcome.

Does NOT cover the auth middleware logic itself — that's the tokens
feature. Does NOT cover any concrete endpoint implementations — those
belong to auth-flows, accounts, or sibling epics.

## Epic context

- Parent epic: `epic-portal-foundation`
- Position in epic: parallel with data-layer; tokens and every other
  endpoint-bearing feature mounts against this chassis.

## Foundation references

- `docs/SPEC.md` — Stack > Backend, Hard constraints, Deployment shape
- `docs/ARCHITECTURE.md` — Portal component overview
- `docs/PROTOCOL.md` — HTTP error contract
- `docs/SECURITY.md` — Self-host security posture (TLS posture)

## Inherited epic design decisions

- **HTTP routing**: `chi` — per-subroute middleware stacks make the
  multi-auth shape clean.
- **TLS posture**: support both native HTTPS (cert path config) and
  HTTP-behind-trusted-proxy mode. Operator selects via config.

## Generated-contracts scope

This feature also owns the initial wiring for the spec-first generated-
contracts pipeline locked in `docs/SPEC.md > Generated contracts`:

- Bootstraps `docs/openapi.yaml` with the OpenAPI 3.1 skeleton (info,
  servers, security schemes for Bearer, `components/schemas/`
  placeholder, empty `paths`). Each subsequent REST feature's design
  pass adds its endpoints + schemas to this same file.
- Wires `oapi-codegen` (chi backend) into the Go build: a Makefile
  target `make generate` reads `docs/openapi.yaml` and produces
  generated Go interfaces under an internal package (e.g.,
  `internal/api/openapi/server.gen.go`).
- Wires `openapi-typescript` into the Vite frontend build similarly:
  `make generate` also produces TS types under
  `frontend/src/lib/api/types.gen.ts`.
- CI verifies sync: `make generate && git diff --exit-code` fails the
  build if the working tree disagrees with the spec.

The actual endpoint definitions in the spec are added by the REST
features as they're designed; this feature just establishes the
authoring + codegen pipeline so subsequent features have a place to
hang their schemas.

<!-- Feature-design will fill in interfaces, signatures, and implementation
units when /agile-workflow:feature-design runs on this. -->
