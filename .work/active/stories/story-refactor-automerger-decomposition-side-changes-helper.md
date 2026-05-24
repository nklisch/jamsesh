---
id: story-refactor-automerger-decomposition-side-changes-helper
kind: story
stage: implementing
tags: [portal, refactor]
parent: feature-refactor-automerger-decomposition
depends_on: []
release_binding: null
gate_origin: refactor-design
created: 2026-05-23
updated: 2026-05-23
---

# Extract buildSideChanges helper from computeMergeDiff

## Brief

`internal/portal/automerger/merge.go:computeMergeDiff` contains two
near-identical loops (lines 160-180 and 182-202) that walk a
`object.Changes` list, extract `from`/`to` files, build `*sideChange`
records, and store them in a `map[string]*sideChange`. The only
difference between the loops is the destination map name (`ourChanges`
vs `theirChanges`) and the error-message prefix.

Extract a shared `buildSideChanges` helper.

## Current state

```go
ourChanges := make(map[string]*sideChange)
for _, ch := range baseToOurs {
    from, to, err := ch.Files()
    if err != nil {
        return mergeDiff{}, fmt.Errorf("automerger: ours change files: %w", err)
    }
    sc := &sideChange{}
    if from != nil {
        sc.fromHash = from.Blob.Hash
        sc.fromPath = from.Name
        sc.mode = from.Mode
    }
    if to != nil {
        sc.toHash = to.Blob.Hash
        sc.toPath = to.Name
        sc.mode = to.Mode
    }
    sc.deleted = (to == nil)
    sc.added = (from == nil)
    ourChanges[effectivePath(sc)] = sc
}

theirChanges := make(map[string]*sideChange)
for _, ch := range baseToTheirs {
    // ... same shape with "theirs" error prefix
}
```

## Target state

```go
// At file scope or in a sibling file:
func buildSideChanges(changes object.Changes, side string) (map[string]*sideChange, error) {
    out := make(map[string]*sideChange)
    for _, ch := range changes {
        from, to, err := ch.Files()
        if err != nil {
            return nil, fmt.Errorf("automerger: %s change files: %w", side, err)
        }
        sc := &sideChange{}
        if from != nil {
            sc.fromHash, sc.fromPath, sc.mode = from.Blob.Hash, from.Name, from.Mode
        }
        if to != nil {
            sc.toHash, sc.toPath, sc.mode = to.Blob.Hash, to.Name, to.Mode
        }
        sc.deleted = (to == nil)
        sc.added = (from == nil)
        out[effectivePath(sc)] = sc
    }
    return out, nil
}

// In computeMergeDiff:
ourChanges, err := buildSideChanges(baseToOurs, "ours")
if err != nil {
    return mergeDiff{}, err
}
theirChanges, err := buildSideChanges(baseToTheirs, "theirs")
if err != nil {
    return mergeDiff{}, err
}
```

## Implementation notes

- The helper is package-private (lowercase `buildSideChanges`).
- Error wording preserved via the `side` parameter — current strings are
  "automerger: ours change files: %w" and "automerger: theirs change files: %w".
- After extraction, `computeMergeDiff` shrinks by ~30 lines.

## Acceptance criteria

- [ ] `computeMergeDiff` calls `buildSideChanges` twice instead of inlining
      the loop.
- [ ] Error messages emit identical strings as before (verifiable by
      running existing automerger tests).
- [ ] `go build ./...` clean.
- [ ] `go test ./internal/portal/automerger/...` clean.

## Risk

**Low.** Mechanical extraction, single file, well-tested package.

## Rollback

`git revert` the commit.
