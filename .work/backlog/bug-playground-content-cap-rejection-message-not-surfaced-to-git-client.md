---
id: bug-playground-content-cap-rejection-message-not-surfaced-to-git-client
kind: story
stage: implementing
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
