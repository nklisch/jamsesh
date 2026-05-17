---
id: portal-test-ws-allow-origins-env-parsing
kind: story
stage: implementing
tags: [testing, portal]
parent: null
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-17
updated: 2026-05-17
---

# Test `JAMSESH_WS_ALLOW_ORIGINS` env-var parsing

`cmd/portal/main.go` parses `JAMSESH_WS_ALLOW_ORIGINS` into a
`[]string` for `wsgateway.Gateway.AllowOrigins` (comma-separated,
trimmed, empty entries dropped). The wiring has no unit test.

Add a table-driven test (likely in a new `cmd/portal/wsorigins_test.go`
or by extracting the parsing into a small package-level helper that
can be tested directly). Cases:

- empty string → `nil` (deny all)
- single origin → 1-element slice
- multiple origins comma-separated → N-element slice
- whitespace around commas → trimmed values
- empty entries (`",,a,,b,"`) → dropped, leaving only `["a","b"]`
- whitespace-only entries → dropped

The simplest path is to extract `parseAllowOrigins(envValue string) []string`
in `cmd/portal/main.go` and test that. Keep the change minimal — this
is a small piece of plumbing, not a refactor.

## History

Surfaced during review of `dev-docker-compose-setup` (commit `8d0e04e`),
which added the env-var parsing in `cmd/portal/main.go` lines 320-328.
