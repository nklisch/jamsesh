---
id: portal-test-ws-allow-origins-env-parsing
kind: story
stage: done
tags: [testing, portal]
parent: null
depends_on: []
release_binding: v0.1.0
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

## Implementation notes

Extracted `parseAllowOrigins(v string) []string` to a new file
`cmd/portal/wsorigins.go`. The inline 6-line `for _, o := range
strings.Split(...)` block at `main.go:323-330` collapsed to a single
call: `wsAllowOrigins := parseAllowOrigins(os.Getenv("JAMSESH_WS_ALLOW_ORIGINS"))`.
The `strings` import is no longer needed in `main.go` and was removed.

Test file `cmd/portal/wsorigins_test.go` is a `package main` table-
driven test covering all six cases listed above plus four edge cases
spotted during writing:

- single `,` returns nil
- only whitespace returns nil
- trailing comma after a single entry
- leading comma before a single entry

10 test cases total. `go test ./cmd/portal/...` is green.

## Review findings — nits

- `parseAllowOrigins` doc comment is unusually thorough for a 12-line helper
  (mentions the nil-vs-empty distinction is "purely for clarity at call
  sites"). Fine as-is; if anything, it's a model for other helpers.
- Test case names are descriptive and the table-driven layout is clean.
  No changes requested.

Approve. Acceptance criteria satisfied: all six story-listed cases plus
four edge cases covered; `parseAllowOrigins` cleanly extracted with
sensible signature and doc; original inline behavior preserved
(empty → nil, trim, drop empty/blank); `strings` import removal from
`main.go` correct; `go test ./cmd/portal/...` green.
