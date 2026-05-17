---
id: epic-cc-plugin-binary-foundation-router-state-mcp
kind: story
stage: implementing
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
