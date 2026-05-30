---
id: epic-bug-squash-handler-error-classification
kind: feature
stage: implementing
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

## Architectural choice

**Local classification fixes at each handler boundary**, routed through the
existing `deperr`→`httperr` pipeline where applicable. No shared helper — the
three handlers have different transport shapes (oapi strict handler, git
smart-HTTP subprocess, chi middleware). All 3 stories are independent files
(auth/magic_link.go, githttp/receive_pack.go, githttp/auth.go) — parallelizable.

## Implementation Units

### Unit 1: Magic-link distinguishes race-loss (401) from driver error (5xx)
**Files**: `db/queries/sqlite/magic_link_tokens.sql`,
`db/queries/postgres/magic_link_tokens.sql`, regenerated `*store/*`,
`internal/db/store/{sqlite,postgres}_adapter.go`, `internal/portal/auth/magic_link.go`
**Story**: `bug-squash-magic-link-db-error-masked-401` (High)

`ConsumeMagicLinkToken` is `UPDATE ... WHERE id = ? AND used_at IS NULL`; a
race-loser updates 0 rows with NO error, so today the `err != nil` branch only
fires on a real transient DB failure yet returns a permanent 401. Change the
query to `:execrows` (rows-affected) in BOTH dialects; classify in the handler:

```go
affected, err := h.store.ConsumeMagicLinkToken(ctx, store.ConsumeMagicLinkTokenParams{ID: row.ID, UsedAt: &now})
if err != nil {
    return nil, deperr.WrapDBIfTransient(fmt.Errorf("magic-link: consume token: %w", err)) // real driver err → 5xx
}
if affected == 0 {
    return magicLinkUnauthorized("auth.invalid_token", "magic link already used"), nil // race lost → 401
}
// affected == 1 → this caller won; proceed to provision + issue
```

**Implementation Notes**: switching to `:execrows` changes the adapter signature
to `(int64, error)` — update both adapters and the consumer. This ALSO fixes the
secondary bug the scan noted: the 0-row race-loser currently falls through to
`FindOrProvisionAt`/`Issue` and gets a token pair too; gating on `affected==1`
enforces true single-use at the app layer.

**Acceptance Criteria**:
- [ ] A transient DB error on consume → 5xx (not 401); a 0-row race-loss → 401;
      a 1-row win → success (tested via a stub store: error case, 0-rows case,
      1-row case), mirrored for sqlite + postgres.
- [ ] Two concurrent exchanges of the same token: exactly one gets a token pair.

### Unit 2: Receive-pack fails IO/truncation as 500, keeps git rejections as 200
**File**: `internal/portal/githttp/receive_pack.go` (~:227-250)
**Story**: `bug-squash-receive-pack-truncated-200` (Medium)

The stdin-copy error (`<-stdinErrCh`) is drained-and-dropped and the stdout
`io.Copy` error is ignored. Gate on them — BUT preserve the git-protocol
contract (codex caveat): a non-zero `git receive-pack` exit that produced a
report-status is a LEGITIMATE 200 (push rejected by hook / non-ff), not a 500.

```go
var subprocOut bytes.Buffer
_, stdoutErr := io.Copy(&subprocOut, stdout)
cmdErr := cmd.Wait()
stdinErr := <-stdinErrCh

// IO/transport failures (NOT the subprocess closing stdin early on rejection):
if stdoutErr != nil { /* couldn't read the full report */ 500; return }
if stdinErr != nil && !isPipeClosedByPeer(stdinErr) { /* our-side body read failed */ 500; return }

if cmdErr != nil {
    // git-level rejection WITH report-status — 200 + report (unchanged, correct git behavior)
    w.WriteHeader(http.StatusOK); _, _ = w.Write(subprocOut.Bytes()); return
}
// success → sync + 200 (unchanged)
```

**Implementation Notes**: `isPipeClosedByPeer` treats `io.ErrClosedPipe` /
`syscall.EPIPE` as "subprocess closed stdin early" (a rejection signal), which
defers to `cmdErr`/report-status rather than masking it as 500. A genuine
tempfile read error (our side) is a 500. The RPO=0 success path
(EmitForUpdates before 200) is unchanged.

**Acceptance Criteria**:
- [ ] A simulated stdout-read truncation / our-side stdin read error → 500 (not
      a false 200).
- [ ] A git-level push rejection (non-zero exit with report-status) still → 200
      with the report payload (regression guard).

### Unit 3: Git auth middleware: client-abort is not a 5xx
**File**: `internal/portal/githttp/auth.go` (basicAuth ~:47, requireSessionMember ~:87, checkArchived ~:110)
**Story**: `bug-squash-git-auth-client-abort-500` (Low)

The three `default: 500` branches catch `context.Canceled` from a client that
hung up. Discriminate on the REQUEST context (codex caveat: a store-side
`DeadlineExceeded` that is NOT request-ctx cancellation stays 5xx):

```go
default:
    if r.Context().Err() != nil { // request cancelled / client disconnected
        w.WriteHeader(499) // client closed request; no error log, no 5xx metric
        return
    }
    http.Error(w, "internal server error", http.StatusInternalServerError)
```

**Implementation Notes**: gate on `r.Context().Err() != nil` (the request ctx),
NOT on `errors.Is(err, context.DeadlineExceeded)` alone — a store/dep timeout
with a live request ctx is a real 5xx. Apply the same guard to all three
middleware default branches. 499 (nginx "client closed request") is a sentinel
status; the client is already gone so the body is irrelevant. Optionally log at
debug.

**Acceptance Criteria**:
- [ ] A request whose context is cancelled before the store call returns → 499
      (or no-5xx), no ERROR log; a genuine store error with a live ctx → still
      500 (tested by cancelling the request ctx vs a stub store error).

## Implementation Order
All 3 units independent (distinct files) — parallelizable. Units 2 & 3 both live
in `githttp/` but different files (receive_pack.go vs auth.go), no conflict.

## Testing
- Unit 1: stub store with 3 ConsumeMagicLinkToken behaviors (error / 0-rows /
  1-row); dual-dialect query test for `:execrows`; a concurrency test for
  single-use.
- Unit 2: a fake stdout reader that errors mid-read (→500); a subprocess stub
  that exits non-zero with a report (→200); an our-side stdin read error (→500).
- Unit 3: drive each middleware with a cancelled `r.Context()` (→499) and with a
  stub store error on a live ctx (→500).

## Risks
- **Unit 2 EPIPE classification**: distinguishing "subprocess closed stdin on
  rejection" (defer to report-status, 200) from "our tempfile read failed" (500)
  is the subtle part — covered by both regression tests above. If
  `isPipeClosedByPeer` is hard to get right cross-platform, the conservative
  fallback is to treat any stdinErr as 500 (slightly over-strict but safe; git
  clients retry) — decide during implementation.
- **Unit 1 dual-dialect `:execrows`**: ensure both adapters return the same
  rows-affected semantics.

## Design decisions
- **Magic-link**: `:execrows` (rows-affected) over `RETURNING` — simpler, works
  identically in both dialects; gating on `affected==1` also fixes the
  double-provision race.
- **Receive-pack**: gate IO errors as 500 but preserve git-level rejection →
  200+report (the smart-HTTP contract). EPIPE-on-rejection defers to cmdErr.
- **Git-auth**: discriminate on `r.Context().Err()` (request cancellation), not
  the error's deadline type — a dep timeout stays 5xx. Use 499 sentinel.

## Other agent review

_Codex (xhigh) feature peer-review gate pending._
