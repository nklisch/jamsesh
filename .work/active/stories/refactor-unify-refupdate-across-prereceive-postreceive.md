---
id: refactor-unify-refupdate-across-prereceive-postreceive
kind: story
stage: implementing
tags: [refactor, cleanup, portal, git]
parent: null
depends_on: []
release_binding: null
gate_origin: refactor-design
created: 2026-05-20
updated: 2026-05-20
---

# Unify `RefUpdate` across `prereceive` and `postreceive` packages

## Brief

`internal/portal/prereceive/types.go` and `internal/portal/postreceive/emitter.go`
each define a `RefUpdate` struct with identical fields:

```go
// prereceive/types.go
type RefUpdate struct {
    Ref    string // "refs/heads/jam/<session>/<owner>/<branch>"
    OldSHA string // empty if new ref
    NewSHA string
}

// postreceive/emitter.go
type RefUpdate struct {
    Ref    string // e.g. "refs/heads/jam/<session>/<owner>/<branch>"
    OldSHA string // empty if new ref
    NewSHA string
}
```

The duplication forces a 1:1 translation shim at the only crossing point:
`toEmitterUpdates` in `internal/portal/githttp/receive_pack.go:~447`:

```go
func toEmitterUpdates(in []prereceive.RefUpdate) []postreceive.RefUpdate {
    out := make([]postreceive.RefUpdate, len(in))
    for i, u := range in {
        out[i] = postreceive.RefUpdate{Ref: u.Ref, OldSHA: u.OldSHA, NewSHA: u.NewSHA}
    }
    return out
}
```

## Finding origin

Surfaced by the `refactor-design` discovery pass triggered by autopilot Phase 7
on 2026-05-20 against `internal/portal/githttp/pktline.go` and
`internal/portal/githttp/receive_pack.go`. The translator function inside
receive_pack.go is the visible symptom; the duplication is upstream in the two
type-defining packages.

## Fix scope

1. Create `internal/portal/gitref/types.go` (new package) with the single
   shared `RefUpdate` struct definition.
2. Delete `RefUpdate` from `internal/portal/prereceive/types.go` and
   `internal/portal/postreceive/emitter.go`. Replace every usage in each
   package — `prereceive/{validate,refs,commits,types}.go` + their tests,
   `postreceive/emitter.go` + its tests — with `gitref.RefUpdate`.
3. Delete `toEmitterUpdates` from `internal/portal/githttp/receive_pack.go`.
   Update the call site to pass `updates` directly to the emitter.
4. Verify all tests pass: `go test ./internal/portal/...`.

## Acceptance criteria

- [ ] `go build ./...` clean
- [ ] `go test ./internal/portal/...` green
- [ ] `grep -rn 'type RefUpdate' internal/portal/` shows exactly one definition
  (in `gitref/`)
- [ ] `grep -rn 'toEmitterUpdates' internal/portal/` returns no matches

## Risk

Low. Behavior-preserving — same fields, same shape, same passing semantics.
The only risk is missing an import update in a test file, which the test
suite catches immediately.

## Rollback

If a downstream caller turns out to depend on the type identity (e.g.,
type assertions in tests that distinguish the two packages' types), revert
the commits via `git revert` and reintroduce the translator.

## Out of scope

A larger consolidation that pulls the prereceive/postreceive packages
themselves into a shared `gitlifecycle` package is NOT in scope here — those
packages have meaningfully different responsibilities (validation vs event
emission) and should stay separate.
