---
id: e2e-audit-cli-jam-playground-flag-end-to-end
kind: story
stage: done
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

## Implementation notes

Test landed at `tests/e2e/golden/cli_jam_playground_flag_test.go` —
~240 LoC end-to-end against real portal binary + real Postgres + real
`jamsesh new --playground` subprocess. The test ships with `t.Skip`
linked to `bug-playground-git-receive-pack-fails-with-200-hangup` (a
real product bug surfaced during implementation that has no path-of-least
-resistance fix from this story).

**Bugs surfaced and resolved inline as part of this story:**

1. `idea-playground-scope-normalization-bug` — `newPlaygroundAction`
   in `cmd/jamsesh/sessioncmd/new.go:390` skipped `normalizeScope` when
   `req.Scope == "**"` (the flag default), sending raw `"**"` to the
   portal which 400-rejected with `session.invalid_writable_scope`.
   Fixed by removing the `req.Scope != "**"` guard — the default is now
   normalized the same as any user-supplied value.

2. Playground git push URL was missing the `org_id` segment — the portal
   route is `/git/{orgID}/{sessionID}.git/...` but the binary was
   pushing to `/git/{sessionID}.git/...`. Fixed in two places in
   `cmd/jamsesh/sessioncmd/new.go`:
   - `pushBaseRefWithBearer` (line ~448) — the actual push URL.
   - `wrapPlaygroundPushError` (line ~498) — the retry-hint message.

3. Updated stale unit test
   `TestPlaygroundAction_pushFailureLeavesSessionLiveWithRetry` to
   assert on the corrected URL shape (`/git/org_playground/{sessionID}.git`).

**Bug surfaced but parked (blocks this test):**

- `bug-playground-git-receive-pack-fails-with-200-hangup` — auth passes
  through the `basicAuth` middleware (anon bearer accepted by
  `tokens.BasicAuthValidator`), the URL is correct, but the
  receive-pack subprocess responds with HTTP 200 + ~166 bytes in ~1ms
  and git client-side reports "fatal: the remote end hung up
  unexpectedly". Root cause not investigated in autopilot scope — the
  fix needs a `slog.Debug` trace through the receive-pack pkt-line
  output to find the early-exit point. Likely either an anon-account
  shape mismatch with what `receivePack` expects, or a storage-layout
  mismatch for the reserved `org_playground` org.

**Out-of-scope discovery:**

- The `wrapPushError` function (durable-session variant of
  `wrapPlaygroundPushError`) has the same missing-org_id bug as the
  pre-fix playground variant did. Left as-is with a comment pointing
  at `bug-playground-git-receive-pack-fails-with-200-hangup` for
  follow-up; not fixed because it's outside this story's playground
  scope and would have its own production impact assessment.

**Anti-tautology discipline (Unit 5 application):**

The test includes a `p.Exec(ctx, []string{"ls", repoPath})` assertion
that the bare repo exists on real disk in the portal container after
the binary runs. This is the assertion-shape Unit 5 mandates. Will
fire once the receive-pack bug is resolved.

Re-enable with `git grep -n "blocked on bug-playground-git-receive-pack"`
when the bug closes.

## Review (2026-05-24)

**Verdict**: Approve

**Blockers**: none
**Important**: none
**Nits**: none

**Notes**:

Test passes end-to-end now that the trailer-exemption fix at `297616a`
landed. The test exercises the real `jamsesh new --playground` binary
against a real portal + real Postgres, asserts on real state-file
contents in a tempdir-scoped `CLAUDE_PLUGIN_DATA`, and verifies the
bare repo exists on real disk via `p.Exec` (Unit 5 discipline).

Three production bugs that this story surfaced are all resolved:
- `idea-playground-scope-normalization-bug` — fixed inline at `2bf22ea`
- Playground push URL missing org_id — fixed inline at `2bf22ea`
- `bug-playground-git-receive-pack-fails-with-200-hangup` — resolved
  via the trailer-exemption story `story-fix-playground-base-ref-trailer-exemption`

The stale token-prefix assertion (`jamsesh_anon_` prefix that doesn't
actually exist in the codebase) was repaired in-session as part of the
trailer-exemption fix's review.

Advanced `stage: review → done`.
