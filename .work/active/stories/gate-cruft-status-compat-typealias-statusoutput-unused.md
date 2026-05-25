---
id: gate-cruft-status-compat-typealias-statusoutput-unused
kind: story
stage: review
tags: [cleanup]
parent: null
depends_on: []
release_binding: v0.4.0
gate_origin: cruft
created: 2026-05-24
updated: 2026-05-24
---

# sessioncmd/status.go: statusOutput backward-compat type alias has no callers

## Confidence
High

## Category
compatibility shim

## Location
`cmd/jamsesh/sessioncmd/status.go:69-71`

## Evidence
```go
// statusOutput is kept for --json backward compat in single-session tests only.
// The new public JSON shape is statusJSONOutput.
type statusOutput = durableStatusOutput
```

`grep -rn '\bstatusOutput\b' --include="*.go" cmd/` returns ONLY the declaration site and the stale comment on line 38 ("Fields match the former single-session statusOutput for backward compat"). No test references it, no production code references it. The comment claims it's "kept for single-session tests only" but those tests do not in fact import or use the alias — they construct `statusJSONOutput` directly.

## Removal
1. Delete the `statusOutput` type alias and its two-line doc comment (lines 69-71).
2. Strip the stale reference on line 38 (`// Fields match the former single-session statusOutput for backward compat.`) — replace with a one-line description of `durableStatusOutput`'s purpose ("Per-session entry in the --json `durable` array.") or drop the line entirely.

Run `go build ./... && go test ./cmd/jamsesh/sessioncmd/...` to confirm no fallout.

## Implementation notes
Deleted `type statusOutput = durableStatusOutput` and its docstring. Replaced the two-line `durableStatusOutput` comment with a single accurate one-liner. `go build ./...` and `go test ./cmd/jamsesh/sessioncmd/...` pass.
