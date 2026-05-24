---
id: e2e-audit-cli-jam-playground-flag-end-to-end
kind: story
stage: implementing
tags: [testing, e2e-test, audit, playground, cli]
parent: feature-e2e-playground-coverage-golden
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-24
updated: 2026-05-24
---

# `jamsesh jam --playground` has no e2e test — CLI binary never exercised against a real portal anonymously

## Severity
Critical

## Finding type
journey-gap

## Evidence

Grep for the CLI binary's anonymous create path:

```
$ grep -rIn -E "jam --playground|jam.*--playground|JAMSESH_PLAYGROUND" tests/e2e/
(no output)
```

The CLI binary fixture exists at `tests/e2e/fixtures/binary/jamsesh.go` and
is used in exactly one place — `tests/e2e/golden/fork_and_comment_test.go`
(line 16 imports `jamsesh/tests/e2e/fixtures/binary`). That test exercises
the authenticated MCP / hook surface; **no test exercises the CLI's
top-level `jam` command at all**, let alone with `--playground`.

Unit coverage for the `jam` command exists at
`cmd/jamsesh/jamcmd/jam_test.go` but is purely structural:
- `TestJamCommand_Help` — registration smoke
- `TestJamCommand_JoinMissingArg` — flag parsing
- `TestJamCommand_NewMissingOrg` — flag parsing
- `TestJamCommand_TopLevelCommandsUnaffected` — command-tree shape
- `TestJamCommand_IndependentInstances` — instance isolation

None invoke the binary, none hit a portal, none write to
`~/.jamsesh/state.json`, none mint an anonymous bearer.

## Why this matters

The CLI is the user-facing entry point for `--playground`. State
persistence at `~/.jamsesh/state.json` (per-session token, org_id,
account_id) crosses a filesystem boundary that unit tests stub out via
`CLAUDE_PLUGIN_DATA` env override
(`cmd/jamsesh/jamcmd/jam_test.go:35`). A regression in the binary's
playground flag — wrong default URL, wrong token persistence format,
malformed POST body, mishandled non-2xx response — would slip past every
test in CI. The CLI binary fixture pattern is established
(`fork_and_comment_test.go` proves it works); this gap is purely a
missing test, not a missing fixture.

## Suggested remedy

Add `tests/e2e/golden/cli_jam_playground_test.go`. Use `binary.Build(t)`
to compile the CLI. Use the real `portal` fixture with playground enabled.
Set `HOME` to a tempdir to redirect `~/.jamsesh/state.json` writes. Run
`jamsesh jam --playground` as a subprocess via `exec.Command`. Assert:
1. Exit code 0.
2. stdout contains a `session_id`.
3. `~/.jamsesh/state.json` exists, parses as JSON, contains a
   `jamsesh_anon_*` token.
4. Portal-side `GET /api/playground/sessions/<id>` with that bearer
   returns 200.

If the `jam --playground` flow opens a browser, gate the browser-launch
step behind a `--no-browser` flag the binary already accepts (or skip if
not yet implemented and park a follow-on).

## Test sketch

```go
// tests/e2e/golden/cli_jam_playground_test.go
func TestCLI_Jam_PlaygroundFlag(t *testing.T) {
    ctx := context.Background()
    pg := postgres.Start(ctx, t, postgres.Options{})
    p := portal.Start(ctx, t, portal.Options{
        DBDriver:          "postgres",
        DBDSN:             pg.ContainerDSN,
        PlaygroundEnabled: true,
    })

    binPath := binary.Build(t)
    home := t.TempDir()
    t.Setenv("HOME", home)
    t.Setenv("JAMSESH_PORTAL_URL", p.URL)

    cmd := exec.CommandContext(ctx, binPath, "jam", "--playground", "--no-browser")
    out, err := cmd.CombinedOutput()
    if err != nil {
        t.Fatalf("jam --playground: %v\n%s", err, out)
    }

    sessionID := extractSessionID(t, out)

    // Verify state file written.
    statePath := filepath.Join(home, ".jamsesh", "state.json")
    var state struct {
        Sessions map[string]struct {
            Token string `json:"token"`
            OrgID string `json:"org_id"`
        } `json:"sessions"`
    }
    requireJSONFile(t, statePath, &state)
    sess, ok := state.Sessions[sessionID]
    if !ok || !strings.HasPrefix(sess.Token, "jamsesh_anon_") {
        t.Fatalf("state missing anon bearer for %s", sessionID)
    }

    // Portal-side round trip with the bearer.
    summary := getJSON(t, p.URL+"/api/playground/sessions/"+sessionID, sess.Token)
    // assert: 200, summary.OrgID == "playground".
}
```
