---
id: bug-receive-pack-report-status-sideband-wrapping
kind: story
stage: implementing
tags: [bug, portal, git, protocol]
parent: null
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-19
updated: 2026-05-20
---

# receive-pack report-status missing sideband-64k wrap

## Brief

When the git smart-HTTP client negotiates `sideband-64k` in the
receive-pack capabilities (the modern default), the portal must wrap its
report-status pkt-line payload in an outer sideband channel — `\x01`
(band 1) for the data. The current `writeReportStatusRejection` writes
report-status pkt-lines directly to the response body without that outer
wrap; the client reads the first byte after the outer pkt-line length
prefix as a sideband number and fails with `bad band #117` (where 117 is
'u' from `unpack ok`, or whatever leading byte comes from the inner
pkt-line).

Reproducer (consistent on every run): `TestObjectStorageRPO0/refs_only_update`
does a force-push that pre-receive rejects (or accepts then errors)?
actually it's a force-push that should succeed but fails sideband-parsing
on the response. Verify by running the test locally with `git -c
sendpack.sideband=false push ...` — the side-band-less protocol bypasses
this bug.

Fix scope:
1. Parse capabilities from the first command list line in
   `internal/portal/githttp/pktline.go::readCommandList`. Today
   capabilities are stripped (line 66) but not retained.
2. Pass the parsed capability set into `writeReportStatusRejection` (and
   the non-rejection path that flushes `subprocOut.Bytes()` after the
   subprocess exits — that path may have the same gap if the subprocess
   writes inner pkt-lines directly).
3. When `sideband-64k` is negotiated, wrap each report-status pkt-line:
   `<outer-pkt-line-len><band-byte=\x01><inner-pkt-line>` and emit a
   final outer flush.
4. Also handle the error path: bands 2 (progress) and 3 (error) for any
   plain-text status the server wants to emit during the response.

Unit test: extend `pktline_test.go` with a case that asserts the outer
sideband wrap is present when the input capability set includes
`sideband-64k`, and absent when it doesn't.

References:
- `internal/portal/githttp/pktline.go` — pkt-line + report-status writers
- `internal/portal/githttp/receive_pack.go:165-174` — rejection branch
- `internal/portal/githttp/receive_pack.go:240-247` — subprocess output
  flush; also potentially affected
- Pack-protocol-v2 sideband-64k: per `docs/PROTOCOL.md` (if documented)
  or upstream git's `Documentation/technical/pack-protocol.txt`
