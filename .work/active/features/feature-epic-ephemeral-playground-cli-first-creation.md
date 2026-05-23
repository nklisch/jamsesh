---
id: feature-epic-ephemeral-playground-cli-first-creation
kind: feature
stage: implementing
tags: [plugin, portal]
parent: epic-ephemeral-playground
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-23
updated: 2026-05-23
---

# CLI-first session creation

## Brief

Adds the `jamsesh new` Go subcommand that drives session creation
end-to-end from the user's local checkout. The subcommand interactively
prompts for goal, writable scope, default mode, and org (when the user
has multiple), with flag overrides for every prompt for non-interactive
use (`--org`, `--goal`, `--scope`, `--mode`, `--invite`). After the
portal accepts the create call, it pushes the local HEAD as the session
base ref and writes the per-session state file under
`${CLAUDE_PLUGIN_DATA}/sessions/<session-id>/`.

This feature is the foundation that unifies session creation onto a
CLI-first pattern (replacing the current "create-in-portal, then
join-from-CLI" two-step). It is intentionally durable-only — no
playground concerns. The `session-lifecycle` sibling feature layers
`--playground` flag handling on top once anon bearers and the reserved
org exist.

The portal's session-create REST handler may need a small refactor: the
current shape assumes the join step seeds the base ref, while the
CLI-first flow expects to push HEAD immediately after creation. Confirm
the handler accepts a session with `base_sha: NULL` and stamps it on the
first push (the schema already allows nullable base_sha, so this may be
a no-op).

## Epic context
- Parent epic: `epic-ephemeral-playground`
- Position in epic: **wave 1 foundation** — no dependencies; required by
  `session-lifecycle` (wave 2) for the `--playground` flag handling and
  by `plugin-skills` (wave 3) for the `/jamsesh:new` skill.

## Foundation references
- `docs/SPEC.md` § Lifecycle — current durable creation flow
- `docs/ARCHITECTURE.md` § The `jamsesh` binary — existing subcommand
  surface that `jamsesh new` slots into
- `docs/UX.md` § Flow: creating a session — the flow this feature
  reworks; UX.md roll-forward is owned by this feature's design pass

## Mockups
- Inherits parent epic flow:
  `.mockups/flows/playground-onboarding/index.html` (step 02 is the
  representative CLI-output state). For durable creation, the CLI output
  is parallel: same shape, different identity (real account vs.
  anonymous handle) and different lifecycle indicators (no countdown).
  No additional mocks needed at the feature tier.

## Design decisions

Locked at `--only-questions` time. Feature-design Phase 5 inherits these
as fixed input.

### Overarching: agent-primary mental model

The primary user of `jamsesh new` is **a human in a Claude Code session
whose agent invokes the binary on their behalf**, not a human typing
in bash directly. Every UX decision below has two arms:

- **Agent path** (primary): the agent reads the human's natural-language
  request ("spin up a session for the auth refresh, scope to `docs/auth/**`,
  goal: tighten the OAuth callback contract"), maps that to explicit
  flags, and invokes `jamsesh new --org <id> --goal '<text>'
  --scope '<glob>' --mode <sync|isolated>`. The agent never sees stdin
  prompts because the CC bash tool doesn't drive a TTY; flags are the
  only deterministic interface. If params are missing, the agent
  asks the human inside CC, not via the binary's prompt.
- **Direct-human path** (fallback): a developer in their own terminal
  runs `jamsesh new` directly — the binary detects TTY, drops into
  interactive prompts. Useful for ops, debugging, scripting; not the
  primary case.

This framing belongs in the `/jamsesh:new` SKILL.md body (owned by the
`plugin-skills` sibling feature) so the agent is taught the parameter
mapping and the "ask in CC, never via stdin" rule.

### Decisions

- **Org picker when user has multiple orgs**: interactive picker with
  the most-recently-used org pre-selected (TTY only); `--org <id>` flag
  required when stdin is not a TTY (i.e. agent invocation). The agent
  pattern: parse the human's request for the org reference, error out
  early if ambiguous (the skill body teaches "if the human's request
  doesn't pin an org, ask them which one"). Hard-fails on non-TTY
  without `--org` rather than silently picking — silent picks risk
  agent-driven creates in the wrong tenant.

- **Invite handling**: both inline `--invite alice@x,bob@x` flag on
  `jamsesh new` AND a separate `jamsesh invite <session-id> <emails>`
  subcommand. The agent uses `--invite` when the human's create request
  mentions collaborators in one breath; the separate subcommand
  exists for follow-up adds. Both produce identical invite rows.

- **Post-create HEAD-push failure**: the session row stays live with
  `base_sha: NULL`; the CLI prints a clear retry command
  (`git push <session-remote> HEAD:jam/<session-id>/base`). The
  `/jamsesh:new` skill body instructs the agent to: (1) retry the push
  automatically once, (2) on second failure, surface the error to the
  human with the explicit retry command. The portal's `pre-receive`
  validates the first push and stamps `base_sha` then — schema already
  allows nullable `base_sha`, so this is consistent with existing
  semantics. No transactional packfile-on-create refactor.

- **Required vs optional fields at creation**: goal and writable scope
  are **optional with defaults** — goal defaults to empty (settable
  later via the portal UI or a planned `jamsesh edit`), writable scope
  defaults to `**` (permissive). Interactive prompts ask but `enter`
  accepts defaults; flags (`--goal`, `--scope`, `--mode`) override. Same
  agent-friendliness: when the human's create request omits a field,
  the agent passes the flag-default value (or asks if scope-default
  `**` is too permissive for the human's intent) rather than blocking
  on a required field.

## Architectural choice

**Layered orchestrator + focused helpers** in `cmd/jamsesh/sessioncmd/new.go`.
The `newAction` function orchestrates a sequence of single-purpose helpers
(`resolveCreateParams`, `pickOrgInteractive`, `createSessionAPI`,
`pushBaseRef`, `writeSessionState`, `sendInvitesIfRequested`), each
unit-testable via the project's function-stub idiom (see `join.go`'s
`runGit`/`runGitOutput` replacement pattern in `join_test.go`).

Why over alternatives:
- **Monolithic action**: harder to test, mixes prompt UX with API calls
  and git invocation. Doesn't match the existing `join.go` structure.
- **Refactor `join.go` to expose a shared "bind to session" helper**:
  couples the two subcommands. Future `--playground` variant (owned by
  `session-lifecycle`) would need to fork the shared helper, undoing the
  abstraction. Better to keep `new` and `join` parallel-but-separate.

Fits the existing `cmd/jamsesh/sessioncmd/` package's pattern: each
subcommand is its own file with `XxxCommand() *cli.Command` returning
the urfave/cli v3 registration. Reuses `portalclient` (PostJSON/GetJSON
helpers handle token refresh automatically) and `state` (atomic write +
session-dir layout under `${CLAUDE_PLUGIN_DATA}/`).

## Discovered scope (gap from explore)

The portal's session-create endpoint accepts `name`, `goal`, `scope`,
`default_mode` as **all required** per the OpenAPI spec, but the locked
design decision says "optional with defaults." Resolution: the CLI
generates defaults locally and always sends a complete request — keeps
the backend contract stable; no OpenAPI changes needed.

The portal's `SetSessionBaseSHA()` method exists in the store layer
(`internal/db/store/store.go`, queries in `db/queries/sqlite/sessions.sql`)
but is **never called in production**. The receive-pack post-receive
handler at `internal/portal/githttp/receive_pack.go` lines 260-310 seeds
the draft ref from the base commit but doesn't stamp `base_sha` on the
session row. This is a pre-existing gap; fixing it is necessary for the
CLI's success path to leave the session in a complete state. **Scope:
this feature owns the fix** as Story C below — it's small (~20 LOC),
discovered during the design pass, and not addressable from any other
feature in the epic.

## Implementation units

### Unit 1: `jamsesh new` subcommand registration
**File**: `cmd/jamsesh/sessioncmd/new.go`
**Story**: `story-epic-ephemeral-playground-cli-first-creation-new`

```go
package sessioncmd

import (
    "context"
    "github.com/urfave/cli/v3"
)

func NewCommand() *cli.Command {
    return &cli.Command{
        Name:  "new",
        Usage: "Create a session from the current repo checkout",
        Description: "Creates a session on the portal, pushes local HEAD as base ref, " +
            "and binds this Claude Code instance to your namespace in the session. " +
            "Run from inside a git checkout.",
        Flags: []cli.Flag{
            &cli.StringFlag{Name: "org", Usage: "Org ID (required when stdin is not a TTY)"},
            &cli.StringFlag{Name: "name", Usage: "Session name (default: jam-<timestamp>)"},
            &cli.StringFlag{Name: "goal", Usage: "Session goal"},
            &cli.StringFlag{Name: "scope", Value: "**", Usage: "Writable scope as a single glob or JSON array (default: '**')"},
            &cli.StringFlag{Name: "mode", Value: "sync", Usage: "Default mode (sync|isolated)"},
            &cli.StringFlag{Name: "invite", Usage: "Comma-separated emails to invite after creation"},
            &cli.BoolFlag{Name: "non-interactive", Usage: "Skip all prompts; require all params via flags"},
        },
        Action: newAction,
    }
}
```

Register in `cmd/jamsesh/main.go` alongside the existing sessioncmd entries.

**Acceptance criteria**:
- [ ] `jamsesh new --help` shows the listed flags and usage
- [ ] Registered in the binary's command list (verifiable via `jamsesh --help`)

---

### Unit 2: `newAction` orchestrator
**File**: `cmd/jamsesh/sessioncmd/new.go`
**Story**: `story-epic-ephemeral-playground-cli-first-creation-new`

```go
func newAction(ctx context.Context, cmd *cli.Command) error {
    // 1. Construct portal client (reads portal URL + token via state helpers)
    pc, err := buildPortalClient()
    if err != nil { return err }

    // 2. Resolve all params (flags + prompts + defaults)
    params, err := resolveCreateParams(ctx, cmd, pc)
    if err != nil { return err }

    // 3. Call portal API to create session row + member row
    session, err := createSessionAPI(ctx, pc, params)
    if err != nil { return err }

    // 4. Push local HEAD as base ref (may fail; if so, leave session live)
    pushErr := pushBaseRef(ctx, pc, session.ID)
    if pushErr != nil {
        // Per locked decision: session stays live with base_sha NULL.
        // CLI prints retry command, returns wrapped error.
        return wrapPushError(pushErr, session, pc.BaseURL)
    }

    // 5. Write per-session state files for subsequent jamsesh invocations
    if err := writeSessionState(session, params); err != nil { return err }

    // 6. If --invite flag set, send invites (best-effort; reports failures
    //    but doesn't fail the whole create)
    if invites := strings.TrimSpace(cmd.String("invite")); invites != "" {
        if err := sendInvitesIfRequested(ctx, pc, session.ID, parseInviteEmails(invites)); err != nil {
            // Print warning but don't fail; session is live and pushed.
            fmt.Fprintf(os.Stderr, "warning: invites partially failed: %v\n", err)
        }
    }

    // 7. Print success summary (session URL, your ref, ends-in if playground)
    printSuccessSummary(session, params, pc.BaseURL)

    // 8. Update most-recently-used org for next time's prompt pre-selection
    _ = state.Write("last_org_id", []byte(params.OrgID), 0600) // best-effort

    return nil
}
```

**Implementation notes**:
- `buildPortalClient` is a small wrapper around the existing `portalclient.Client{BaseURL, HTTP, Refresh}` construction + `portalclient.WireRefresh(pc)`. Extract for testability.
- Error wrapping for the push failure path includes the explicit retry command per the locked design decision.

**Acceptance criteria**:
- [ ] On happy path: session row created, base ref pushed, state files written, success summary printed, exit code 0
- [ ] On push failure: error message includes the retry command `git push <session-remote-url> HEAD:refs/heads/jam/<session-id>/base`; session NOT abandoned (still active per the locked decision); exit code 1
- [ ] On invite failure: warning printed to stderr; create still succeeds; exit code 0

---

### Unit 3: `resolveCreateParams`
**File**: `cmd/jamsesh/sessioncmd/new.go`
**Story**: `story-epic-ephemeral-playground-cli-first-creation-new`

```go
type CreateParams struct {
    OrgID       string
    Name        string  // never empty by the time this returns
    Goal        string  // may be empty
    Scope       string  // never empty; JSON array of globs
    DefaultMode string  // "sync" or "isolated"
}

func resolveCreateParams(ctx context.Context, cmd *cli.Command, pc *portalclient.Client) (CreateParams, error) {
    nonInteractive := cmd.Bool("non-interactive") || !isTTY(os.Stdin)

    params := CreateParams{
        OrgID:       cmd.String("org"),
        Name:        cmd.String("name"),
        Goal:        cmd.String("goal"),
        Scope:       cmd.String("scope"),
        DefaultMode: cmd.String("mode"),
    }

    // Org: required when non-interactive; picker when interactive multi-org
    if params.OrgID == "" {
        if nonInteractive {
            return CreateParams{}, errors.New("non-interactive mode: --org <id> is required (use `jamsesh status --json` to list orgs)")
        }
        picked, err := pickOrgInteractive(ctx, pc)
        if err != nil { return CreateParams{}, err }
        params.OrgID = picked
    }

    // Name: auto-generate if blank (jam-<unix-timestamp>)
    if params.Name == "" {
        params.Name = fmt.Sprintf("jam-%d", time.Now().Unix())
    }

    // Scope: normalize to JSON array (accept single glob like "docs/**" or a JSON array)
    params.Scope = normalizeScope(params.Scope)

    // Mode: validate
    if params.DefaultMode != "sync" && params.DefaultMode != "isolated" {
        return CreateParams{}, fmt.Errorf("invalid --mode %q: must be sync or isolated", params.DefaultMode)
    }

    return params, nil
}
```

**Implementation notes**:
- `isTTY(os.Stdin)` uses `golang.org/x/term`'s `term.IsTerminal(int(fd))`. Confirm `golang.org/x/term` is already an indirect dependency (check `go.mod`); if not, add it (small surface, std-adjacent).
- `normalizeScope` accepts either a single glob (`"docs/**"`) or a JSON array string (`'["docs/**","src/*.go"]'`). Normalizes to JSON-array form for the API. Validates the glob via `doublestar.ValidatePattern` if a single glob is provided.

**Acceptance criteria**:
- [ ] Non-TTY without `--org` fails fast with explicit error mentioning `--org` and `jamsesh status --json`
- [ ] Single-glob `--scope "docs/**"` normalizes to JSON array
- [ ] JSON-array `--scope '["a","b"]'` passes through unchanged
- [ ] Auto-generated name has form `jam-<unix-timestamp>`
- [ ] Invalid mode rejected with clear error

---

### Unit 4: `pickOrgInteractive`
**File**: `cmd/jamsesh/sessioncmd/new.go`
**Story**: `story-epic-ephemeral-playground-cli-first-creation-new`

```go
func pickOrgInteractive(ctx context.Context, pc *portalclient.Client) (string, error) {
    me, err := portalclient.GetJSON[openapi.Me](ctx, pc, "/api/me")
    if err != nil { return "", fmt.Errorf("fetch /api/me: %w", err) }

    if len(me.Orgs) == 0 {
        return "", errors.New("no org memberships yet; create one in the portal UI first")
    }
    if len(me.Orgs) == 1 {
        return me.Orgs[0].ID, nil // no picker needed
    }

    // Pre-select most-recently-used (best-effort read)
    preselected, _ := state.Read("last_org_id")

    // Numbered-list picker on stdin (simple, no arrow keys — matches the
    // existing `auth` flow's stdin-bufio.Scanner idiom)
    fmt.Println("Which org for this session?")
    defaultIdx := 0
    for i, org := range me.Orgs {
        marker := " "
        if string(preselected) == org.ID {
            marker = "*"; defaultIdx = i
        }
        fmt.Printf("  [%d]%s %s (%s)\n", i+1, marker, org.Name, org.ID)
    }
    fmt.Printf("Pick a number [1-%d, default %d]: ", len(me.Orgs), defaultIdx+1)

    line, err := readLine(os.Stdin)
    if err != nil { return "", fmt.Errorf("read picker input: %w", err) }
    line = strings.TrimSpace(line)
    if line == "" {
        return me.Orgs[defaultIdx].ID, nil
    }
    pick, err := strconv.Atoi(line)
    if err != nil || pick < 1 || pick > len(me.Orgs) {
        return "", fmt.Errorf("invalid pick %q", line)
    }
    return me.Orgs[pick-1].ID, nil
}
```

**Implementation notes**:
- Numbered-list picker, NOT arrow-key TUI. Simpler, matches the auth flow's stdin-line-read idiom. No external `survey`/`promptui` dependency.
- `state.Read("last_org_id")` is best-effort; missing file = no pre-selection, defaults to first org.
- Stamps an asterisk next to the pre-selected org for visual marking.

**Acceptance criteria**:
- [ ] Single-org user: returns the org ID without prompting
- [ ] Zero-org user: returns explicit error pointing at portal-UI org creation
- [ ] Multi-org user with last_org_id set: pre-selects it (default on enter)
- [ ] Multi-org user without last_org_id: defaults to first org
- [ ] Invalid pick (out-of-range, non-numeric) returns clear error

---

### Unit 5: `createSessionAPI`
**File**: `cmd/jamsesh/sessioncmd/new.go`
**Story**: `story-epic-ephemeral-playground-cli-first-creation-new`

```go
func createSessionAPI(ctx context.Context, pc *portalclient.Client, p CreateParams) (openapi.Session, error) {
    req := openapi.CreateSessionRequest{
        Name:        p.Name,
        Goal:        p.Goal, // may be empty string — OpenAPI requires non-null but accepts empty
        Scope:       p.Scope,
        DefaultMode: openapi.CreateSessionRequestDefaultMode(p.DefaultMode),
    }
    path := fmt.Sprintf("/api/orgs/%s/sessions", url.PathEscape(p.OrgID))
    return portalclient.PostJSON[openapi.Session](ctx, pc, path, req)
}
```

**Implementation notes**:
- Uses existing `portalclient.PostJSON[T]` helper — handles bearer attach + 401 refresh-retry automatically.
- OpenAPI spec marks `goal` required (string), but accepts empty string. CLI sends `""` when no goal provided; the portal handler validates max 4096 chars (which "" satisfies).
- Confirm in implementation: if the existing handler rejects empty `goal`, lift the rejection in the same PR (small handler change; locked design decision says goal is optional). If discovered, log it in the implementation notes.

**Acceptance criteria**:
- [ ] On 201: returns the decoded `Session` struct with ID, member list, base_sha=nil
- [ ] On 400 (e.g. invalid scope JSON): returns the error envelope from the portal verbatim
- [ ] On 401: portalclient's automatic refresh-and-retry kicks in; if refresh fails, returns auth error
- [ ] On 403 (not org member): returns clear error mentioning the org

---

### Unit 6: `pushBaseRef` (the trickiest unit)
**File**: `cmd/jamsesh/sessioncmd/new.go`
**Story**: `story-epic-ephemeral-playground-cli-first-creation-new`

```go
func pushBaseRef(ctx context.Context, pc *portalclient.Client, sessionID string) error {
    // Verify we're in a git checkout with a HEAD
    if err := runGit(ctx, "rev-parse", "--git-dir"); err != nil {
        return fmt.Errorf("not a git checkout: %w", err)
    }
    headSHA, err := runGitOutput(ctx, "rev-parse", "HEAD")
    if err != nil {
        return fmt.Errorf("repo has no commits yet (nothing to push as base): %w", err)
    }
    headSHA = strings.TrimSpace(headSHA)

    // Construct the session remote URL.
    //
    // Auth: HTTP Basic — username arbitrary, password is the OAuth bearer.
    // We pass credentials via `-c http.<base>.extraHeader` rather than
    // embedding in the URL (avoids leaking the token into git's reflog
    // or `git remote -v` if the user later inspects).
    token, err := state.ReadToken()
    if err != nil { return fmt.Errorf("read token: %w", err) }

    remoteURL := strings.TrimRight(pc.BaseURL, "/") + "/git/" + sessionID + ".git"
    basicHeader := "Authorization: Basic " + base64.StdEncoding.EncodeToString(
        []byte("jamsesh:" + string(token)))

    // Run the push — anonymous remote, no `git remote add` side effects.
    // Refspec: HEAD -> refs/heads/jam/<session>/base
    refspec := "HEAD:refs/heads/jam/" + sessionID + "/base"
    return runGitWithEnv(ctx,
        []string{}, // no extra env
        "-c", "http.extraHeader=" + basicHeader,
        "push", remoteURL, refspec,
    )
}
```

**Implementation notes**:
- **No `git remote add`** — we push to the URL directly. This avoids leaving a `jam-remote` entry in the user's `.git/config` that could leak the session URL on `git remote -v`.
- **Credential injection via `-c http.extraHeader`** instead of `https://user:pass@host` URL form. The latter leaks the token into git's process listing (visible to other users on shared systems) and into git's reflog. The header form is process-local.
- **`runGit`, `runGitOutput`, `runGitWithEnv`** are extensions of the existing `cmd/jamsesh/sessioncmd/git.go` helpers (already used by `join.go`'s test stubbing). New helper `runGitWithEnv` (or `runGitWithArgs`) added if not present — reuses the function-pointer pattern for test stubbing.
- **Error translation**: git's stderr for common failures (auth rejected, scope rejected, network down) is verbose. Post-process to extract the relevant line: look for `remote: ERROR:` lines (jamsesh `pre-receive` rejects use this prefix), `fatal:` lines, or `Authentication failed`.
- **Concurrency**: if the user runs `jamsesh new` in a repo with an in-flight `git push`, git's internal locks prevent races. No additional locking needed at the CLI layer.

**Acceptance criteria**:
- [ ] In a clean repo with at least one commit: push succeeds, ref `refs/heads/jam/<sessionID>/base` exists on the portal-side bare repo
- [ ] In an empty repo (no commits): clear error "repo has no commits yet"
- [ ] In a non-git directory: clear error "not a git checkout"
- [ ] Auth failure (stale token + refresh failed): clear error including hint to run `jamsesh auth`
- [ ] Scope rejection from pre-receive: surfaces the `remote: ERROR:` line in the CLI error (pre-receive shouldn't reject a base ref push, but if the repo isn't empty server-side, it will — handle the error message cleanly)
- [ ] After successful push: NO new entry in `.git/config` (verifies the no-remote-add approach)

---

### Unit 7: `writeSessionState`
**File**: `cmd/jamsesh/sessioncmd/new.go`
**Story**: `story-epic-ephemeral-playground-cli-first-creation-new`

```go
func writeSessionState(session openapi.Session, params CreateParams) error {
    sessionID := session.ID
    creatorAccountID := session.Members[0].AccountID // creator is the only member at this point
    ref := fmt.Sprintf("jam/%s/%s/main", sessionID, creatorAccountID)

    base := "sessions/" + sessionID
    writes := []struct{ name, value string }{
        {base + "/ref", ref},
        {base + "/org_id", params.OrgID},
        {base + "/account_id", creatorAccountID},
        {base + "/last_seen_seq", "0"},
    }
    for _, w := range writes {
        if err := state.Write(w.name, []byte(w.value), 0600); err != nil {
            return fmt.Errorf("write %s: %w", w.name, err)
        }
    }
    // instance_id binding happens on first /jamsesh:join-equivalent invocation
    // by the CC session — not at create time, since we may not have a CC session
    // attached yet (user might be running this in plain bash).
    return nil
}
```

**Implementation notes**:
- Mirrors the state-layout from the existing `join.go` flow.
- `instance_id` binding is deferred — `jamsesh new` doesn't know which CC instance (if any) will bind to this session.
- Mode 0600 per the state package's contract.

**Acceptance criteria**:
- [ ] All four state files exist under `${CLAUDE_PLUGIN_DATA}/sessions/<sessionID>/` after success
- [ ] File modes are 0600
- [ ] No `instance_id` file written (binding deferred to first attach)

---

### Unit 8: `sendInvitesIfRequested` + `parseInviteEmails`
**File**: `cmd/jamsesh/sessioncmd/new.go` (helpers shared with Unit 9)
**Story**: `story-epic-ephemeral-playground-cli-first-creation-new` (helper called)
+ `story-epic-ephemeral-playground-cli-first-creation-invite` (separate subcommand owns the implementation)

```go
func parseInviteEmails(raw string) []string {
    parts := strings.Split(raw, ",")
    out := make([]string, 0, len(parts))
    for _, p := range parts {
        p = strings.TrimSpace(p)
        if p != "" { out = append(out, p) }
    }
    return out
}

func sendInvitesIfRequested(ctx context.Context, pc *portalclient.Client, sessionID string, emails []string) error {
    var firstErr error
    var failedCount int
    for _, email := range emails {
        req := openapi.CreateSessionInviteRequest{Email: email}
        path := fmt.Sprintf("/api/sessions/%s/invites", url.PathEscape(sessionID))
        _, err := portalclient.PostJSON[openapi.SessionInvite](ctx, pc, path, req)
        if err != nil {
            failedCount++
            if firstErr == nil { firstErr = err }
            fmt.Fprintf(os.Stderr, "  invite %s: FAILED — %v\n", email, err)
            continue
        }
        fmt.Fprintf(os.Stderr, "  invite %s: sent\n", email)
    }
    if firstErr != nil {
        return fmt.Errorf("%d of %d invites failed (first error: %w)", failedCount, len(emails), firstErr)
    }
    return nil
}
```

**Implementation notes**:
- Confirm during implementation: the invite endpoint shape (`POST /api/sessions/{id}/invites` vs `POST /api/orgs/{org}/sessions/{id}/invites`). Adjust path accordingly.
- Partial failures are non-fatal at the CLI layer — `newAction` prints a warning but still considers the create a success. Rationale: the session is live; the user can retry invites via `jamsesh invite`.
- The Story B owns the actual `jamsesh invite` subcommand AND the underlying helper. This Unit 8 in `new.go` calls into that helper (cross-story import is normal within a package).

**Acceptance criteria**:
- [ ] All emails sent successfully → returns nil, prints one "sent" line per email
- [ ] Partial failure → returns wrapped error mentioning failed count; per-email outcomes printed
- [ ] Total failure (e.g. session deleted between create and invite) → returns error; all emails marked FAILED

---

### Unit 9: `jamsesh invite` subcommand
**File**: `cmd/jamsesh/sessioncmd/invite.go`
**Story**: `story-epic-ephemeral-playground-cli-first-creation-invite`

```go
func InviteCommand() *cli.Command {
    return &cli.Command{
        Name:      "invite",
        Usage:     "Invite one or more emails to an existing session",
        ArgsUsage: "<session-id> <email1>[,<email2>,...]",
        Action:    inviteAction,
    }
}

func inviteAction(ctx context.Context, cmd *cli.Command) error {
    args := cmd.Args().Slice()
    if len(args) < 2 {
        return fmt.Errorf("usage: jamsesh invite <session-id> <emails>")
    }
    sessionID := args[0]
    emails := parseInviteEmails(strings.Join(args[1:], ","))
    if len(emails) == 0 {
        return errors.New("no emails to send")
    }
    pc, err := buildPortalClient()
    if err != nil { return err }
    return sendInvitesIfRequested(ctx, pc, sessionID, emails)
}
```

Register in `cmd/jamsesh/main.go` next to `NewCommand()`.

**Acceptance criteria**:
- [ ] `jamsesh invite sess123 a@x.com,b@y.com` sends two invites
- [ ] `jamsesh invite sess123 a@x.com b@y.com c@z.com` (space-separated args) also works
- [ ] Missing session ID or emails → usage error, exit 1
- [ ] Sends are non-atomic (partial failure surfaced); exits non-zero if any failed

---

### Unit 10: Portal post-receive `base_sha` stamping
**File**: `internal/portal/githttp/receive_pack.go`
**Story**: `story-epic-ephemeral-playground-cli-first-creation-base-sha`

The post-receive handler at receive_pack.go lines 260-310 currently seeds
the draft ref from the base commit on first base push but doesn't stamp
the `sessions.base_sha` column. Add:

```go
// After draft-ref seeding succeeds, stamp the base SHA on the session row.
// Idempotent — only stamps when base_sha is currently NULL; subsequent
// base pushes (which pre-receive rejects anyway) would no-op.
if baseRef := findBaseRefUpdate(updates); baseRef != nil {
    if err := h.Store.SetSessionBaseSHA(ctx, store.SetSessionBaseSHAParams{
        OrgID:     orgID,
        SessionID: sessionID,
        BaseSHA:   sql.NullString{String: baseRef.NewSHA, Valid: true},
    }); err != nil {
        // Non-fatal: log but don't fail the push.
        // Worst case is the audit field stays NULL; doesn't affect runtime.
        h.Logger.Warn("set base_sha failed", "session_id", sessionID, "err", err)
    }
}
```

**Implementation notes**:
- `findBaseRefUpdate(updates)` is a small helper to scan the push's ref-updates list for one matching `refs/heads/jam/<sessionID>/base` (there can be at most one per the existing pre-receive validation).
- The `SetSessionBaseSHA` query in `db/queries/sqlite/sessions.sql` and `db/queries/postgres/sessions.sql` already exists (per the dual-dialect mirror pattern). Regenerate sqlc if any signature drift surfaces — but the query is already defined, so this should be a no-op for the codegen.
- Use `sql.NullString` per the existing sqlc-generated types for nullable columns.
- Log non-fatal — a missing base_sha is degraded but not broken.

**Acceptance criteria**:
- [ ] After the CLI's pushBaseRef succeeds, `sessions.base_sha` is populated with the head commit SHA
- [ ] Subsequent (non-base) ref pushes don't re-stamp `base_sha`
- [ ] If `SetSessionBaseSHA` fails (e.g. DB transient): logged as warning; push still succeeds (driving the user-visible flow)

---

## Implementation Order

Stories A, B, C are independent — `implement-orchestrator` runs all three
in parallel. After all three complete:
- Story C lets the CLI's success path leave `base_sha` correctly populated
- Story B's `jamsesh invite` subcommand is reachable
- Story A's `jamsesh new` orchestrator works end-to-end including the
  invite-flag pathway (which calls Story B's helpers)

Wave layout for the orchestrator: 3 parallel sub-agents, one per story.

## Testing

### Story A (`jamsesh new` subcommand)
**File**: `cmd/jamsesh/sessioncmd/new_test.go`

Tests follow the `join_test.go` pattern:

- `TestNewAction_happyPathSingleOrg` — mock portal returns 1 org from /api/me, 201 from create-session; stub git push to succeed; verify state files written, session URL in stdout, exit 0
- `TestNewAction_happyPathMultiOrg` — mock returns 2 orgs; set `last_org_id` state file; simulate stdin "enter" (default pick); verify the pre-selected org's ID flows to the create call
- `TestNewAction_nonInteractiveRequiresOrg` — `--non-interactive` without `--org`: returns error "non-interactive mode: --org <id> is required"
- `TestNewAction_pushFailureLeavesSessionLive` — stub git push to fail; verify error message contains retry command; verify NO portal call to abandon session
- `TestNewAction_inviteFlag` — `--invite a@x,b@y`; mock portal create-session then accept 2 invite posts; verify both invite paths called
- `TestNewAction_inviteFailureWarnsButSucceeds` — mock portal create-session OK + invite endpoint returns 500; verify stderr warning, exit 0
- `TestNewAction_emptyRepo` — `t.TempDir()` with no commits; verify "no commits yet" error
- `TestNewAction_defaultName` — no `--name`; verify the sent body's name has form `jam-<timestamp>`
- `TestNewAction_scopeNormalization` — `--scope "docs/**"` (single glob) → API receives `["docs/**"]`

Test helpers:
- Extend `cmd/jamsesh/sessioncmd/testhelpers_test.go`: add `setupNewEnv(t, srvURL) testEnv` paralleling `setupJoinEnv`
- Stub `runGit`/`runGitOutput`/`runGitWithEnv` per call (assigned in test, restored via `t.Cleanup`)
- Stub `isTTY` (extract to a package-level var like `isTTY = func(...) bool`) for non-TTY tests

### Story B (`jamsesh invite` subcommand)
**File**: `cmd/jamsesh/sessioncmd/invite_test.go`

- `TestInviteAction_happyPath` — two emails, two 201 responses; verify both posted
- `TestInviteAction_partialFailure` — first 201, second 500; verify error wraps "1 of 2 failed"
- `TestInviteAction_usageError` — missing emails or session ID; verify usage error
- `TestParseInviteEmails` — table-driven: comma-separated, space-mixed, empty entries trimmed

### Story C (post-receive `base_sha` stamping)
**File**: `internal/portal/githttp/receive_pack_test.go` (extend existing or add)

- `TestPostReceive_BaseRefStampsBaseSHA` — set up a session row with `base_sha: NULL`, simulate a base ref push via the receive-pack handler, verify `sessions.base_sha` is now populated with the pushed SHA
- `TestPostReceive_NonBaseRefDoesNotReStamp` — set base_sha to a known value, push a non-base ref (e.g. user ref), verify base_sha unchanged
- `TestPostReceive_SetBaseSHAFailureIsNonFatal` — inject a store wrapper that fails `SetSessionBaseSHA`, verify the push still succeeds and a warning is logged

Multi-dialect: per the `dual-dialect-mirror-queries` pattern, tests run
via `stores(t)` harness — SQLite always, Postgres when
`JAMSESH_TEST_PG_DSN` is set.

## Risks

- **TTY detection edge cases**: `golang.org/x/term`'s `IsTerminal` handles
  most cases but can misreport in certain CI environments and inside
  Docker without `-t`. Mitigation: `--non-interactive` flag override always
  works; users in weird environments can pass it explicitly. Document this
  in the SKILL.md (owned by plugin-skills feature).

- **`golang.org/x/term` dependency**: confirm it's already in `go.mod`
  before adding (likely indirect via the chi router or other dep). If
  fresh add, scope creep is minimal (stdlib-adjacent module).

- **Token leakage via git remote URL**: addressed by the
  `-c http.extraHeader` approach (Unit 6). Worth a comment in the unit's
  source to prevent a well-meaning refactor from switching to URL-embedded
  credentials.

- **Invite endpoint shape unverified**: the design assumes
  `POST /api/sessions/{id}/invites` exists. The exploration didn't
  confirm the exact path. Story B implementation must verify by reading
  `docs/openapi.yaml`; if the endpoint differs or doesn't exist, story
  scope expands. Realistic risk; documented as the first action item in
  Story B.

- **Partial failure semantics across invites**: chose "non-atomic + warn"
  per the locked agent-primary framing — the agent can retry individual
  failed invites via `jamsesh invite`. Alternative would have been
  transactional ("all or nothing") which adds substrate complexity for
  modest UX gain.

- **No CC-instance binding at create time**: `jamsesh new` doesn't write
  `instance_id` because the user might run it in plain bash, with no CC
  attached. The subsequent attach (via a CC `/jamsesh:join` equivalent on
  the same session) is what binds. This means the locked decision's
  "writes per-session state under `${CLAUDE_PLUGIN_DATA}/sessions/<id>/`"
  is partial — `instance_id` is deferred. Documented in Unit 7 above.

- **`docs/UX.md` roll-forward owed**: the locked decision noted UX.md
  needs updating to describe the CLI-first creation flow. The actual
  update is content-only (no code), can land in any of the three
  stories' commits. Story A is the natural place since it's the main
  user-facing change. Acceptance: UX.md "Flow: creating a session"
  section updated to describe the `jamsesh new` flow (or the unified
  flow that mentions both `jamsesh new` for durable and a forward
  reference to `jamsesh new --playground` for the playground variant
  shipped later in this epic).
