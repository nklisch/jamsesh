---
id: epic-finalize-flow-plugin-finalize-command-finalize-and-finalize-run-flow
kind: story
stage: implementing
tags: [plugin]
parent: epic-finalize-flow-plugin-finalize-command
depends_on: []
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

- [ ] `jamsesh finalize` (no flag) opens
      `<portal>/sessions/<sid>/finalize` via the platform opener and
      prints the URL to stdout
- [ ] `jamsesh finalize --local` calls
      `GET /api/sessions/<sid>/finalize-plan` and prints the plan
      summary + script body
- [ ] `jamsesh finalize-run <session>:<lock>` parses the plan-id,
      fetches the plan, prints the summary, prompts `Proceed? [Y/n]`,
      runs pre-flight, then executes mode-appropriate cherry-pick
- [ ] Mid-pick detection: invoking `finalize-run` when
      `CHERRY_PICK_HEAD` exists prints the offending SHA + remaining
      SHAs + resume command and exits 0 (no state mutation)
- [ ] Pre-flight: branch collision (local + remote) bails with named
      messages
- [ ] Pre-flight: dirty WT prompts `Stash first? [Y/n]`; on Y, runs
      `git stash push -u -m "jamsesh finalize-run <plan-id>"` and pops
      on clean exit (NOT on conflict exit)
- [ ] Squash mode: composes `cherry-pick --no-commit` over N commits
      then `git commit --author=<runner> -F -` with the plan's
      composed message via stdin; resulting commit's subject + body +
      Co-authored-by trailers match the plan
- [ ] Preserve mode: composes per-commit `git cherry-pick` over N
      commits; result is N new commits with original authorship
- [ ] On conflict during cherry-pick: prints offending SHA + remaining
      SHAs + `git cherry-pick --continue` / `--abort` hint and exits
      non-zero; the partial state is left for the user
- [ ] `--yes` bypasses both the proceed prompt and the stash prompt
- [ ] `--print-script` prints the shell script (heredoc form for
      squash mode) and does NOT touch the repo
- [ ] Verbose per-step logging: every state-mutating git invocation
      prints `+ git <args>` to stdout BEFORE running, flushed
- [ ] `go build ./...` clean
- [ ] `go test ./cmd/jamsesh/finalizecmd/...` passes
- [ ] `go test ./cmd/jamsesh/...` passes (no regression in sibling
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

<!-- Filled in by /agile-workflow:implement after work completes. -->

## Review

<!-- Filled in by /agile-workflow:review when this story reaches stage:review. -->
