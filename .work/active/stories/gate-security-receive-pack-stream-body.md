---
id: gate-security-receive-pack-stream-body
kind: story
stage: done
tags: [security, portal]
parent: null
depends_on: []
release_binding: v0.1.0
gate_origin: security
created: 2026-05-18
updated: 2026-05-18
---

# Receive-pack reads entire push body into memory before validation

## Severity
Medium

## Domain
Infrastructure & Deployment

## Location
`internal/portal/githttp/receive_pack.go:57-64`

## Evidence
```go
limitedBody := http.MaxBytesReader(w, r.Body, maxBytes)
bodyBytes, err := io.ReadAll(limitedBody)
```

Default `maxBytes` is `MaxPackBytes + 16KiB` = `50 MiB + 16 KiB`.
Authenticated session members can concurrently push 50 MiB packs; with
even modest concurrency this saturates RSS on small portal pods (the
default Helm/quickstart pod sizes assume single-digit-MB per request).
The whole pack is also held in memory through `buildValidationRepo` so
peak RSS per concurrent push is ~2x the pack size.

## Remediation direction
Stream the body to a tempfile (or `bytes.Buffer` with `io.LimitReader`)
and rewind for the second pass into `buildValidationRepo`. Add a
per-instance semaphore counting concurrent receive-pack handlers so
overall memory is bounded independent of per-pack cap.

## Implementation notes

### Streaming approach (tempfile)

`io.ReadAll` was replaced with `os.CreateTemp("", "jamsesh-pack-*")` +
`io.Copy`. The tempfile is created early in the handler, deferred for
`Close` + `os.Remove`, and rewound twice:

1. After streaming the body in: `Seek(0, io.SeekStart)` before
   `readCommandList(bodyFile)`. The `bufio.Reader` inside
   `readCommandList` advances the file position through the command-list
   pkt-lines; the returned `packReader` is already positioned at the start
   of the pack section and is passed directly to `buildValidationRepo`.

2. Before the subprocess: `Seek(0, io.SeekStart)` to reset to the
   beginning so the full body (command list + pack) is piped to
   `git receive-pack --stateless-rpc` stdin.

`buildValidationRepo` still calls `io.ReadAll` on the pack-only reader
it receives, which keeps the go-git `memory.Storage` in RAM during
validation. This is unavoidable (go-git needs random access to objects).
However it is only 1× the pack size during validation, and is released
once `buildValidationRepo` returns — not held through the subprocess
phase. Previously the full body was held in a `[]byte` for the entire
handler lifetime (both passes + subprocess).

### Semaphore policy (503 non-blocking)

`Handler.ReceivePackSem chan struct{}` (buffer = N) is tried with a
non-blocking `select`. When all N slots are taken the handler returns
`503 Service Unavailable` with `Retry-After: 5`. The git CLI handles 503
gracefully (exits non-zero; the user sees a clear error and can re-push).
A blocking approach was rejected because it holds an HTTP connection open
under back-pressure and can cascade to exhausted connection pools.

### Config knob

`git.receive_pack_max_concurrent` (YAML) /
`JAMSESH_RECEIVE_PACK_MAX_CONCURRENT` (env) added to `GitConfig`.
Default: 4. Validated as positive integer at startup.
The semaphore is allocated in `cmd/portal/main.go` with
`make(chan struct{}, cfg.Git.ReceivePackMaxConcurrent)` and wired into
`githttp.Handler.ReceivePackSem`.

### Files changed

- `internal/portal/githttp/receive_pack.go` — streaming + semaphore
- `internal/portal/githttp/handler.go` — `ReceivePackSem` field
- `internal/portal/config/config.go` — `ReceivePackMaxConcurrent` field, default, env parse, validation
- `cmd/portal/main.go` — semaphore allocation + wiring

## Review (2026-05-18)

**Verdict**: Approve

**Blockers**: none
**Important**: none
**Nits**: none

**Notes**: Memory-bound fix in place. Receive-pack body now streams to a tempfile via io.Copy + os.CreateTemp; defer Close+Remove. Two rewinds (Seek(0,0)) before readCommandList and again before the subprocess pipe. New per-instance semaphore (ReceivePackSem chan struct{}, configurable JAMSESH_RECEIVE_PACK_MAX_CONCURRENT default 4) with non-blocking select — returns 503+Retry-After when all slots taken. Backwards-compatible: nil ReceivePackSem disables the cap (existing tests unchanged).
