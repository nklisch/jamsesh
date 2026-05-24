---
id: story-refactor-automerger-decomposition-flatten-submodule-extract
kind: story
stage: implementing
tags: [portal, refactor]
parent: feature-refactor-automerger-decomposition
depends_on: [story-refactor-automerger-decomposition-merge-file-split]
release_binding: null
gate_origin: refactor-design
created: 2026-05-23
updated: 2026-05-23
---

# Extract submodule-recursion branch from flattenTreeInto

## Brief

`internal/portal/automerger/merge.go:flattenTreeInto` (lines ~446-468) walks
a tree and accumulates entries into a flat map. The submodule branch (gitlink
mode) currently inlines a recursive sub-tree walk into the same function,
producing deep nesting and obscuring the simple case (regular blob entries).

Extract the submodule case into its own `flattenSubmodule` helper.

## Current state

```go
func flattenTreeInto(t *object.Tree, prefix string, into map[string]treeEntry) error {
    for _, e := range t.Entries {
        path := prefix + e.Name
        switch e.Mode {
        case filemode.Submodule:
            // ... inline recursive walk into sub-tree
            sub, err := /* resolve submodule */
            if err != nil { ... }
            if err := flattenTreeInto(sub, path + "/", into); err != nil { ... }
        case filemode.Dir:
            // ... walk into sub-tree
        default:
            // regular blob → record entry
            into[path] = treeEntry{hash: e.Hash, mode: e.Mode}
        }
    }
    return nil
}
```

## Target state

```go
func flattenTreeInto(t *object.Tree, prefix string, into map[string]treeEntry) error {
    for _, e := range t.Entries {
        path := prefix + e.Name
        switch e.Mode {
        case filemode.Submodule:
            if err := flattenSubmodule(e, path, into); err != nil {
                return err
            }
        case filemode.Dir:
            // ... walk into sub-tree
        default:
            into[path] = treeEntry{hash: e.Hash, mode: e.Mode}
        }
    }
    return nil
}

func flattenSubmodule(e object.TreeEntry, path string, into map[string]treeEntry) error {
    // inline body of the previous submodule branch
}
```

## Implementation notes

- This is the smallest extraction in the feature — read lines 446-468
  carefully before writing, because if the submodule branch is already a
  one-liner the extraction is not worth it. If it's > 5 LoC the extract
  adds value; if not, **close this story as "no-op land mode"** with a
  note in implementation notes.
- The submodule case may carry context that needs to flow in (e.g. a
  repo handle for object resolution) — the helper's signature reflects
  what's actually used.

## Acceptance criteria

- [ ] If the extraction is meaningful (submodule branch > 5 LoC):
  - [ ] `flattenSubmodule` exists as a package-private helper.
  - [ ] `flattenTreeInto`'s submodule case is a one-line call.
  - [ ] All existing automerger tests pass.
  - [ ] `go build ./...` clean.
- [ ] If the extraction is not meaningful (≤ 5 LoC inline):
  - [ ] Close this story with `## Implementation notes` explaining the
        decision; do not change `merge.go`.
  - [ ] Advance the story to `review` regardless.

## Risk

**Low.** This is the smallest and least load-bearing step in the feature.

## Rollback

`git revert` the commit.

## Sequencing

`depends_on: [story-refactor-automerger-decomposition-merge-file-split]`
— serial chain across all four steps to avoid concurrent edits to
`merge.go`.
