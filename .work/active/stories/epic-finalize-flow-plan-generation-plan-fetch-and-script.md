---
id: epic-finalize-flow-plan-generation-plan-fetch-and-script
kind: story
stage: review
tags: [portal]
parent: epic-finalize-flow-plan-generation
depends_on: [epic-finalize-flow-plan-generation-locks-schema-and-rest]
release_binding: null
gate_origin: null
created: 2026-05-17
updated: 2026-05-17
---

# Finalize Plan — Fetch and Script Composition

## Scope

Land the read-side of the finalize flow: `GET .../finalize-plan`
endpoint, the deterministic squash-message composer, the bash
script-body builders (squash + preserve), and the `FirstParentLeafCommits`
helper the curation UI uses to populate its default selection from
`draft`.

Plan generation is the moment the curated SHAs become a concrete,
copy-pasteable shell script with a fully-formed commit message and
co-author trailer list. Determinism matters: the same lock state +
same bare repo must always produce the same script bytes and the
same composed message, so the plugin and UI render identical
previews to the human before execution.

## Units delivered

- **`internal/portal/finalize/message.go`** —
  `func ComposeSquashMessage(sessionGoal, userOverrideSubject string,
  commits []*object.Commit) (subject, body string, coAuthors
  []CoAuthor)`. Subject: if `userOverrideSubject` non-empty use its
  first line truncated to 72 chars, else use `sessionGoal` truncated
  at word boundary to 72 chars with `…` suffix when truncated.
  Body: blank line then `- <subject>` per commit in selection order,
  where `<subject>` is the first line of each commit's message
  stripped of trailing trailer lines (Jam-* / Co-authored-by /
  Resolves-Conflict / etc.). Footer: blank line then one
  `Co-authored-by: <Display Name> <email>` per distinct author
  (dedup by `strings.ToLower(email)`, preserve first-appearance
  casing for the rendered trailer, preserve first-appearance order).
- **`internal/portal/finalize/message_test.go`** — golden test
  using `testdata/squash_message.golden.txt`. Cases: 1-author
  3-commits, 3-author 3-commits, case-variant emails dedup
  (`Alice@x` and `alice@x` produce ONE trailer), user-override
  subject (only first line used), session-goal truncation at
  72-char word boundary, empty selection (returns subject only,
  empty body, empty trailers).
- **`internal/portal/finalize/script.go`** —
  - `type ScriptInput struct { Mode, TargetBranch, BaseSHA,
    SquashMessageBody string; SelectedSHAs []string }`
  - `func BuildScript(in ScriptInput) string` dispatches.
  - `buildSquashScript(in)` template per feature design with literal
    placeholders `$JAMSESH_FETCH_REMOTE`, `$JAMSESH_RUNNER_NAME`,
    `$JAMSESH_RUNNER_EMAIL` the plugin substitutes.
    `set -euo pipefail` prologue; verbose `echo "==> ..."` before
    each git command; heredoc-delimited commit message via
    `JAMSESH_MSG` sentinel; chains `git commit --author=... -F -
    <<'JAMSESH_MSG' ... JAMSESH_MSG`.
  - `buildPreserveScript(in)` — same prologue, per-commit `git
    cherry-pick <sha>` instead of `--no-commit + git commit`; no
    squash message. Conflicts on any single cherry-pick halt the
    script (set -e); the plugin's resume logic kicks in on
    re-invocation.
  - `FirstParentLeafCommits(repo *gogit.Repository, draftTipSHA
    string) ([]*object.Commit, error)` — walks first-parent from
    `draftTipSHA`; on commits with `Auto-Merger: true` trailer
    (per PROTOCOL.md trailer conventions), follows the second-
    parent first-parent chain back to the merge-base with the
    current first-parent's position to enumerate the integrated
    leaves in chronological order. Returns leaves in DAG-natural
    chronological order (oldest first). The auto-merger merge
    commits themselves are NOT included.
- **`internal/portal/finalize/script_test.go`** — golden tests for
  squash and preserve scripts. Cases: 1-commit, 3-commit,
  10-commit selections. Goldens checked into
  `testdata/squash_script.golden.txt` and
  `testdata/preserve_script.golden.txt`. `FirstParentLeafCommits`
  test uses an in-test bare repo with an auto-merger merge commit
  in the middle of a 5-commit draft chain.
- **`internal/portal/finalize/plan.go`** —
  `func (h *Handler) GetFinalizePlan(ctx, req) (resp, error)`
  Implementation order matches the design:
  1. Load lock by `lockID` (from query param), verify session
     match, check `IsLockExpired`, check `superseded_by_lock_id IS NULL`.
  2. Membership check on caller (org + session).
  3. `gogit.PlainOpen(storage.RepoPath(orgID, sessionID))`.
  4. Resolve each curated SHA via `repo.CommitObject(plumbing.NewHash(sha))`;
     on `plumbing.ErrObjectNotFound` return 409
     `finalize.commit_missing` with `details.missing_sha`.
  5. Build `selected_commits` PlanCommit list (sha, author_name,
     author_email, account_id best-effort via
     `store.GetAccountByEmail`, subject, committed_at).
  6. Squash branch: compose message + co-authors; preserve branch:
     leave commit_message / co_authors null.
  7. Compose script via `BuildScript`.
  8. Build `fetch_source` — kind="https",
     `remote_url = portalURL + "/git/" + orgID + "/" + sessionID + ".git"`,
     `token_expires_at = null`.
  9. Return `PlanResponse` with `plan_id = sessionID + ":" + lockID`.
- **`internal/portal/finalize/plan_test.go`** — integration test
  against in-memory sqlite + a TestMain-built bare repo fixture.
  Covers: happy squash plan, happy preserve plan, missing-SHA-409,
  expired-lock-409, superseded-lock-409, lock-belongs-to-different-
  session-404, non-member-403.
- **OpenAPI additions** — `docs/openapi.yaml`:
  - Schemas: `PlanResponse`, `PlanCommit`, `CoAuthor`, `FetchSource`.
  - Path: `/api/orgs/{orgID}/sessions/{sessionID}/finalize-plan`
    GET with query param `lock_id` (required) → 200 returning
    `PlanResponse`; 409 with `ErrorEnvelope` for the three lock-state
    conflicts (`finalize.lock_expired`, `finalize.lock_superseded`,
    `finalize.commit_missing`); 404 lock not found; standard 401/403.
- **Plan endpoint wired into the handler** — replaces the
  `501 not_implemented` stub from story 1.

## Acceptance Criteria

- [x] `make generate` succeeds
- [x] `go build ./...` clean
- [x] `go test ./internal/portal/finalize/...` green
- [x] `ComposeSquashMessage` golden test passes byte-exact across runs
- [x] Case-variant emails (`Alice@x`, `alice@x`) collapse to ONE
      `Co-authored-by` trailer (using first-seen casing)
- [x] Co-authors render in first-appearance order (NOT alphabetical)
- [x] Squash script template carries `set -euo pipefail`, verbose
      `==>` echos, and the heredoc-delimited commit message
- [x] Preserve script template carries one `git cherry-pick <sha>`
      per selected commit, in selection order
- [x] Script placeholders `$JAMSESH_FETCH_REMOTE`,
      `$JAMSESH_RUNNER_NAME`, `$JAMSESH_RUNNER_EMAIL` appear verbatim
      so the plugin's substitution step is deterministic
- [x] `FirstParentLeafCommits` on a 5-commit chain with an auto-
      merger merge in the middle returns the leaves in chronological
      order, excluding the merge commit itself
- [x] Plan-fetch returns 409 `finalize.lock_expired` when called
      on a lock with `last_activity_at` > 30 min ago
- [x] Plan-fetch returns 409 `finalize.commit_missing` with
      `details.missing_sha` when any curated SHA is absent from the
      bare repo
- [x] Plan-fetch returns 409 `finalize.lock_superseded` with
      `details.superseded_by_lock_id` when the lock has been overridden
- [x] `PlanResponse.plan_id == sessionID + ":" + lockID`
- [x] `PlanResponse.fetch_source.remote_url` is
      `<portalURL>/git/<orgID>/<sessionID>.git`
- [x] In squash mode, `PlanResponse.commit_message` and
      `co_authors` are populated; in preserve mode both are null/empty
- [x] Plan-fetch from non-member returns 403

## Files touched

- `internal/portal/finalize/{message,script,plan}.go` (new)
- `internal/portal/finalize/{message,script,plan}_test.go` (new)
- `internal/portal/finalize/testdata/{squash_message,squash_script,preserve_script}.golden.txt` (new)
- `docs/openapi.yaml` (add `PlanResponse`, `PlanCommit`, `CoAuthor`, `FetchSource`, +1 path)
- `internal/api/openapi/server.gen.go` (regenerated)
- `frontend/src/lib/api/schema.d.ts` (regenerated via `make generate-api-ts`)

## Implementation notes

Landed `message.go`, `script.go`, `plan.go` plus tests under
`internal/portal/finalize/` per the locked design. All acceptance criteria
verified. Wired `GetFinalizePlan` into `cmd/portal/main.go`'s
`combinedHandler` and added the `/finalize-plan` GET route mount. Test
stubs in `internal/portal/{accounts,auth,comments,sessions,tokens}` had
`GetFinalizePlan` panic-stubs added so they still satisfy the strict
interface.

### Deviations from spec

- **`make generate-api-ts` output filename.** Story spec lists
  `frontend/src/lib/api/schema.d.ts` as the regen target; the actual
  Makefile target writes `frontend/src/lib/api/types.gen.ts`. That file
  is regenerated by `make generate-api`. No code action — just a
  spec-line typo.
- **`stripTrailersFromSubject` is intentionally a no-op.** The story
  text said "default: take everything before the first newline, trim
  whitespace, done. Don't over-engineer." That's already what
  `firstLine` does. The named function exists for documentation /
  future-extension but currently returns its input as-is.
- **`-update` flag is provided** for both message and script golden
  tests. Story said "nice-to-have"; I added it because the goldens are
  large and re-deriving them by hand on intentional output changes
  would be tedious.
- **`hardTruncate`** added for the user-override subject path so the
  rune-aware truncation logic is shared with `wordBoundaryTruncate`.
  Inline rune iteration was getting awkward to read.
- **`RenderSquashMessageBody`** exposed as a public helper so
  `plan.go` can compose the same exact bytes the script heredoc
  consumes. This keeps "the message in the response body" and "the
  message bytes inside the script heredoc" guaranteed identical.

### Golden tests added

- `testdata/squash_message_*.golden.txt` — 6 cases (single-author,
  three-authors, case-variant dedup, user-override, truncation, empty)
- `testdata/squash_script_*.golden.txt` — 3 cases (1, 3, 10 commits)
- `testdata/preserve_script_*.golden.txt` — 3 cases (1, 3, 10 commits)
- `FirstParentLeafCommits` — 3 scenarios (linear chain, single commit,
  5-commit chain with auto-merger merge in the middle)
- `plan_test.go` — 7 integration scenarios against an in-memory store +
  a real bare repo on disk built via go-git's object store

### Auto-merger trailer

PROTOCOL.md > Commit trailers and `internal/portal/automerger/outcomes.go`
both pin the trailer string as `Auto-Merger: true`. The
`autoMergerTrailer` constant in `script.go` matches verbatim.
