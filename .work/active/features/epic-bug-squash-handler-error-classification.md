---
id: epic-bug-squash-handler-error-classification
kind: feature
stage: drafting
tags: [bug, portal]
parent: epic-bug-squash
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-30
updated: 2026-05-30
---

# Portal request-handler error & status classification

## Brief

Three portal HTTP handlers misclassify errors or report false success. The
bug-scan found: a magic-link token-consume DB error masked as a permanent 401
"already used" (a valid unused link becomes unusable on any transient DB
failure, High); a git smart-HTTP receive-pack that returns 200 OK with
possibly-truncated output because the stdin-copy and stdout-read errors are
discarded (a partially-failed push acknowledged as success, Medium); and a git
auth middleware that maps client-abort (context cancellation) to HTTP 500
(inflating 5xx alerts, Low).

This feature delivers correct error/status classification at these handler
boundaries: transient DB failures surface as retryable 5xx (not a false 401),
a truncated/failed push fails loudly instead of a false 200, and client aborts
are not counted as server errors — routing through the existing
`deperr`→`httperr` pipeline where applicable. It covers these three handlers'
classification correctness only; it does NOT change auth semantics, the git
wire protocol, or the error-envelope shape. Note: corrected status codes are a
deliberate, intended behavior change — feature-design must update any tests
asserting the old (wrong) codes.

## Epic context
- Parent epic: `epic-bug-squash`
- Position in epic: independent backend feature — touches
  `internal/portal/auth` and `internal/portal/githttp`.

## Foundation references
- `docs/ARCHITECTURE.md` — Portal § REST API, Git smart-HTTP
- Patterns: `deperr-translate-pipeline`, `authfail-three-branch-guard`

## Child stories (pre-existing, from bug-scan — re-parented here)
- `bug-squash-magic-link-db-error-masked-401` — High, error-handling — `internal/portal/auth/magic_link.go:174`
- `bug-squash-receive-pack-truncated-200` — Medium, error-handling — `internal/portal/githttp/receive_pack.go:228`
- `bug-squash-git-auth-client-abort-500` — Low, error-handling — `internal/portal/githttp/auth.go:47`

## Design caveats (from codex decomposition gate — feature-design must honor)
- **receive-pack**: the fix is NARROW. Git-level rejections from `git
  receive-pack` (pre-receive hook reject, non-fast-forward) MUST still return
  HTTP 200 with the report-status payload — that is the smart-HTTP protocol.
  Only stdin/stdout copy/truncation/IO failures (and a post-receive failure that
  occurs before any report-status header is flushed) become HTTP 500. Do not
  turn protocol-level push rejections into 5xx.
- **git-auth**: only genuine request-context cancellation / client disconnect
  may skip the 5xx + error log. A store-side `context.DeadlineExceeded` (a real
  dependency timeout) MUST remain a 5xx. Decide and document the client-abort
  convention (no response vs a 499-style metric label) rather than silently
  swallowing.
- **magic-link**: behavior change is safe only if a 0-rows-affected consume
  stays a permanent 401 while a real driver error becomes a transient 5xx —
  requires `:execrows`/`RETURNING` (or a re-read) mirrored across BOTH sqlite and
  postgres queries, plus updated single-use/concurrency tests.

<!-- feature-design fills in the rows-affected vs error distinction for
magic-link, the stdin/stdout error gating for receive-pack, and the
context-cancellation detection for git auth. -->
