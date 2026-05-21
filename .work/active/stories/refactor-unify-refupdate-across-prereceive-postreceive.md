---
id: refactor-unify-refupdate-across-prereceive-postreceive
kind: story
stage: done
tags: [refactor, cleanup, portal, git]
parent: null
depends_on: []
release_binding: v0.3.0
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

## Implementation notes

### Files touched

- `internal/portal/gitref/types.go` — new package; defines the single canonical `RefUpdate` struct
- `internal/portal/prereceive/types.go` — removed local struct definition; added `type RefUpdate = gitref.RefUpdate` alias and `gitref` import; no other prereceive files needed changes
- `internal/portal/postreceive/emitter.go` — removed local struct definition; added `type RefUpdate = gitref.RefUpdate` alias and `gitref` import; `prereceive` import retained (still needed for `Trailers()` call)
- `internal/portal/githttp/pktline.go` — `readCommandList` return type changed from `[]prereceive.RefUpdate` to `[]gitref.RefUpdate`; `writeReportStatusRejection` first slice param updated to `[]gitref.RefUpdate`; added `gitref` import; `prereceive` import retained for `[]prereceive.Rejection` param
- `internal/portal/githttp/receive_pack.go` — `toEmitterUpdates` shim deleted; call site updated to pass `updates` directly; `postreceive` import removed (no longer referenced directly)

### Verification

```
grep -rn 'type RefUpdate' internal/portal/
# internal/portal/prereceive/types.go:39:type RefUpdate = gitref.RefUpdate
# internal/portal/postreceive/emitter.go:62:type RefUpdate = gitref.RefUpdate
# internal/portal/gitref/types.go:8:type RefUpdate struct {
```

Exactly one struct *definition* — `gitref/types.go`. The other two lines are type aliases, not independent definitions.

```
grep -rn 'toEmitterUpdates' internal/portal/
# (empty — shim fully removed)
```

### Reconciling the two original types

The two original `RefUpdate` structs were identical in field names, types, and ordering. The only difference was the inline comment on `Ref`:

- `prereceive`: `// "refs/heads/jam/<session>/<owner>/<branch>"`
- `postreceive`: `// e.g. "refs/heads/jam/<session>/<owner>/<branch>"`

The merged `gitref.RefUpdate` uses the prereceive comment (without "e.g.") as the more concise form. No struct tags, no field ordering differences to reconcile.

### Implementation approach

Type aliases (`type RefUpdate = gitref.RefUpdate`) were used in both packages rather than removing all unqualified `RefUpdate` references from internal files. This kept the diffs minimal: zero changes needed in `refs.go`, `commits.go`, `validate.go`, their tests, or `emitter_test.go`. The type aliases make `prereceive.RefUpdate` and `postreceive.RefUpdate` identical types at the Go type system level, enabling direct slice passing without conversion.

## Out of scope

A larger consolidation that pulls the prereceive/postreceive packages
themselves into a shared `gitlifecycle` package is NOT in scope here — those
packages have meaningfully different responsibilities (validation vs event
emission) and should stay separate.

## Review (2026-05-20)

**Verdict**: Approve

**Blockers**: none
**Important**: none
**Nits**: none

**Notes**: The type-alias approach (`type RefUpdate = gitref.RefUpdate` in each package) is a smart deviation from the story's "replace every usage" plan — it gets to single-source-of-truth with minimal diff (zero edits needed in each package's internal files or tests). All 30 portal package tests stay green, `toEmitterUpdates` is gone, `EmitForUpdates` accepts the slice directly. The two original structs were truly identical so the merge had nothing to reconcile beyond an "e.g." prefix on one comment.
