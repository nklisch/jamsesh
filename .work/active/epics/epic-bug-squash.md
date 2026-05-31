---
id: epic-bug-squash
kind: epic
stage: done
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

Split by **capability / shared-code seam**, not by layer. Six of the seven
features are independent roots (parallelizable); only
`frontend-sessionlist-subscription` depends on the WebSocket-lifecycle rework,
because its SessionList fixes build on the corrected `subscribe`/`close`
contract. (The codex decomposition gate split this 2-story concern out of
`frontend-async-races` so the WS dependency does not block the two independent
High async fixes — see `## Other agent review`.) The two natural hotspots — the
auto-merger (`internal/portal/automerger/`) and the WS manager
(`ws.svelte.ts`) — each become their own feature so related fixes share a design
pass.

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
- `epic-bug-squash-frontend-async-races` — WS-independent screen/component async
  + reactive races (5 stories: 2H/2M/1L) — depends on: `[]`
- `epic-bug-squash-frontend-sessionlist-subscription` — SessionList subscription
  + event-refetch correctness (2 stories: 2M) — depends on: `[epic-bug-squash-frontend-ws-lifecycle]`

## Design decisions

(Resolved with judgment under autopilot per the queue policy; a codex
cross-model peer-review gate aligns on these after this decomposition — see
`## Other agent review`.)

- **Regression test per fix**: every story lands a failing-first regression test
  before its fix — per the project test-integrity rule; no fix without a test
  that would have caught it. Concurrency stories additionally get `go test -race`
  coverage and, where practical, a deterministic interleaving harness.
- **Corrected behavior is the intended contract**: fixes that change observable
  status are deliberate corrections, and `feature-design` updates tests asserting
  the old (wrong) behavior. But the corrections are NARROW (see each feature's
  `## Design caveats`): receive-pack returns 500 only on IO/truncation failure
  (git-level rejections still return 200 + report-status); git-auth skips 5xx
  only for genuine client-abort/request-context cancellation (a store
  `DeadlineExceeded` stays 5xx); magic-link returns transient 5xx only on a real
  driver error (a 0-row consume stays a permanent 401).
- **Dual-dialect + migration discipline (data-tx feature)**: the Postgres `seq`
  change is a **non-destructive widening** (INTEGER → BIGINT) covering both
  `events.seq` and `event_seq.next`, with mirrored sqlc regen across
  sqlite + postgres, removal of the adapter `int32` casts, an existing-row
  migration test, and a no-destructive-down policy (narrowing would truncate).
  Keyset pagination threads `cur.LastID` through both dialect queries.
- **WS lifecycle lands first (scoped)**: `frontend-ws-lifecycle` is the
  foundation only for `frontend-sessionlist-subscription` (the 2 SessionList
  stories). The other frontend async fixes (`frontend-async-races`) are
  WS-independent and parallelizable.
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

Codex (cross-model, xhigh) reviewed this decomposition. Verdict: backend
decomposition sound; the frontend over-constraint was fixed before
feature-design.

**Accepted & applied:**
- Split `frontend-sessionlist-subscription` (2 SessionList stories that need the
  WS contract) out of `frontend-async-races`, so the WS dependency no longer
  blocks the two independent High async fixes (ArtifactPane, finalize-stores).
- Pinned narrow-correction caveats into `handler-error-classification`
  (receive-pack git-rejection vs IO-failure; git-auth client-abort vs store
  timeout; magic-link 0-row vs driver error, dual-dialect `:execrows`).
- Tightened the seq-migration decision ("non-destructive widening"; both
  columns + sqlc regen + int32-cast removal + migration test + no destructive
  down) in `data-tx-integrity`.
- Added intra-feature ordering: `finalize-lock-no-transaction` depends_on
  `sqlite-withtx-deferred-not-immediate`.
- Recorded the finalize-store pattern caveat (adopt `per-instance-factory-rune-store`).

**Rejected / confirmed non-issues:** `handler-error-classification` is a
coherent seam (not artificial); no backend cross-feature edges needed
(automerger / worker-lifecycle / data-tx / handler-error are parallelizable);
`gateway-slow-consumer-close` (backend) needs no edge to the frontend WS work.

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

## Completion

All 28 correctness fixes implemented, tested, reviewed, and advanced to
`stage: done` across the 7 features. Highlights:

- **Design tier**: 8 codex (cross-model, xhigh) gates — 1 on the epic
  decomposition + 1 per feature — each caught real issues (a false dependency
  that blocked two High fixes; double-emit/cancelled-ctx hazards; a finalize-tx
  scope gap; an LRU TOCTOU; a `?1`-aliasing + missing first-page sentinel; a
  too-broad receive-pack EPIPE rule; ws-lifecycle async-cancellation guards).
- **Implementation**: 7 bundles (serial, main-tree, explicit-path commits) +
  the sqlc invariant verified clean (`sqlc generate` → zero diff).
- **Final completion gate**: a codex xhigh review over the full 88-file bundle
  found 5 BLOCKING correctness regressions + 2 test-integrity gaps. All fixed
  across two fix passes and re-verified clean by codex (conflict.resolved emit
  escalation; finalize stale-lock TOCTOU closed via an atomic conditional
  release; ForkDialog `/mcp` bearer; strict first-pkt-line `looksLikeReportStatus`;
  LRU resident-byte accounting; two tests made genuine/honest).
- **Verification**: backend `go build`/`go vet`/`-race` clean on touched
  packages; frontend 765 vitest + `svelte-check` green; dual-dialect store tests
  pass (Postgres testcontainer skips gracefully where no test DB is configured —
  noted for release-time validation).
- **Deferred follow-ups parked** (`.work/backlog/`): `bug-followup-tombstone-int32`,
  `bug-followup-adjacent-client-abort-classification`.

Release binding stays `null` (late-binding) — `release-deploy` cuts the release.
