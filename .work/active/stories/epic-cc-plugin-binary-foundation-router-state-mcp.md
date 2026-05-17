---
id: epic-cc-plugin-binary-foundation-router-state-mcp
kind: story
stage: done
tags: [plugin]
parent: epic-cc-plugin-binary-foundation
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-16
updated: 2026-05-16
---

# CC Binary Foundation — Router, State, MCP Headers

## Scope

Stand up the `jamsesh` Go binary's foundation: subcommand router via
urfave/cli/v3, hook IO scaffold, local state package with atomic
writes, and the `jamsesh mcp-headers` subcommand. After this story,
the binary builds and `jamsesh mcp-headers` works (assuming
`${CLAUDE_PLUGIN_DATA}/token` exists).

## Units delivered

- `cmd/jamsesh/main.go` — urfave/cli/v3 app + subcommand registration + signal handling
- `cmd/jamsesh/state/state.go` + `state_test.go` — PluginDataDir, Read, Write (atomic), typed wrappers
- `cmd/jamsesh/hookio/hookio.go` + `hookio_test.go` — generic Run helper
- `cmd/jamsesh/mcpheaders/mcpheaders.go` + `mcpheaders_test.go` — the subcommand
- Add `github.com/urfave/cli/v3` to go.mod

## Acceptance Criteria

- [ ] `go build ./cmd/jamsesh` produces a binary
- [ ] `jamsesh --help` lists `auth`, `mcp-headers` subcommands (auth registered as a placeholder until next story; `mcp-headers` works)
- [ ] `jamsesh --version` prints the buildinfo version
- [ ] `CLAUDE_PLUGIN_DATA=$tmpdir jamsesh mcp-headers` reads `$tmpdir/token` and outputs `{"Authorization":"Bearer <token>"}`
- [ ] `CLAUDE_PLUGIN_DATA=$tmpdir jamsesh mcp-headers` (no token file) exits 2 with stderr message
- [ ] `state.Write` is atomic — verified by a test that opens the temp file mid-write (use injectable temp-name helper if needed) and confirms target file doesn't appear until rename
- [ ] `state.WriteToken` produces a file at mode 0600
- [ ] `hookio.Run` round-trips JSON in → handler → JSON out
- [ ] All tests green: `go test ./cmd/jamsesh/...`

## Notes

- Use the `internal/buildinfo` package (already exists) to source the Version string for `--version`.
- The `auth` subcommand should be registered with a stub Action that prints "auth subcommand lands in the next story". The cli structure is here; behavior comes from the oauth-browser-and-device story.
- Token files MUST be mode 0600. The test enforces this.

## Implementation notes

Files created:
- `cmd/jamsesh/main.go` — urfave/cli/v3 app wired with `auth` stub and `mcp-headers` subcommands; signal.NotifyContext for graceful interrupt; `buildinfo.Version` for `--version`.
- `cmd/jamsesh/auth/auth.go` — stub Command() printing "auth subcommand lands in the next story".
- `cmd/jamsesh/state/state.go` — `PluginDataDir()`, `Read()`, `Write()` (atomic via temp+rename+chmod before rename), typed wrappers `ReadToken`/`WriteToken`/`ReadRefreshToken`/`WriteRefreshToken`/`ReadPortalURL` with env→file→default precedence.
- `cmd/jamsesh/state/state_test.go` — round-trip, ErrNotExist propagation, no-temp-leakage after successful Write, mode 0600 enforcement, whitespace trimming, ReadPortalURL precedence.
- `cmd/jamsesh/hookio/hookio.go` — generic `Run[I, O any]` reading stdin, calling handler, encoding output; error envelope `{"error":…,"message":…}` on failure.
- `cmd/jamsesh/hookio/hookio_test.go` — happy path, malformed JSON, handler error, empty-object input.
- `cmd/jamsesh/mcpheaders/mcpheaders.go` — `mcp-headers` Command() reading token via state.ReadToken, exiting 2 with stderr message on missing token.
- `cmd/jamsesh/mcpheaders/mcpheaders_test.go` — integration tests building the binary; token-present outputs `{"Authorization":"Bearer …"}`; missing token exits 2 with non-empty stderr.

Dependency added: `github.com/urfave/cli/v3 v3.9.0`

All acceptance criteria verified:
- `go build ./cmd/jamsesh` — produces binary
- `jamsesh --help` lists `auth`, `mcp-headers`
- `jamsesh --version` prints `jamsesh version dev`
- `CLAUDE_PLUGIN_DATA=$tmp jamsesh mcp-headers` → `{"Authorization":"Bearer bogus"}`
- Missing token → exit 2 + stderr (covered by mcpheaders_test.go)
- Atomic write: no temp leakage (state_test.go TestWrite_atomicNoTempLeakage)
- Mode 0600 enforced (state_test.go TestWrite_mode0600)
- hookio round-trip green
- `go test ./cmd/jamsesh/...` — all green

## Review (2026-05-16)

**Verdict**: Approve

**Blockers**: none
**Important**: none
**Nits**: none

**Notes**: Foundation laid clean. urfave/cli/v3 wiring, atomic state writes verified, hookio generic helper, mcp-headers subcommand returns proper Bearer JSON. Integration test compiles the binary to verify exit codes — solid.
