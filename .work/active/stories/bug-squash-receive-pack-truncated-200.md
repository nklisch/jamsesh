---
id: bug-squash-receive-pack-truncated-200
kind: story
stage: done
tags: [bug, portal, error-handling]
parent: epic-bug-squash-handler-error-classification
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-30
updated: 2026-05-30
bug_origin: scan
bug_severity: medium
bug_domain: error-handling
bug_location: internal/portal/githttp/receive_pack.go:228
---

# Receive-pack returns HTTP 200 with possibly-truncated output; stdin-copy error discarded

**Location**: `internal/portal/githttp/receive_pack.go:228` · **Severity**: medium · **Pattern**: close/read error ignored on a write path; wrong success-reporting

The error from `io.Copy(stdin, bodyFile)` is sent on `stdinErrCh` and then thrown away (`<-stdinErrCh`); the `io.Copy(&subprocOut, stdout)` error is also ignored. If feeding the pack to `git receive-pack` fails mid-write (read error after seek, broken pipe) the subprocess may have consumed a truncated body, and a truncated report-status read is treated as complete. The success branch then writes 200 OK with whatever buffered — so a partially-failed push can be acknowledged to the client as success. Fix: check the stdin-copy error from `stdinErrCh` and the stdout copy error; only emit 200 when the body was fully fed and stdout fully read, else fail with 500.

```go
go func() { defer stdin.Close(); _, err := io.Copy(stdin, bodyFile); stdinErrCh <- err }()
io.Copy(&subprocOut, stdout) //nolint:errcheck
cmdErr := cmd.Wait()
<-stdinErrCh // error value discarded
w.WriteHeader(http.StatusOK); _, _ = w.Write(subprocOut.Bytes())

## Implementation notes

Updated `receive_pack.go` to capture `stdoutErr` from `io.Copy`, drain
`stdinErr := <-stdinErrCh`, and classify:
- `stdoutErr != nil` → 500 (truncated report read)
- `cmdErr == nil && stdinErr != nil` → 500 (impossible-success: clean exit but
  stdin not fully fed → don't acknowledge)
- `cmdErr != nil && !looksLikeReportStatus(subprocOut.Bytes())` → 500 (crash,
  no report; was previously a false 200)
- `cmdErr != nil && looksLikeReportStatus(...)` → 200+report (git-level
  rejection: hook/non-ff; protocol-correct behavior preserved)

Added `looksLikeReportStatus(buf []byte) bool` helper at end of file: requires
a 4-hex-digit pkt-line length prefix AND at least one of `unpack `, `ng `,
`ok ` keywords — conservative so malformed/absent reports → 500.

Tests added:
- `receive_pack_internal_test.go`: `TestLooksLikeReportStatus` with 11 cases
  covering empty, too-short, valid pktline, non-hex prefix, missing keywords,
  real rejection shape
- `receive_pack_test.go`: `TestReceivePack_GitRejection_Returns200WithReport`
  (regression guard: git-level rejection via pre-receive → 200+report preserved)
  and `TestReceivePack_MalformedBody_Returns500NotFalse200` (empty body → not 200)
```
