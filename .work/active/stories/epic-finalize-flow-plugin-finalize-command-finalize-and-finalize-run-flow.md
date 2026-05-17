---
id: epic-finalize-flow-plugin-finalize-command-finalize-and-finalize-run-flow
kind: story
stage: review
tags: [plugin]
parent: epic-finalize-flow-plugin-finalize-command
depends_on: [epic-finalize-flow-plan-generation-plan-fetch-and-script, epic-finalize-flow-plan-generation-fetch-token-and-mark-shipped]
release_binding: null
gate_origin: null
created: 2026-05-17
updated: 2026-05-17
---

# Finalize Command — `finalize` + `finalize-run` Flow

## Scope

Build the two `jamsesh` binary subcommands (`finalize` and
`finalize-run`) and all of the local execution machinery EXCEPT the
fetch-source-selection helper (which is sibling story 2). The
finalize-run action wires every step of the 11-step flow except step
7's chooseFetchSource — that's stubbed here to a placeholder that
returns kind:"local"/URL:"." so the unit tests of every other step
work; story 2 replaces the placeholder.

## Units delivered

- `cmd/jamsesh/finalizecmd/finalize.go` — `FinalizeCommand()` (browser
  opener + `--local`)
- `cmd/jamsesh/finalizecmd/finalizerun.go` — `FinalizeRunCommand()`
  (11-step orchestration)
- `cmd/jamsesh/finalizecmd/plan.go` — `parsePlanID`, `Plan` struct,
  `fetchPlan`
- `cmd/jamsesh/finalizecmd/midpick.go` — `detectMidPick`,
  `reportMidPick`
- `cmd/jamsesh/finalizecmd/preflight.go` — `runPreflight` (7-step
  check list)
- `cmd/jamsesh/finalizecmd/prompt.go` — `confirm` + injectable
  `stdin`
- `cmd/jamsesh/finalizecmd/execute.go` — `execute`, `conflictError`,
  `runGitVerbose`, `runGitCommitVerbose`
- `cmd/jamsesh/finalizecmd/script.go` — `printScript`
- `cmd/jamsesh/finalizecmd/browseropen.go` — platform-aware URL
  opener (inline copy of `auth.defaultOpenURL`)
- `cmd/jamsesh/finalizecmd/git.go` — `runGit`, `runGitOutput`,
  `runGitCwd` package vars (test-overridable)
- `cmd/jamsesh/finalizecmd/fetchsource_stub.go` — placeholder
  `chooseFetchSource` returning local/"." (deleted by story 2)
- `cmd/jamsesh/main.go` (edit) — register `finalizecmd.FinalizeCommand()`
  and `finalizecmd.FinalizeRunCommand()`
- `cmd/jamsesh/sessioncmd/session.go` (edit) — export `ResolveSession`
  (rename from lowercase) so finalizecmd can reuse it
- Test files for each unit (real-git integration tests in a temp dir
  for execute + preflight + midpick; function-variable mocks for the
  rest)

## Acceptance Criteria

- [x] `jamsesh finalize` (no flag) opens
      `<portal>/sessions/<sid>/finalize` via the platform opener and
      prints the URL to stdout
- [x] `jamsesh finalize --local` calls
      `GET /api/sessions/<sid>/finalize-plan` and prints the plan
      summary + script body
- [x] `jamsesh finalize-run <session>:<lock>` parses the plan-id,
      fetches the plan, prints the summary, prompts `Proceed? [Y/n]`,
      runs pre-flight, then executes mode-appropriate cherry-pick
- [x] Mid-pick detection: invoking `finalize-run` when
      `CHERRY_PICK_HEAD` exists prints the offending SHA + remaining
      SHAs + resume command and exits 0 (no state mutation)
- [x] Pre-flight: branch collision (local + remote) bails with named
      messages
- [x] Pre-flight: dirty WT prompts `Stash first? [Y/n]`; on Y, runs
      `git stash push -u -m "jamsesh finalize-run <plan-id>"` and pops
      on clean exit (NOT on conflict exit)
- [x] Squash mode: composes `cherry-pick --no-commit` over N commits
      then `git commit --author=<runner> -F -` with the plan's
      composed message via stdin; resulting commit's subject + body +
      Co-authored-by trailers match the plan
- [x] Preserve mode: composes per-commit `git cherry-pick` over N
      commits; result is N new commits with original authorship
- [x] On conflict during cherry-pick: prints offending SHA + remaining
      SHAs + `git cherry-pick --continue` / `--abort` hint and exits
      non-zero; the partial state is left for the user
- [x] `--yes` bypasses both the proceed prompt and the stash prompt
- [x] `--print-script` prints the shell script (heredoc form for
      squash mode) and does NOT touch the repo
- [x] Verbose per-step logging: every state-mutating git invocation
      prints `+ git <args>` to stdout BEFORE running, flushed
- [x] `go build ./...` clean
- [x] `go test ./cmd/jamsesh/finalizecmd/...` passes
- [x] `go test ./cmd/jamsesh/...` passes (no regression in sibling
      packages from the `ResolveSession` rename)

## Notes

- The `chooseFetchSource` helper is stubbed in this story
  (`fetchsource_stub.go`); the real implementation lands in story 2.
  Tests that exercise the full flow either inject a fake `runGit`
  that doesn't actually run `git fetch`, or use the stub which
  fetches from "." (a valid no-op against a real local repo).
- Heredoc safety: the squash commit message is passed via `git
  commit -F -` with `cmd.Stdin = strings.NewReader(plan.CommitMessage)`.
  No shell quoting, no heredoc terminator collision risk.
- `runGit` / `runGitOutput` / `runGitCwd` are package-level function
  vars (mirroring `sessioncmd`). Tests override them by reassignment
  via `t.Cleanup`.
- The `confirm` helper reads from an injectable package-level `stdin
  io.Reader` (default `os.Stdin`); tests use a `strings.Reader`.
- Browser-open uses an inlined copy of `auth.defaultOpenURL` — 10
  lines, no cross-package coupling. A future cleanup item parks the
  shared helper under `cmd/jamsesh/internal/osopen` if a third
  consumer appears.
- Min git version: 2.30 (for `--git-path`). CI image has 2.40+.

## Implementation notes

Landed the full `finalizecmd` package under `cmd/jamsesh/finalizecmd/`:

- `finalize.go` — `FinalizeCommand()` with default browser-open and
  `--local` plan-print paths. URL is always echoed to stdout for
  copyability.
- `finalizerun.go` — `FinalizeRunCommand()` orchestrating the full
  11-step flow: parse plan-id, mid-pick short-circuit, plan fetch,
  summary print, `--print-script` short-circuit, pre-flight, proceed
  prompt, fetch-source choose, fetch, runner identity, execute,
  conflict-or-clean exit handling.
- `plan.go` — `parsePlanID`, the local `Plan`/`CoAuthor`/`PlanCommit`/
  `FetchSource`/`LockStatus` mirrors of the openapi types, `fetchPlan`,
  and a `selectedSHAs()` convenience.
- `midpick.go` — `detectMidPick` (uses `git rev-parse --show-toplevel`
  + `--git-path CHERRY_PICK_HEAD` then anchors the relative path
  against the absolute toplevel so `os.Stat` works regardless of the
  caller's process cwd), `reportMidPick`, `remainingAfter` (prefix-
  tolerant so short SHAs from CHERRY_PICK_HEAD match full plan SHAs).
- `preflight.go` — the 7-step ordered check list. Step 2 (mid-pick)
  is performed by the caller before reaching preflight, per the
  feature design.
- `prompt.go` — package-level `var stdin io.Reader = os.Stdin`,
  `confirm`, and the `confirmFn` type passed to preflight.
- `execute.go` — `execute`, `conflictError` (with `OffendingSHA`,
  `Remaining`, and `Underlying` for `errors.As`/`Unwrap`),
  `runGitVerbose` (sh -x style `+ git …` lines, best-effort Sync),
  `classifyCherryPickError` (re-detects mid-pick post-failure to
  decide conflict vs generic error), `renderConflict`,
  `resolveRunnerIdentity`.
- `script.go` — `printScript` builds the script locally rather than
  echoing `plan.Script`; squash mode uses `pickHeredocTerminator`
  with a `JAMSESH_EOF` base + numeric suffix fallback for
  terminator-collision safety. `printPlanSummary` renders the
  proceed-prompt header.
- `browseropen.go` — inlined platform opener (`xdg-open` / `open` /
  `rundll32`) plus `openURL` package var for tests.
- `git.go` — `runGit`, `runGitOutput`, `runGitCwd`,
  `runGitOutputCwd`, `runGitWithStdin` (used for `git commit -F -`,
  no shell-quoting risk).
- `fetchsource_stub.go` — story-1 placeholder returning
  `{Kind: "local", URL: "."}` + no-op cleanup. Story 2 replaces.

Sibling-package change:

- `cmd/jamsesh/sessioncmd/session.go` — `resolveSession` renamed to
  `ResolveSession`. Updated callers in `fork.go`, `mode.go`,
  `status.go` (3 lines). No external consumers to worry about
  (`hooks/sessionstart.go` had its own local `resolveSessionID` copy).
- `cmd/jamsesh/main.go` — registered both new subcommands.

Tests (45 total: 15 real-git integration + 30 unit-style):

- Real-git integration (run against `git init` tempdirs):
  - `TestExecute_preserveHappy` / `_squashHappy` — verify resulting
    `git log --format=%s` and squash-commit author identity.
  - `TestExecute_conflictReturnsConflictError` — sets up a
    3-way-conflicting setup, asserts `*conflictError` is returned
    and `CHERRY_PICK_HEAD` persists post-call.
  - `TestExecute_unknownModeErrors`, `_emptyPlanErrors`,
    `TestResolveRunnerIdentity`.
  - `TestDetectMidPick_noPick`, `_notARepo`, `_inProgress`.
  - `TestPreflight_notARepoErrors`, `_localBranchCollision`,
    `_dirtyWT_stashYes` (verifies stash created + correct message),
    `_dirtyWT_stashNoBails`, `_originReachableHappy`,
    `_originUnreachableErrors`.
- Unit:
  - `TestParsePlanID` (8 subtests).
  - `TestConfirm` (9 subtests) + `_invalidThenValid` + `_threeStrikeInvalid`.
  - `TestPrintScript_preserve`, `_squash`, `_unknownMode`,
    `TestPickHeredocTerminator_unique`, `TestPrintPlanSummary`.
  - `TestRenderConflict`, `TestReportMidPick_withPlan`/`_noPlan`,
    `TestRemainingAfter` (incl. prefix-match for short-SHA vs
    full-SHA case).

Test infrastructure: `testhelpers_test.go` provides
`gitInTempRepo(t)`, `commit()`, and `pinGitToCwd(t, dir)` which
swaps all 5 git-runner package vars to subprocess-pinned variants
restored via `t.Cleanup`.

Verification:

- `go build ./...` clean.
- `go test ./cmd/jamsesh/...` green (incl. sibling packages, verifying
  the `ResolveSession` rename didn't regress fork/mode/status).
- `go test ./...` green across the whole repo.

Deviations from the spec:

- `detectMidPick` was upgraded from "just call `git rev-parse
  --git-path CHERRY_PICK_HEAD` and stat" to "pair with
  `git rev-parse --show-toplevel` and anchor the relative path
  against the absolute toplevel". The naive form works in production
  (binary's cwd IS the repo) but failed in tests where the binary's
  cwd ≠ the pinned git cwd. Anchoring at the toplevel is correct
  for both cases.
- `runGitOutputCwd` was added alongside the spec'd `runGitCwd`. The
  detection path needs *output*, not stream-inherit, so adding the
  capturing twin keeps the test-override discipline consistent.
- The stub `chooseFetchSource` returns
  `{Kind: "local", URL: ".", cleanup: no-op}`. `performFetch` runs
  `git fetch .` against it. Story 2's real implementation replaces
  both with a state-file lookup + HTTPS fallback path.

## Review

<!-- Filled in by /agile-workflow:review when this story reaches stage:review. -->
