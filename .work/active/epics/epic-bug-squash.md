---
id: epic-bug-squash
kind: epic
stage: implementing
tags: [bug, portal, ui]
parent: null
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-30
updated: 2026-05-30
---

# Bug Squash: correctness findings from the 2026-05-30 repo-wide bug-scan

## Brief

A repo-wide `/agile-workflow:bug-scan` (full 8-domain audit, 8 parallel deep
scanners over ~344 source files) surfaced **28 confirmed correctness bugs**:
0 Critical, 5 High, 15 Medium, 8 Low. This epic bundles all 28 into a single
bug-squashing arc so they can be designed, fixed, reviewed, and shipped as a
cohesive effort rather than scattered backlog items.

Each finding was confirmed by reading the surrounding code (not grep hits), and
each child story carries its `bug_severity`, `bug_domain`, and `bug_location`
metadata plus a one-paragraph capture and code evidence. The full scored report
lives at `bug-scan-report.md` (repo root, overall score **5.8/10**).

This epic is decomposed into **6 child features** (see `## Decomposition`)
that group the 28 stories by shared code and a common design pass. Each feature
gets a `feature-design` pass (with a codex cross-model peer-review gate), then
its stories are implemented and reviewed.

## Decomposition

Split by **capability / shared-code seam**, not by layer. Five features are
independent roots (parallelizable); the frontend async cluster depends on the
WebSocket-lifecycle rework because its SessionList fixes build on the corrected
`subscribe`/`close` contract. The two natural hotspots — the auto-merger
(`internal/portal/automerger/`) and the WS manager (`ws.svelte.ts`) — each
become their own feature so the related fixes share one design pass.

### Child features

- `epic-bug-squash-automerger-correctness` — auto-merger lost-event race,
  swallowed emit, error-classification (4 stories: 2H/2L) — depends on: `[]`
- `epic-bug-squash-worker-lifecycle` — lease/wsgateway/objectstore/ratelimit
  concurrency & lifecycle (6 stories: 4M/2L) — depends on: `[]`
- `epic-bug-squash-data-tx-integrity` — pagination keyset, SQLite tx isolation,
  seq type, tx/event consistency (5 stories: 4M/1L) — depends on: `[]`
- `epic-bug-squash-handler-error-classification` — magic-link/receive-pack/git-auth
  status classification (3 stories: 1H/1M/1L) — depends on: `[]`
- `epic-bug-squash-frontend-ws-lifecycle` — ws.svelte.ts connection lifecycle
  (3 stories: 2M/1L) — depends on: `[]`
- `epic-bug-squash-frontend-async-races` — screen/component async + reactive
  races (7 stories: 2H/4M/1L) — depends on: `[epic-bug-squash-frontend-ws-lifecycle]`

## Design decisions

(Resolved with judgment under autopilot per the queue policy; a codex
cross-model peer-review gate aligns on these after this decomposition — see
`## Other agent review`.)

- **Regression test per fix**: every story lands a failing-first regression test
  before its fix — per the project test-integrity rule; no fix without a test
  that would have caught it.
- **Corrected behavior is the intended contract**: fixes that change observable
  status (receive-pack false-200 → 500; git-auth client-abort → not-5xx;
  magic-link false-401 → transient 5xx) are deliberate corrections.
  `feature-design` updates any test asserting the old (wrong) behavior rather
  than preserving it.
- **Dual-dialect + migration discipline (data-tx feature)**: schema changes
  (Postgres `seq` INTEGER → BIGINT; keyset pagination) ship as a forward goose
  migration with mirrored sqlc regen across sqlite + postgres; widening is
  additive (no destructive down), validated by the dual-dialect test matrix.
- **WS lifecycle lands first**: `frontend-ws-lifecycle` is the foundation;
  `frontend-async-races` depends on its corrected `subscribe`/`close` contract.
- **Late release binding**: `release_binding` stays `null`; `release-deploy`
  binds and runs gates later.

## Decomposition risks

- **`automerger-correctness`** carries the riskiest single change — the
  worker/queue lost-event race needs a lifecycle redesign with race-detector
  tests; highest blast radius (silent missed merges) if done wrong.
- **`frontend-async-races`** is the largest (7 stories) and spans many
  components; `feature-design` may sub-group by screen. Coupling to the WS
  contract is mitigated by the depends_on edge to `frontend-ws-lifecycle`.
- **`data-tx-integrity`** includes a Postgres migration; mitigated by additive
  widening + testcontainers postgres coverage.
- **Concurrent agent**: another agent is draining `feature-cli-jam-open-in-browser`
  in `cmd/jamsesh/`. This epic touches `internal/portal/*` and `frontend/*`
  (plus the `cmd/portal/main.go` retention call site) — disjoint from
  `cmd/jamsesh/`, so no file conflicts are expected.

## Other agent review

<!-- codex cross-model peer-review gate fills this in after epic-design. -->
_Pending codex (xhigh) peer-review gate on this decomposition._

## Story inventory (by severity)

Full per-story inventory. Each story is parented to one of the 6 features above;
severity ordering (Highs first) drives the recommended fix order.

### High (5) — fix first
| Story | Domain | Location |
|---|---|---|
| `bug-squash-automerger-strands-commit-event` | concurrency | `internal/portal/automerger/worker.go:130` |
| `bug-squash-automerger-swallows-merge-emit` | error-handling | `internal/portal/automerger/outcomes.go:155` |
| `bug-squash-magic-link-db-error-masked-401` | error-handling | `internal/portal/auth/magic_link.go:174` |
| `bug-squash-artifactpane-stale-fetch-overwrite` | async | `frontend/src/lib/components/ArtifactPane.svelte:25` |
| `bug-squash-finalize-stores-module-singletons` | async | `frontend/src/lib/finalize/useFinalizeLock.svelte.ts:18` |

### Medium (15)
| Story | Domain | Location |
|---|---|---|
| `bug-squash-cursor-pagination-drops-rows` | data-layer | `db/queries/sqlite/comments.sql:27` |
| `bug-squash-sqlite-withtx-deferred-not-immediate` | data-layer | `internal/db/store/sqlite_adapter.go:1034` |
| `bug-squash-pghandle-heartbeat-conn-race` | concurrency | `internal/portal/lease/postgres.go:219` |
| `bug-squash-lru-evicts-hot-sessions` | concurrency | `internal/portal/storage/objectstore/lifecycle.go:350` |
| `bug-squash-ticketstore-stop-double-close` | concurrency | `internal/portal/wsgateway/tickets.go:92` |
| `bug-squash-lease-retention-frozen-now` | time-numbers | `internal/portal/lease/retention.go:25` |
| `bug-squash-comments-fanout-omits-seq` | error-handling | `internal/portal/comments/service.go:254` |
| `bug-squash-finalize-lock-no-transaction` | error-handling | `internal/portal/finalize/lock_acquire.go:187` |
| `bug-squash-receive-pack-truncated-200` | error-handling | `internal/portal/githttp/receive_pack.go:228` |
| `bug-squash-ws-connection-never-closed` | resource-leak | `frontend/src/lib/ws.svelte.ts:317` |
| `bug-squash-ws-reconnect-cursor-reset` | async | `frontend/src/lib/ws.svelte.ts:248` |
| `bug-squash-ws-refetch-stale-overwrite` | async | `frontend/src/lib/screens/SessionList.svelte:78` |
| `bug-squash-sessionlist-resubscribe-churn` | state | `frontend/src/lib/screens/SessionList.svelte:68` |
| `bug-squash-magic-link-fetch-no-trycatch` | async | `frontend/src/lib/screens/Login.svelte:110` |
| `bug-squash-forkdialog-empty-org-refs-fetch` | async | `frontend/src/lib/components/ForkDialog.svelte:48` |

### Low (8)
| Story | Domain | Location |
|---|---|---|
| `bug-squash-postgres-seq-32bit` | data-layer | `db/schema/postgres.sql:118` |
| `bug-squash-ratelimit-reservation-cancel` | concurrency | `internal/portal/ratelimit/store.go:106` |
| `bug-squash-gateway-slow-consumer-close` | concurrency | `internal/portal/wsgateway/gateway.go:127` |
| `bug-squash-errors-is-not-used-errnotfound` | error-handling | `internal/portal/automerger/worker.go:338` |
| `bug-squash-diff-exit-code-ignored` | error-handling | `internal/portal/automerger/heuristics.go:228` |
| `bug-squash-git-auth-client-abort-500` | error-handling | `internal/portal/githttp/auth.go:47` |
| `bug-squash-subscribe-floats-open-rejection` | async | `frontend/src/lib/ws.svelte.ts:299` |
| `bug-squash-countdownbadge-per-tick-write` | async | `frontend/src/lib/components/CountdownBadge.svelte:50` |

## Driving this epic

- List features: `.work/bin/work-view --parent epic-bug-squash`
- List a feature's stories: `.work/bin/work-view --parent epic-bug-squash-<feature>`
- Surface the Highs across the epic: `.work/bin/work-view --tag high` (filter to bug-squash ids)
- Each feature is at `stage: drafting` → `feature-design` (then a codex gate),
  which advances it to `implementing` and readies its stories. Then
  `implement-orchestrator` per feature, then `review`.
- The epic advances to `done` when all 6 features (and their 28 stories) reach
  `stage: done`.

## Source

`bug-scan-report.md` — repo-wide bug-scan, generated 2026-05-30. All child
stories were promoted from `.work/backlog/bug-scan-*.md` (`bug_origin: scan`)
via `/agile-workflow:scope`.
