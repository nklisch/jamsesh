---
id: bug-scan-receive-pack-truncated-200
created: 2026-05-30
tags: [bug, error-handling]
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
```
