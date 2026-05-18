---
id: epic-finalize-flow-plugin-finalize-command
kind: feature
stage: done
tags: [plugin]
parent: epic-finalize-flow
depends_on: [epic-finalize-flow-plan-generation, epic-cc-plugin-binary-foundation]
release_binding: v0.1.0
gate_origin: null
created: 2026-05-16
updated: 2026-05-17
---

# Finalize Flow — Plugin Finalize Command

## Brief

The two `jamsesh` binary subcommands that wire the local side of
finalize:

- **`jamsesh finalize`** — the entry point. Default: opens the
  portal's finalize view in the user's browser
  (`https://<portal>/sessions/<session>/finalize`). With `--local`:
  fetches the plan and prints it to stdout (for headless users who
  curated via web from another device and just want the cherry-pick
  script locally).

- **`jamsesh finalize-run <plan-id>`** — the one-click execution
  command the portal UI hands the user via copy-to-clipboard. The
  flow:
  1. Resolve plan-id (format `<session_id>:<lock_id>`).
  2. **Mid-pick detection** — before anything else, check `git
     status` in the cwd. If `CHERRY_PICK_HEAD` is present (the user
     resolved a previous run's conflict and re-invoked us), report
     the offending commit and what remains in the sequence based on
     the plan's commit list, then point them at `git cherry-pick
     --continue` (or `--abort`) and exit. The binary does NOT
     try to drive `--continue` itself.
  3. `GET /api/sessions/<session_id>/finalize-plan?lock_id=<lock_id>`
     for the fresh plan. Response includes `mode`, `summary`,
     `script`, `commit_message` (squash mode), `co_authors` (squash
     mode), `fetch_source` (HTTPS-fallback info).
  4. Print the plain-English summary as it appeared in the portal,
     so the user confirms intent locally. Squash variant:
     > This will create a branch `<target>` from base commit
     > `<sha>` in your local source-repo checkout, then squash <N>
     > commits from <M> authors into one commit titled
     > "<subject>" with a Co-authored-by trailer for each
     > contributor. Conflicts during the squash will be left in
     > your working tree for you to resolve. Nothing will be
     > pushed.
     >
     > Commit message:
     > <heredoc-style preview of the composed message>

     Preserve variant uses the existing N-commit cherry-pick wording.
  5. Run pre-flight checks (in order; bail with a clear message on
     any failure):
     - `git rev-parse --verify refs/heads/<target>` — local branch
       collision (suggest `--force-recreate` or new name)
     - `git ls-remote --heads origin <target>` — remote branch
       collision
     - `git status --porcelain` — dirty working tree → prompt
       `Stash first? [Y/n]`; on `Y`, `git stash push -u` with a
       jamsesh-tagged message and remember to pop on clean exit
     - Record current branch (`git symbolic-ref --short HEAD`);
       warn if it has unpushed commits vs `origin/<branch>`; offer
       to `git checkout -` on successful exit
     - `git ls-remote origin` — source remote reachable
  6. Prompt `Proceed? [Y/n]` and read stdin. Bail on `n`.
  7. **Choose commit source** — local-first:
     - If the plugin's local session-checkout path exists on disk
       (from join-time state), use it: `git fetch <local-path>`.
       No auth, no portal touched.
     - Else fall back to HTTPS: mint an ephemeral fetch-only token
       via `POST /api/sessions/<id>/finalize/fetch-token`, build
       `https://x-access-token:<tok>@<portal>/git/<org>/<sess>.git`,
       `git remote add jamsesh <url>`, `git fetch jamsesh`, then
       `git remote remove jamsesh` on exit.
  8. Execute the mode-appropriate script with verbose per-step
     logging (each git operation printed before running):
     - **Squash mode**:
       - `git checkout -b <target> <base-sha>`
       - `git cherry-pick --no-commit <c1> <c2> ... <cN>`
       - `git commit --author="<runner>" -F <heredoc-of-composed-message>`
     - **Preserve mode**:
       - `git checkout -b <target> <base-sha>`
       - `git cherry-pick <c1> <c2> ... <cN>`
  9. On a conflict during cherry-pick: halt the script with a clear
     message showing the offending commit + remaining commits in
     the sequence + the exact resolution command (`git cherry-pick
     --continue` after the user fixes conflicts, or `--abort`).
     The user's own Claude Code mediates resolution with full
     project context. Re-invoking `jamsesh finalize-run <plan-id>`
     re-enters at step 2 and reports current state — it never
     drives `--continue` itself.
 10. On clean completion: print next-step instructions:
     > Branch `<name>` is ready. Push when you're ready:
     >   `git push origin <name>`
     > Then mark the session shipped in the portal.
 11. Optional: `--print-script` flag on `finalize-run` prints the
     raw shell script (mode-appropriate) the binary is about to
     execute for power-user inspection.

**Safety semantics**:

- The command runs in the user's CURRENT working directory (assumed
  to be a source-repo checkout). If the directory isn't a git repo,
  fail fast with a clear message.
- All four pre-flight checks (above) run before any state-mutating
  git command. Failures bail without touching the working tree.
- The temporary `jamsesh` remote (HTTPS-fallback path only) is
  removed on exit — clean success, clean failure, or signal — so
  the user's `git remote -v` doesn't accumulate entries.
- The ephemeral fetch token has a ~5 min TTL on the portal side; even
  if cleanup misses the remote, the credential expires on its own.

**Headless considerations**: `finalize-run` is interactive (Y/n
prompt) by default; add `--yes` for non-interactive use.

Does NOT cover the curation view (`portal-ui-curation-view`). Does NOT
cover the plan-generation backend (`plan-generation`). Does NOT cover
the "Mark as shipped" transition — that's a button in the portal UI;
this command prints next-step instructions pointing the user back.

## Epic context

- Parent epic: `epic-finalize-flow`
- Position in epic: the local-execution layer of the cross-component
  slice. Depends on plan-generation for the plan API; on
  cc-plugin-binary-foundation for the subcommand router, portal
  client, and JSON IO scaffold.

## Foundation references

- `docs/ARCHITECTURE.md` — Reconciliation (local) section (the
  canonical cherry-pick shell flow)
- `docs/UX.md` — Flow: finalizing (steps 4-6 are this command)
- `docs/SPEC.md` — Local client (slash command subcommands)

## Inherited epic design decisions

- **`jamsesh finalize-run` is interactive by default**: plain-English
  summary echo + Y/n prompt + verbose per-step logging.
- **Conflicts halt the script cleanly**: user resolves with their own
  Claude Code, then resumes; no auto-continue.
- **`--local` mode for headless users**: fetches and prints, no
  browser.
- **`--print-script` for power users**: dumps the raw shell for
  inspection before running.
- **Mark-shipped is left to the portal UI**: this command prints
  next-step instructions; doesn't transition status itself.
- **Conflict-resume model**: re-invokable command detects mid-pick
  via `git status`. No `finalize-resume` subcommand, no state file
  — `jamsesh finalize-run <plan-id>` checks for `CHERRY_PICK_HEAD`
  on every invocation and reports state if present. User drives
  `git cherry-pick --continue` / `--abort` themselves with their own
  Claude Code mediating.
- **Commit-source strategy**: local-first (filesystem path to user's
  session checkout, tracked in plugin state from join time) with
  HTTPS fallback (ephemeral fetch-only token embedded in remote URL,
  ~5 min TTL, vended by `POST /finalize/fetch-token`).
- **Linearized merge handling**: the plan's commit list is already
  linearized server-side — the binary just consumes it. The
  cherry-pick / cherry-pick `--no-commit` list never contains merge
  commits.
- **Finalization mode**: the binary handles both. Squash mode runs
  `cherry-pick --no-commit <c1>...<cN>` + `git commit --author=...
  -F <heredoc>` with the composed message from the plan. Preserve
  mode runs `cherry-pick <c1>...<cN>`. Mode comes from the plan
  response; the binary doesn't carry mode state of its own.
- **Pre-flight checks** (in order): target-branch collision (local
  + source remote), dirty working tree (with stash prompt), current
  branch awareness (with restoration), source remote reachable.
  All run before any state-mutating git command.

## Design decisions

- **Package**: `cmd/jamsesh/finalizecmd/` — sibling of `sessioncmd/` and
  `hooks/`. Two exported constructors `FinalizeCommand()` and
  `FinalizeRunCommand()` register under the root binary in
  `cmd/jamsesh/main.go`. The package is internal to the binary; no
  importers outside `cmd/jamsesh/...`.
- **Git driver is a function variable, not an interface** — mirror the
  `sessioncmd` precedent (`runGit`, `runGitOutput`):
  ```go
  var runGit       = func(args ...string) error { ... }
  var runGitOutput = func(args ...string) (string, error) { ... }
  ```
  Tests override these package-level vars instead of plumbing an
  interface; consistent with the codebase. A second helper
  `runGitCwd(cwd string, args ...string) error` is added for the
  conflict-detection check that runs in the user's cwd before any state
  is mutated (default `cwd = "."`, but injectable in tests via a third
  var).
- **Real-git integration tests** — for the script-execution path, the
  test creates a real temp git repo (`git init`, a couple of commits)
  and runs the binary against it. The function-variable override is
  used only for the *outbound* HTTPS-fallback path (we don't want tests
  to actually hit a portal smart-HTTP endpoint) and for the
  pre-flight-failure tests that simulate dirty WT / branch collision.
  Real git is the source of truth for `CHERRY_PICK_HEAD` detection,
  conflict halt semantics, and exit codes — mocking those would be
  cargo-culting.
- **Interactive prompt pattern** — match the in-tree precedent. The
  closest analogue is the `auth/browser.go` "press Enter" pattern and
  the hook IO `WithIO` context-key plumbing. For finalize-run we use a
  simple package-level `var stdin io.Reader = os.Stdin` so tests can
  inject a `strings.NewReader("y\n")`. `--yes` short-circuits the
  prompt before reading. The prompt helper is one file:
  `cmd/jamsesh/finalizecmd/prompt.go` with `confirm(prompt string,
  defaultYes bool) (bool, error)`.
- **The "script" is built in Go, not from the server's `script` field**.
  The server's `script` field is for `--print-script` and for
  human-eyeball verification only. The binary builds the same shell
  semantics by composing `runGit(...)` calls — this keeps the binary
  in control of stdout flushing, error trapping, and the conflict-halt
  return path (a shell sub-process running the server-supplied script
  would lose typed access to exit codes). The server-supplied `script`
  is rendered verbatim by `--print-script` so the user can audit what
  the binary *will* do, but the binary does not `bash -c` it.
- **Mode branching is a sealed switch on `plan.Mode`** — `squash`
  composes `cherry-pick --no-commit ...` then `git commit --author=... -F -`
  (heredoc via `cmd.Stdin = strings.Reader`); `preserve` composes
  `cherry-pick c1 c2 ... cN`. Any unknown mode value is a hard error
  with the offending value echoed.
- **Heredoc safety**: the squash commit message is passed via
  `git commit --author=<author> -F -` with stdin wired to a
  `strings.NewReader(plan.CommitMessage)`. This avoids ALL shell quoting
  / heredoc terminator collision risk that a `bash -c` path would have.
  `--print-script` renders the heredoc form (`<<'JAMSESH_EOF'` with
  embedded-terminator guard) for the user's eye only.
- **Mid-pick detection**: `git rev-parse --git-path CHERRY_PICK_HEAD`
  → check the printed path for existence. This is the official git way
  to detect mid-cherry-pick state and works inside worktrees. If
  present, fetch the plan (to enumerate remaining commits in the
  sequence vs. the CHERRY_PICK_HEAD sha) and print the resume hint.
  Plan fetch failures degrade gracefully: we still print the offending
  commit and the generic resume command.
- **Pre-flight check ordering** — strictly:
  1. Is cwd a git repo? (`git rev-parse --is-inside-work-tree`) — fail
     fast.
  2. Is there a mid-pick? — detect and bail with resume hint.
  3. Target branch collision (local): `git rev-parse --verify
     refs/heads/<target>`. Exit-zero means it exists → collision.
  4. Target branch collision (remote): `git ls-remote --heads origin
     <target>` returning a line means collision.
  5. Dirty WT: `git status --porcelain` non-empty → prompt
     `Stash first? [Y/n]`. On Y, `git stash push -u -m "jamsesh
     finalize-run <plan-id>"`. Record `stashed=true` so we pop on
     clean exit.
  6. Current branch awareness: `git symbolic-ref --short HEAD`. Record
     so we can offer `git checkout -` on clean exit. Compare against
     `origin/<branch>` via `git rev-list --left-right --count
     origin/<branch>...HEAD` — print a warning if the right-side
     count is > 0 (unpushed commits exist), but don't bail.
  7. Source remote reachable: `git ls-remote origin` — if this fails,
     bail (we'll need it for the HTTPS-fallback path even if we
     end up using local-first, since the user's `origin` IS the
     ultimate destination after the finalize).
- **Cleanup discipline** — the temporary `jamsesh` remote, the stash
  (if we created it), and the original-branch checkout are all unwound
  via a single `defer` registered in execution order before any
  mutating call. The defer list is a slice of `func() error` so we
  always run later-registered cleanups first (LIFO), like a real
  defer stack. Cleanup also runs on SIGINT — we listen on
  `signal.NotifyContext` (already done at the root in `main.go` —
  ctx propagates to us; we also catch `<-ctx.Done()` mid-execution
  and bail through the cleanup path before re-raising the cancel).
  We do NOT install our own signal handler; we trust the root
  context. The ephemeral fetch token's ~5min TTL is our backstop if
  cleanup is killed with `kill -9`.
- **Verbose per-step logging**: every git invocation in the
  execution phase prints `+ git <args>` to stdout BEFORE running,
  flushed. This mirrors `set -x` semantics so the user sees in real
  time what just happened. Pre-flight checks print only on failure
  (we don't want to log `git rev-parse` chatter on success).
- **Browser-open**: `finalize` (no flag) uses the same
  `defaultOpenURL` pattern as `cmd/jamsesh/auth/browser.go` —
  `xdg-open` / `open` / `rundll32` by GOOS; falls back to printing
  the URL on failure (graceful degrade). Factor out a tiny shared
  helper in `cmd/jamsesh/finalizecmd/browseropen.go` rather than
  importing from `auth/` (cross-package dependency direction is
  awkward — both finalizecmd and auth want it; the right move is to
  promote it to a tiny `cmd/jamsesh/internal/osopen` package). For
  this feature, however, we'll inline a private copy (10 lines) to
  keep the blast radius small; promotion to a shared package is a
  follow-up cleanup item if a third consumer appears.

## Implementation Units

### Unit 1: `finalize` subcommand (browser-opener + --local)

**File**: `cmd/jamsesh/finalizecmd/finalize.go`
**Story**: `epic-finalize-flow-plugin-finalize-command-finalize-and-finalize-run-flow`

```go
package finalizecmd

import (
    "context"
    "github.com/urfave/cli/v3"
)

func FinalizeCommand() *cli.Command {
    return &cli.Command{
        Name:  "finalize",
        Usage: "Open the portal's finalize view (default) or fetch the plan locally (--local)",
        Flags: []cli.Flag{
            &cli.BoolFlag{Name: "local", Usage: "Fetch and print the plan to stdout instead of opening a browser"},
        },
        Action: finalizeAction,
    }
}

func finalizeAction(ctx context.Context, cmd *cli.Command) error
```

Behavior:
- Resolve session via `sessioncmd.ResolveSession` (export the existing
  unexported helper, or duplicate the 30-line mapping — preference:
  export). Resolve org + portal URL the same way `fork.go` does.
- **Default**: build `<portalURL>/sessions/<sessionID>/finalize` and
  open via the platform-appropriate command; print the URL to stdout
  so headless users always have a copyable string even if the open
  succeeded.
- **`--local`**: `GET /api/sessions/<id>/finalize-plan` *without* a
  `lock_id` — server returns 409 if no lock is held. For headless
  users we accept that error and surface it to the user with the
  hint "open the portal first to start a finalize session". When
  the call succeeds, pretty-print the summary + the script body to
  stdout. Same printing path as `finalize-run --print-script`,
  factored into a shared `printPlan(w io.Writer, plan)` helper.

### Unit 2: `finalize-run` subcommand (the workhorse)

**File**: `cmd/jamsesh/finalizecmd/finalizerun.go`
**Story**: `epic-finalize-flow-plugin-finalize-command-finalize-and-finalize-run-flow`

```go
func FinalizeRunCommand() *cli.Command {
    return &cli.Command{
        Name:      "finalize-run",
        Usage:     "Execute a finalize plan in the current working directory",
        ArgsUsage: "<plan-id>",
        Flags: []cli.Flag{
            &cli.BoolFlag{Name: "yes", Aliases: []string{"y"}, Usage: "Skip interactive confirmation"},
            &cli.BoolFlag{Name: "print-script", Usage: "Print the raw shell script instead of executing"},
        },
        Action: finalizeRunAction,
    }
}

// 11-step flow per brief.
func finalizeRunAction(ctx context.Context, cmd *cli.Command) error
```

The action body composes the named helpers below. Each step prints a
short status line so the user can follow along. Errors from any
helper are surfaced with the helper's named context.

### Unit 3: Plan ID parsing + plan fetch

**File**: `cmd/jamsesh/finalizecmd/plan.go`
**Story**: `epic-finalize-flow-plugin-finalize-command-finalize-and-finalize-run-flow`

```go
type planID struct {
    SessionID string
    LockID    string
}

func parsePlanID(s string) (planID, error) // "<session>:<lock>"; both must be non-empty

// Plan mirrors openapi.PlanResponse but lives in this package so we
// don't take a hard dep on a not-yet-generated openapi type. When
// plan-generation lands its openapi schemas, swap this to a re-export.
type Plan struct {
    PlanID        string          `json:"plan_id"`
    Mode          string          `json:"mode"`           // "squash" | "preserve"
    Summary       string          `json:"summary"`
    Script        string          `json:"script"`
    CommitMessage string          `json:"commit_message,omitempty"` // squash only
    CoAuthors     []CoAuthor      `json:"co_authors,omitempty"`     // squash only
    BaseSHA       string          `json:"base_sha"`
    TargetBranch  string          `json:"target_branch"`
    Commits       []PlanCommit    `json:"commits"`                  // ordered cherry-pick list
    FetchSource   FetchSource     `json:"fetch_source"`
    LockStatus    LockStatus      `json:"lock_status"`
}

type PlanCommit struct {
    SHA     string `json:"sha"`
    Author  string `json:"author"`
    Subject string `json:"subject"`
}
type CoAuthor    struct{ Name, Email, AccountID string }
type FetchSource struct {
    Kind            string `json:"kind"`             // "local" | "https"
    Path            string `json:"path,omitempty"`
    RemoteURL       string `json:"remote_url,omitempty"`
    TokenExpiresAt  string `json:"token_expires_at,omitempty"`
}
type LockStatus struct {
    HeldBy     string `json:"held_by"`
    ExpiresAt  string `json:"expires_at"`
    IsCaller   bool   `json:"is_caller"`
}

func fetchPlan(ctx context.Context, pc *portalclient.Client, p planID) (*Plan, error)
//     GET /api/sessions/<session_id>/finalize-plan?lock_id=<lock_id>
```

### Unit 4: Mid-pick detection

**File**: `cmd/jamsesh/finalizecmd/midpick.go`
**Story**: `epic-finalize-flow-plugin-finalize-command-finalize-and-finalize-run-flow`

```go
// detectMidPick returns the SHA from CHERRY_PICK_HEAD if the cwd is mid
// cherry-pick, else "".
func detectMidPick() (string, error)

// reportMidPick prints the "you have a conflict mid-flight" message,
// listing the offending commit + remaining commits in the plan's
// sequence (computed by finding offendingSHA in plan.Commits) + the
// exact resume command. Returns nil — the caller exits cleanly after.
func reportMidPick(w io.Writer, offendingSHA string, plan *Plan) error
```

Detection: `git rev-parse --git-path CHERRY_PICK_HEAD` then `os.Stat`
the result. If the file exists, read it for the SHA.

### Unit 5: Pre-flight checks

**File**: `cmd/jamsesh/finalizecmd/preflight.go`
**Story**: `epic-finalize-flow-plugin-finalize-command-finalize-and-finalize-run-flow`

```go
type preflightResult struct {
    OriginalBranch string  // for "git checkout -" on clean exit
    Stashed        bool    // true if we created a stash
    StashMessage   string  // the message we used; for pop targeting
}

func runPreflight(ctx context.Context, plan *Plan, prompt confirmFn) (*preflightResult, error)
```

Implements the 7-step ordered check list under Design decisions. The
`confirmFn` type is `func(prompt string, defaultYes bool) (bool, error)`
so the dirty-WT stash prompt is testable without stdin plumbing.

### Unit 6: Confirmation prompt

**File**: `cmd/jamsesh/finalizecmd/prompt.go`
**Story**: `epic-finalize-flow-plugin-finalize-command-finalize-and-finalize-run-flow`

```go
// Stdin is overridable in tests.
var stdin io.Reader = os.Stdin

// confirm reads a line; "" + defaultYes=true → true; "y" / "yes" → true;
// "n" / "no" → false. Other inputs re-prompt up to 3 times.
func confirm(out io.Writer, prompt string, defaultYes bool) (bool, error)

// confirmFn is the function type pre-flight + main flow accept.
type confirmFn func(prompt string, defaultYes bool) (bool, error)
```

### Unit 7: Script executor (mode-branching)

**File**: `cmd/jamsesh/finalizecmd/execute.go`
**Story**: `epic-finalize-flow-plugin-finalize-command-finalize-and-finalize-run-flow`

```go
// execute runs the mode-appropriate git script with verbose per-step
// logging. Returns nil on clean completion, a conflictError on
// cherry-pick conflict (which the caller renders with a resume hint),
// or any other error wrapped.
type conflictError struct {
    OffendingSHA string
    Remaining    []string  // SHAs that haven't been picked yet
}
func (e *conflictError) Error() string { ... }

func execute(ctx context.Context, plan *Plan, out io.Writer) error
```

Body:
- `runGitVerbose(out, "checkout", "-b", plan.TargetBranch, plan.BaseSHA)`
- Switch on `plan.Mode`:
  - `"squash"`: `runGitVerbose(out, append([]string{"cherry-pick",
    "--no-commit"}, shaArgs(plan.Commits)...)...)`; on conflict, return
    conflictError. Then `runGitCommitVerbose(out, plan.CommitMessage,
    runnerIdentity)`.
  - `"preserve"`: `runGitVerbose(out, append([]string{"cherry-pick"},
    shaArgs(plan.Commits)...)...)`; on conflict, return conflictError
    (offending SHA inferred from `CHERRY_PICK_HEAD` post-failure).

`runGitVerbose` prints `+ git <args>` to `out`, flushes, then invokes
`runGit` (the package var). On non-zero exit it inspects
`CHERRY_PICK_HEAD` to decide whether to return a `*conflictError` or
generic error. `runGitCommitVerbose` pipes the commit message through
stdin to `git commit --author=<runner> -F -`.

### Unit 8: Script printer

**File**: `cmd/jamsesh/finalizecmd/script.go`
**Story**: `epic-finalize-flow-plugin-finalize-command-finalize-and-finalize-run-flow`

```go
// printScript renders the mode-appropriate shell script the binary
// would execute, as a single bash heredoc-safe block. Used by
// `finalize-run --print-script` and `finalize --local`.
func printScript(w io.Writer, plan *Plan) error
```

Output mirrors the server's `plan.Script` when present, but is composed
locally so `--print-script` works even if the server omits it.

### Unit 9: Fetch-source selection (local-first vs HTTPS fallback)

**File**: `cmd/jamsesh/finalizecmd/fetchsource.go`
**Story**: `epic-finalize-flow-plugin-finalize-command-fetch-source-selection-and-cleanup`

```go
// chooseFetchSource picks local-first if the plugin state lists a
// local session checkout path that exists on disk, else falls back
// to HTTPS. Returns the fetch source and a cleanup func registered
// with the caller's cleanup stack.
type fetchSource struct {
    Kind     string  // "local" | "https"
    URL      string  // local path or HTTPS URL
    cleanup  func() error  // remove jamsesh remote (https) or no-op (local)
}

func chooseFetchSource(ctx context.Context, pc *portalclient.Client, plan *Plan, sessionID string) (*fetchSource, error)

// performFetch runs `git fetch <kind-specific args>` with verbose logging.
func performFetch(ctx context.Context, fs *fetchSource, out io.Writer) error
```

Behavior:
- Local-first: read `${CLAUDE_PLUGIN_DATA}/sessions/<sid>/local_path`
  (a new state file written at join-time by a sibling story — for
  this feature, if the file is absent we go straight to HTTPS). When
  present and the path is a directory containing `.git` (or is a
  bare repo), use it: `git fetch <path>`. Cleanup is a no-op.
- HTTPS fallback: `POST /api/sessions/<id>/finalize/fetch-token` →
  `{token, expires_at}`. Build the URL with `url.UserPassword`
  ("x-access-token", token) mirroring the existing `buildCloneURL`
  helper in `sessioncmd/join.go`. Run `git remote add jamsesh <url>`
  → `git fetch jamsesh`. Cleanup runs `git remote remove jamsesh`
  and errors are logged but not returned (best-effort).

### Unit 10: Cleanup stack

**File**: `cmd/jamsesh/finalizecmd/cleanup.go`
**Story**: `epic-finalize-flow-plugin-finalize-command-fetch-source-selection-and-cleanup`

```go
// cleanupStack is a LIFO list of teardown funcs registered during the
// flow. Run() invokes them in reverse order, collecting errors into a
// joined error. Listen() watches ctx.Done() in a goroutine so a SIGINT
// to the binary still drains the stack before exit.
type cleanupStack struct { ... }

func newCleanupStack(ctx context.Context, out io.Writer) *cleanupStack
func (c *cleanupStack) Push(name string, fn func() error)
func (c *cleanupStack) Run() error
```

Used by the finalize-run action to register, in order:
1. Stash pop (if pre-flight stashed).
2. Original-branch restore (`git checkout -`) — only on CLEAN exit.
   On conflict we leave the user in the partial branch.
3. Temp `jamsesh` remote removal (HTTPS path only).

The stash-pop and original-branch-restore cleanups are conditional on
clean exit (the caller passes an `outcome` bit to `Run`). The remote
removal runs unconditionally so the user's `git remote -v` is always
clean.

The SIGINT path: a goroutine watches `ctx.Done()`; on cancel it calls
`Run(outcomeAborted)` and then `os.Exit(130)`. The main goroutine
also handles the cancel through its own error path; whichever fires
first wins (the cleanup funcs are idempotent — running them twice is
harmless, e.g. `git remote remove jamsesh` is `|| true`).

### Unit 11: Wiring + binary glue

**File**: `cmd/jamsesh/main.go` (edit)
**Story**: `epic-finalize-flow-plugin-finalize-command-finalize-and-finalize-run-flow`

Add to the root command's `Commands:` slice:
```go
finalizecmd.FinalizeCommand(),
finalizecmd.FinalizeRunCommand(),
```

Export `sessioncmd.ResolveSession` (rename the current
`resolveSession` to capitalized) so `finalizecmd` can call it without
duplicating the CC-session-id resolution logic.

## Story decomposition

Two stories, depends_on chain.

1. **`epic-finalize-flow-plugin-finalize-command-finalize-and-finalize-run-flow`**
   — `depends_on: []`
   - Both `finalize` and `finalize-run` subcommands (Units 1-8, 11)
   - urfave/cli wiring, plan fetch, summary print, Y/n prompt
   - Pre-flight checks (all 7) and mid-pick detection
   - Mode-aware script executor (squash + preserve)
   - Conflict halt with clear message + remaining commits
   - `--print-script` and `--local`
   - Stubs the `chooseFetchSource` helper to a placeholder that
     returns `kind: "local"` with `URL: "."` (will compile, will
     fail at fetch time — sufficient for unit tests that mock the
     fetch step out).

2. **`epic-finalize-flow-plugin-finalize-command-fetch-source-selection-and-cleanup`**
   — `depends_on: [epic-finalize-flow-plugin-finalize-command-finalize-and-finalize-run-flow]`
   - Implement `chooseFetchSource` (local-first vs HTTPS fallback,
     Unit 9)
   - Implement `cleanupStack` (Unit 10) and wire it into the
     `finalize-run` action
   - Defer-style cleanup on signals + clean/dirty exit
   - Ephemeral-credential handling: `POST /finalize/fetch-token`,
     URL injection via `url.UserPassword`, `git remote add/remove
     jamsesh` cycle
   - Replace the placeholder from story 1 with the real helper

Rationale for the split: story 1 stands alone as a vertical slice
(it executes against a co-located local checkout out of the box) and
delivers reviewable value. Story 2 is purely the "talk to the portal
to fetch when no local checkout is present" layer — it plugs into the
story-1 scaffold without changing the user-facing flow. Splitting
keeps each story under ~600 LoC and lets a reviewer evaluate the
flow shape independently of the credential-handling code.

## Testing

### Unit-level (function-variable mocking)

- `parsePlanID`: 6 cases (happy, missing colon, empty session, empty
  lock, multiple colons → split on first, whitespace)
- `detectMidPick`: temp dir with / without `CHERRY_PICK_HEAD`
- `confirm`: y/Y/yes/n/N/no/empty+defaultYes/empty+defaultNo + 3-strike
  invalid
- `printScript`: snapshot test against a canned plan (both modes)
- `chooseFetchSource`: state-file present vs absent vs path doesn't
  exist on disk → falls back to HTTPS

### Integration-level (real git in a temp repo)

The test helper `setupRepoWithCommits(t)` creates a fresh `git init`
repo with 3 commits on `main`, a `feature` branch with 2 commits, and
configures user.name/email. Then:

- **Happy path, preserve mode**: real cherry-pick of 2 commits, end
  state asserted via `git log --format=%H`.
- **Happy path, squash mode**: cherry-pick `--no-commit` x2 + commit
  with composed message; assert resulting commit subject + body +
  Co-authored-by trailers.
- **Conflict halt**: two commits that touch the same line. Assert
  conflictError, assert `CHERRY_PICK_HEAD` exists post-call, assert
  exit message names the offending SHA + the resume command.
- **Re-invocation mid-pick**: in the same temp repo (still mid-pick
  from previous test), invoke `finalize-run <plan-id>` — assert it
  reports the mid-pick state and exits without touching state.
- **Pre-flight: branch collision (local)**: pre-create
  `refs/heads/<target>`, assert clean bail with named message.
- **Pre-flight: dirty WT + stash prompt**: dirty the WT, inject
  `confirmFn` returning true, assert stash created with the
  jamsesh-tagged message + stash pop on clean exit.
- **`--yes` bypasses prompts**: assert no read from `stdin`.
- **`--print-script` doesn't execute**: assert no commits added to
  the repo.

### HTTPS-fallback (httptest)

Patterned after `cmd/jamsesh/sessioncmd/fork_test.go`. Spin up an
`httptest.NewServer`, serve the plan JSON + the fetch-token JSON,
then assert the binary built the right URL (with
`x-access-token:<token>` user info) and registered the `jamsesh`
remote. Cleanup test: assert `git remote -v` after the run has no
`jamsesh` entry, both on clean exit and on simulated cancel.

### Coverage targets

- `finalizecmd/`: ≥ 80% line coverage
- `runGit` / `runGitOutput` / `runGitCwd` paths: covered indirectly
  through the real-git integration tests
- Verbose-logging output: golden-snapshot a short flow's stdout

## Risks

- **Real-git integration tests pin a minimum git version**. Decision:
  require git ≥ 2.30 (`--git-path` is older than that). CI image
  already has 2.40+. Document the floor in the story's
  Implementation notes.
- **The `local_path` state file isn't written yet** by any existing
  feature. Story 2 reads it; if absent it falls back to HTTPS. A
  follow-up substrate item (parked, not blocking) is to teach
  `sessioncmd.JoinCommand` to write the local-checkout path on
  successful clone. This feature works without that file (HTTPS
  fallback covers everything); the local-first optimization
  activates once the parked item lands.
- **`--print-script` could drift from the server's `plan.Script`**.
  Mitigation: a contract test in the plan-generation feature
  asserts the server's rendered script equals what
  `finalizecmd.printScript` produces for the same input. Owned by
  the plan-generation feature's test plan, not this one.
- **Concurrent-finalize edge cases**: another member's lock supersedes
  ours mid-execution. The portal API returns 409 on subsequent
  PATCH but the binary at this point has already fetched the plan;
  the cherry-pick is a local operation and completes. The
  "mark-shipped" UI on the portal is where the conflict actually
  surfaces — out of scope for this feature.

## Implementation summary

<!-- Filled in by /agile-workflow:implement after stories complete. -->

## Review

<!-- Filled in by /agile-workflow:review when this feature reaches stage:review. -->

## Implementation summary

2 child stories landed (commits be4d967, 0304134). `cmd/jamsesh/finalizecmd/` owns the two subcommands (`finalize` browser-opener with --local fallback; `finalize-run <plan-id>` 11-step orchestrator), local-first vs HTTPS-fallback chooser, and the LIFO cleanup stack with SIGINT-safe teardown. 63 finalizecmd tests pass.

## Review

**Verdict**: Approve. Local-execution layer ships clean.
