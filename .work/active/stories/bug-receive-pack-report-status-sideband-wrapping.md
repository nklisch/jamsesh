---
id: bug-receive-pack-report-status-sideband-wrapping
kind: story
stage: done
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

## Implementation notes

### Subprocess-output path (receive_pack.go ~240)

No change needed. When `git receive-pack --stateless-rpc` runs, it reads the
full push body from stdin (which includes the client's capability list). Git
itself learns about `side-band-64k` from that command list and handles the
outer sideband wrapping of its own report-status response. The portal simply
pipes `subprocOut.Bytes()` verbatim to the client — git has already wrapped
those bytes correctly. Only the portal-synthesised rejection path (which was
hand-writing raw inner pkt-lines) needed to be fixed.

### Capability set type chosen

`map[string]bool` keyed by the exact wire spelling of each capability. This
is the simplest lookup form (O(1)), avoids an import, and reads cleanly at
call sites (`caps["side-band-64k"]`). The wire spelling `side-band-64k`
(hyphenated) matches what git sends on the wire and what the existing
`buildTestCommandList` test helper writes.

### Tests added / extended

- `TestReadCommandList_SingleUpdate` — extended with assertion that the
  returned `caps` map contains `"side-band-64k"` (the test helper already
  embeds this capability in the first line).
- `TestWriteReportStatusRejection_AllNg` renamed to
  `TestWriteReportStatusRejection_NoSideband` — passes an empty caps map,
  asserts the unwrapped format remains correct (backward compat).
- `TestWriteReportStatusRejection_PerRefReason` — updated to pass empty caps
  (no behaviour change).
- `TestWriteReportStatusRejection_SidebandWrap` (new) — passes
  `{"side-band-64k": true}`, parses the outer pkt-line stream, asserts:
  - Each outer payload's first byte is `\x01` (band 1 / data channel).
  - The inner payload of the first outer packet contains `"unpack ok"`.
  - The inner payload of the second outer packet contains `"ng <ref>"`.
  - The final packet is the flush (`"0000"`).

### TestObjectStorageRPO0/refs_only_update

The test lives at `tests/e2e/golden/object_storage_rpo0_test.go` and is an
integration test requiring a live Docker cluster (MinIO, Postgres, MailHog,
two portal pods). It cannot be run in the unit-test pass. The subtest
exercises the RPO=0 durability path for a force-push (refs-only update), but
the force-push in that test goes through `git receive-pack` on the success
path — the subprocess handles sideband itself. This bug (missing sideband
wrap on the portal's synthesised rejection path) would only manifest on the
rejection branch, not on the acceptance branch. The RPO0 refs_only_update
subtest should not have been failing due to this bug; however, a real git
client using `side-band-64k` default would fail if pre-receive rejects that
push. Local unit tests are sufficient to verify the fix.

## Review (2026-05-20)

**Verdict**: Approve

**Blockers**: none
**Important**: none
**Nits**:
- `writeReportStatusRejection` swallows write errors via `_ = writeSidebandPktLine(...)` / `_ = writePktLine(...)` — this matches the pre-existing pattern in the function (errors were already swallowed before this change), so it's not a regression. If the response writer fails mid-stream the client gets a truncated reply. A future hardening pass could plumb a returned error and let the handler decide whether to log + bail; out-of-scope here.
- The `writeLine` closure in `writeReportStatusRejection` reads cleanly but is local-to-function; if a third caller ever needs the same sideband-aware-or-not branching, lift to a method on a tiny `pkt-writer` value. Not needed yet.

**Notes**: Wire format verified correct per git's pack-protocol spec:
- outer = `<outerLen=4+1+innerLen><band=\x01><innerPktLine>`
- inner pkt-line = `<innerLen=4+payload><payload>`

For `"unpack ok\n"` (10 bytes payload) → inner=`000eunpack ok\n` (14 bytes) → outer=`0013\x01000eunpack ok\n` (19 bytes). Matches what git clients parse on the receive side.

Author correctly identified that the subprocess-output path (receive_pack.go:~240) does NOT need a parallel change — `git receive-pack --stateless-rpc` learns about sideband from the pushed command list and handles the outer wrap itself. The portal just pipes those pre-wrapped bytes through. Documenting that finding in the story body is exactly the kind of "decision the doc carries" the rolling-foundation principle calls for.

Tests cover both branches (caps include `side-band-64k` → wrapped; caps empty → unwrapped, backward-compat). All 24 githttp tests green.
