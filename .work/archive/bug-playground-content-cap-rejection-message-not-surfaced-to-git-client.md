---
id: bug-playground-content-cap-rejection-message-not-surfaced-to-git-client
kind: story
stage: done
tags: [bug, playground, git, ux]
parent: null
depends_on: []
release_binding: null
gate_origin: e2e-audit-playground-content-cap-pre-receive-enforcement
created: 2026-05-24
updated: 2026-05-24
---

# Playground content-cap rejection message not surfaced to git client

## Severity
Medium

## Finding type
ux-bug / protocol-bug

## Evidence

Found during implementation of `e2e-audit-playground-content-cap-pre-receive-enforcement`.
The e2e test confirms the content cap IS enforced (push exits non-zero), but the git client
displays `fatal: the remote end hung up unexpectedly` instead of the intended human-readable
message `playground session content limit exceeded`.

Observed portal behavior:
- On oversize push, portal returns HTTP 200 with ~241 bytes (the pkt-line report-status
  payload from `writeReportStatusRejection`).
- Git client interprets this as a protocol error and shows "remote end hung up" instead
  of the rejection message in the pkt-line payload.

## Root cause hypothesis

Git's smart-HTTP stateless-RPC protocol for large pushes sends TWO POST requests:
1. A "probe" POST with only the command list (no pack data) — portal validates OK,
   spawns `git receive-pack --stateless-rpc`, returns 0 bytes (subprocess's output).
2. A "full" POST with command list + pack — portal validates, finds cap exceeded,
   calls `writeReportStatusRejection`.

The `caps` map is re-parsed on each stateless-RPC POST from `readCommandList`. It is
possible that the second POST's command list omits the `side-band-64k` capability (since
capabilities are only advertised once in the first POST per the git wire protocol spec).
If `caps["side-band-64k"]` is false for the second POST, `writeReportStatusRejection`
writes non-sideband pkt-lines — but git expects sideband-64k format because it negotiated
it in the first POST. This mismatch causes "remote end hung up."

Alternatively, git may be expecting the report-status to include a "pkt-flush" from the
subprocess first, then the rejection — but `writeReportStatusRejection` skips the probe
phase's output.

## Impact

Users who exceed the playground content cap see `fatal: the remote end hung up unexpectedly`
instead of `error: remote: playground session content limit exceeded: this push would exceed
the maximum allowed repo size for a playground session`. The push IS correctly rejected
(non-zero exit), so the cap is enforced. Only the UX (error message readability) is affected.

## Suggested remedy

1. Capture the caps from the first stateless-RPC probe POST and store them in the session
   context (or pass them via a header) so the second POST can use the correct format.
2. Alternatively: always write sideband-64k format from `writeReportStatusRejection`
   regardless of caps negotiation (sideband is always advertised by modern git clients).
3. Add a unit test to `receive_pack_test.go` that exercises the two-POST stateless-RPC
   flow with a cap rejection and asserts the git client sees the human-readable message
   (not "hung up").

## Test evidence

```
// From e2e test: playground_content_cap_test.go
content_cap: oversize push correctly rejected (exit non-zero):
git push HEAD:refs/heads/...: exit status 1
output: fatal: the remote end hung up unexpectedly
        fatal: the remote end hung up unexpectedly
        error: failed to push some refs to 'http://...'
```

The portal logs show the rejection is being sent (HTTP 200, bytes=241), so the issue
is in the client-side pkt-line parsing, not in the rejection logic itself.

## Implementation notes

### Which option was picked

Option 2 (always write sideband-64k) was the starting point, but investigation revealed
a deeper protocol bug that required an additional fix.

### Root cause (confirmed via GIT_TRACE_PACKET=1 + GIT_TRACE_CURL=1)

The original hypothesis (caps mismatch between probe POST and full push POST) turned out
to be wrong for this specific test setup — git sends capabilities on EVERY POST body
(not just the first), so `caps["side-band-64k"]` was actually `true` on the rejection
path. The sideband framing was already correct.

The actual bug was a missing inner flush packet in the sideband stream:

- git's report-status parser reads all content — including the terminating flush pkt
  (`0000`) — through the sideband demultiplexer (band 1).
- `writeReportStatusRejection` was calling `writeFlushPkt(w)` which writes the outer
  sideband flush `0000` directly to the writer. This terminates the sideband stream.
- But git's inner report-status parser then tries to read ONE more pkt-line (the inner
  flush) through the (now-closed) sideband. It gets EOF and shows
  "remote end hung up unexpectedly" — even though the `ng` lines were already parsed
  and the rejection was understood.

### Fix applied

Two changes to `writeReportStatusRejection` in `internal/portal/githttp/pktline.go`:

1. **Always use sideband-64k** regardless of the `caps` map (defensive fix; the cap IS
   present in practice but the code should not depend on it).

2. **Send the inner flush through band-1** before the outer sideband flush:
   ```
   writeSidebandPktLine(w, 0x01, "0000")  // inner flush via band-1
   writeFlushPkt(w)                        // outer sideband stream end
   ```

### Regression assertion

`TestReceivePack_RejectionMessageSurfacedToClient` in
`internal/portal/githttp/receive_pack_test.go`. Pushes a commit missing required
trailers through the real `git push` client → httptest server pipeline and asserts the
output contains `"missing required trailers"`. Before the fix this test fails with only
"remote end hung up unexpectedly"; after the fix it passes.

### Side-benefit

All prereceive rejection types (content-cap, missing trailers, ref-namespace violations,
scope violations) flow through `writeReportStatusRejection`. The fix corrects the sideband
framing for all of them, not just content-cap rejections.

## Review (2026-05-24)

**Verdict**: Approve

**Notes**:

The agent's investigation overturned the original hypothesis (sideband
caps missing on the second stateless-RPC POST). Real cause, confirmed
via `GIT_TRACE_PACKET=1 GIT_TRACE_CURL=1`: a missing inner flush
packet INSIDE the sideband stream.

Git's report-status parser reads ALL content — including the inner
terminating `0000` flush — through the sideband band-1 demultiplexer.
The original `writeReportStatusRejection` wrote `writeFlushPkt(w)`
which terminated the OUTER sideband stream directly, so git's inner
parser tried to read one more pkt-line through a now-closed sideband,
hit EOF, and surfaced "remote end hung up" — even though the `ng`
rejection lines had already been parsed correctly.

Fix is two changes in `pktline.go`:
1. Always write sideband-64k format regardless of `caps` (defensive
   for the two-POST stateless-RPC corner).
2. Send inner `0000` flush as a sideband band-1 packet BEFORE the
   outer sideband flush.

**Side-benefit** (worth highlighting): all prereceive rejection paths
flow through `writeReportStatusRejection` — content-cap, missing
trailers, ref-namespace violations, scope violations. The fix benefits
ALL of them. The trailer-rejection bug we fixed earlier (commit
`297616a`) which had to add an exemption to AVOID the bad UX is now
backed by a real UX fix; future trailer rejections that legitimately
fire (on non-base refs) will now surface their reason instead of
"hung up".

Tests:
- `TestWriteReportStatusRejection_SidebandWrap` updated to assert the
  three-packet structure (unpack ok + ng + inner flush) followed by
  outer flush.
- `TestReceivePack_RejectionMessageSurfacedToClient` added as a
  regression that pushes a no-trailers commit through real `git push`
  → httptest server and asserts `"missing required trailers"` appears
  in git's output (not just "hung up").

Optional follow-up (not blocking): extend `TestPlayground_ContentCap`
in the e2e suite to assert the cap's rejection message is now visible
end-to-end through the real container pipeline. Currently the test
only asserts exit-code != 0 + on-disk size constraint. Skipping for
now — the unit-level regression locks the contract and the e2e test
will benefit silently.

Verification: `go test ./internal/portal/githttp/ -count=1` (1.334s)
all green.

Advanced `stage: review → done`.
