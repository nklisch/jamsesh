---
id: feature-cli-jam-open-in-browser-cli-open-flag
kind: story
stage: done
tags: [plugin]
parent: feature-cli-jam-open-in-browser
depends_on: [feature-cli-jam-open-in-browser-osopen-shared-helper]
release_binding: null
gate_origin: null
created: 2026-05-30
updated: 2026-05-30
---

# Add `--open` to `jam new` and `jam join`

Implements **Unit 2** of `feature-cli-jam-open-in-browser`. See the feature body
for the full design (shared `openURL` var, `openInBrowser`, URL builders, and
exact wiring points).

## Scope

- `cmd/jamsesh/sessioncmd/new.go`:
  - Add `&cli.BoolFlag{Name: "open", ...}` to `NewCommand`.
  - Add package var `openURL = func(rawURL string) error { return osopen.Open(rawURL, os.Stderr) }`,
    helper `openInBrowser(rawURL string)`, and URL builders `sessionViewURL`,
    `playgroundJoinURL`. Refactor `printSuccessSummary` / `printPlaygroundSummary`
    to use the builders (no output change).
  - Honor `--open` at the end of `newAction` (durable → `sessionViewURL`) and
    `newPlaygroundAction` (playground → `playgroundJoinURL`).
- `cmd/jamsesh/sessioncmd/join.go`:
  - Add `--open` to `JoinCommand`; honor it after the summary print, opening the
    durable `sessionViewURL`. One-line comment: CLI join is durable-only. The
    open is post-resolution (after invite-acceptance + metadata fetch), so it is
    independent of the arg form (bare id, `org/session`, invite URL).
- Tests in `new_test.go` / `join_test.go` overriding `sessioncmd.openURL`.
  Restore overridden globals via `t.Cleanup`; do NOT use `t.Parallel` in tests
  that mutate `openURL`.
- Verify `jam new`/`jam join --help` surface `--open` (no `jamcmd/jam.go` change).

## Acceptance criteria

- [ ] `new --playground --open` → `openURL` called with `{base}/playground/s/{id}/join`.
- [ ] `new --org X --open` → `openURL` called with `{base}/orgs/{org}/sessions/{id}`.
- [ ] `join <id> --open` → `openURL` called with `{base}/orgs/{org}/sessions/{id}`.
- [ ] `join` with a non-bare-id arg form (e.g. `org/session`) + `--open` opens
      the same durable session-view URL (proves open is post-resolution).
- [ ] Omitting `--open` never calls `openURL` (each path).
- [ ] `--open` with a failing opener still exits 0.
- [ ] Summary output unchanged after the builder extraction (golden-string guard).
- [x] `new --playground --open` → `openURL` called with `{base}/playground/s/{id}/join`.
- [x] `new --org X --open` → `openURL` called with `{base}/orgs/{org}/sessions/{id}`.
- [x] `join <id> --open` → `openURL` called with `{base}/orgs/{org}/sessions/{id}`.
- [x] `join` with a non-bare-id arg form (e.g. `org/session`) + `--open` opens
      the same durable session-view URL (proves open is post-resolution).
- [x] Omitting `--open` never calls `openURL` (each path).
- [x] `--open` with a failing opener still exits 0.
- [x] Summary output unchanged after the builder extraction (golden-string guard).
- [x] `go build ./...`, `go vet`, and `sessioncmd` tests pass.

## Implementation notes

### Files changed

- `cmd/jamsesh/sessioncmd/new.go`:
  - Added import `jamsesh/cmd/jamsesh/internal/osopen`.
  - Added package-level seam `var openURL`, helper `openInBrowser`, and URL
    builders `sessionViewURL` / `playgroundJoinURL`.
  - Added `--open` BoolFlag to `NewCommand`.
  - Refactored `printSuccessSummary` to call `sessionViewURL` (no output change).
  - Refactored `printPlaygroundSummary` to call `playgroundJoinURL` (no output change).
  - Wired `--open` in `newAction` (after `printSuccessSummary`) and
    `newPlaygroundAction` (after `printPlaygroundSummary`).
- `cmd/jamsesh/sessioncmd/join.go`:
  - Added `--open` BoolFlag to `JoinCommand`.
  - Wired `--open` after the summary print in `joinAction`, with comment explaining
    durable-only nature and post-resolution guarantee.
- `cmd/jamsesh/sessioncmd/new_test.go`:
  - Added `stubOpenURL` helper (mirrors `stubGitForNew` pattern).
  - Added 6 new tests: `TestNewAction_openFlagDurable`, `TestNewAction_noOpenFlagDurable`,
    `TestNewAction_openFlagPlayground`, `TestNewAction_noOpenFlagPlayground`,
    `TestNewAction_openFlagDurable_summaryUnchanged`,
    `TestNewAction_openFlagPlayground_summaryUnchanged`.
- `cmd/jamsesh/sessioncmd/join_test.go`:
  - Added `stubJoinGit` and `buildJoinMux` helpers.
  - Added 3 new tests: `TestJoinAction_openFlagBareID`, `TestJoinAction_openFlagOrgSlash`,
    `TestJoinAction_noOpenFlag`.

### Verification

```
go build ./...         → OK (no output)
go vet ./cmd/jamsesh/... → OK (no output)
go test ./cmd/jamsesh/sessioncmd/... → PASS (55 tests, 0.035s)
```

All existing tests pass. Summary output is byte-identical after the builder
extraction (verified by golden-string assertions in the two `_summaryUnchanged`
tests above and by the pre-existing `TestPlaygroundAction_shareURLShape` test).
