---
id: epic-bug-squash
kind: epic
stage: drafting
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

The decomposition is already complete — the 28 child stories exist (see below).
This epic does not need further `epic-design` decomposition; it needs its
children designed (or fast-tracked via `/agile-workflow:fix` for the trivial
ones) and implemented.

## Sequencing

These are **independent fixes** — there is no real `depends_on` graph between
unrelated bugs, so none is fabricated (that would create false blocking and a
forced serial order). Priority is expressed three ways instead:

- `bug_severity` frontmatter field on each child,
- a `high` tag on the 5 High-severity stories (`work-view --tag high --parent epic-bug-squash`),
- the **recommended order** below: Highs first, then Mediums, then Lows.

Two **hotspots** are worth tackling as a batched design pass each, since their
findings share code and context:

- **Auto-merger** (`internal/portal/automerger/`) — 4 findings:
  `bug-squash-automerger-strands-commit-event` (High),
  `bug-squash-automerger-swallows-merge-emit` (High),
  `bug-squash-errors-is-not-used-errnotfound` (Low),
  `bug-squash-diff-exit-code-ignored` (Low). The two Highs touch
  `worker.go`/`outcomes.go` together.
- **Frontend WebSocket manager** (`frontend/src/lib/ws.svelte.ts`) — 3 findings:
  `bug-squash-ws-connection-never-closed` (Medium, leak),
  `bug-squash-ws-reconnect-cursor-reset` (Medium),
  `bug-squash-subscribe-floats-open-rejection` (Low). A single rework of the
  connection lifecycle (ref-counted teardown + reconnect-aware `open()`) likely
  resolves all three.

## Decomposition

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

- List children: `.work/bin/work-view --parent epic-bug-squash`
- Surface the Highs: `.work/bin/work-view --parent epic-bug-squash --tag high`
- Each child sits at `stage: drafting`. Route through the normal design →
  implement → review flow, or `/agile-workflow:fix <id>` for the trivial Lows
  (e.g. `errors-is-not-used-errnotfound`, `diff-exit-code-ignored`,
  `git-auth-client-abort-500`) which need no design pass.
- The epic advances to `done` when all 28 children reach `stage: done`.

## Source

`bug-scan-report.md` — repo-wide bug-scan, generated 2026-05-30. All child
stories were promoted from `.work/backlog/bug-scan-*.md` (`bug_origin: scan`)
via `/agile-workflow:scope`.
