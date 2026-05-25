---
id: gate-cruft-playground-worker-errstopretrying-unused-var
kind: story
stage: implementing
tags: [cleanup]
parent: null
depends_on: []
release_binding: v0.4.0
gate_origin: cruft
created: 2026-05-24
updated: 2026-05-24
---

# playground/worker.go: errStopRetrying sentinel error has zero references

## Confidence
High

## Category
dead function

## Location
`internal/portal/playground/worker.go:146-149`

## Evidence
```go
// errStopRetrying is used internally to signal a step that cannot usefully be
// retried (e.g. the session row is already gone). It is NEVER surfaced to the
// caller — it is swallowed within Destroy() and logged if unexpected.
var errStopRetrying = errors.New("stop retrying: session already absent")
```

`grep -rn 'errStopRetrying' --include="*.go"` returns only the declaration. The doc comment claims it's "used internally" to signal an unretriable step, but no `errors.Is(err, errStopRetrying)` / `return errStopRetrying` / `if err == errStopRetrying` call exists anywhere. Go's compiler does not warn on unused package-level vars, so the dead sentinel sat undetected. `deadcode` also misses it because it only tracks unreachable funcs/methods.

## Removal
Delete the doc comment + var declaration (lines 146-149). The `errors` import is still used elsewhere in worker.go (verify with `goimports -l`), so the import line likely stays. Run `go vet ./internal/portal/playground/...` to confirm.
