---
id: feature-cli-jam-open-in-browser-cli-open-flag
kind: story
stage: implementing
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
    durable `sessionViewURL`. One-line comment: CLI join is durable-only.
- Tests in `new_test.go` / `join_test.go` overriding `sessioncmd.openURL`.
- Verify `jam new`/`jam join --help` surface `--open` (no `jamcmd/jam.go` change).

## Acceptance criteria

- [ ] `new --playground --open` → `openURL` called with `{base}/playground/s/{id}/join`.
- [ ] `new --org X --open` → `openURL` called with `{base}/orgs/{org}/sessions/{id}`.
- [ ] `join <id> --open` → `openURL` called with `{base}/orgs/{org}/sessions/{id}`.
- [ ] Omitting `--open` never calls `openURL` (each path).
- [ ] `--open` with a failing opener still exits 0.
- [ ] Summary output unchanged after the builder extraction (golden-string guard).
- [ ] `go build ./...`, `go vet`, and `sessioncmd` tests pass.
