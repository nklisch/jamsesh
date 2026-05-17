---
name: go-git
description: In-process git operations via go-git v5 — three-way merge composition, commit-trailer parsing/composition, pre-receive pack inspection, ref/object walking. Auto-loads when working with the auto-merger, pre-receive validation, or any Go code importing github.com/go-git/go-git/v5. Trigger keywords - go-git, three-way merge, merge-base, MergeBase, DiffTree, commit trailer, Jam-Session, Jam-Turn, Auto-Merger, pre-receive, packfile, object walking, auto-merger merge-engine.
user-invocable: false
---

# go-git reference for jamsesh

**Module:** `github.com/go-git/go-git/v5`
**Pin:** `v5.19.0` (released 2026-05-06). Do NOT use v6 — alpha-only as of
2026-05.

**Use for:** auto-merger three-way merges, commit-trailer parsing,
pre-receive pack inspection, ref/object walking, bare-repo lifecycle
(`storage` feature), commit-graph traversal.

**Don't use for:** anything that requires `git-receive-pack` /
`git-upload-pack` — those go through subprocess invocation (see the
`git-smart-http` skill).

## The single biggest gotcha

**go-git has no real merge.** `Repository.Merge` only supports
`FastForwardMerge`. There is NO recursive/three-way merge strategy.
Upstream issue [#942](https://github.com/go-git/go-git/issues/942) is
open since 2023-11 with no implementation.

For jamsesh's auto-merger we COMPOSE three-way merge from go-git
primitives (`MergeBase`, `DiffTreeWithOptions`, blob reads) plus a
subprocess call to `git merge-file` for per-file content merging. See
`references/three-way-merge.md` for the full pattern.

## Quick API map

| Need | API |
|------|-----|
| Open bare repo | `git.PlainOpen(path)` (auto-detects bare) |
| Init bare repo | `git.PlainInit(path, true)` |
| Resolve `<ref>` or `<sha>` to hash | `repo.ResolveRevision(plumbing.Revision(s))` |
| Get commit | `repo.CommitObject(hash)` |
| Get tree from commit | `commit.Tree()` |
| Common ancestor | `commit.MergeBase(other) ([]*Commit, error)` |
| Ancestor check | `commit.IsAncestor(other) (bool, error)` |
| Diff two trees | `object.DiffTreeWithOptions(ctx, a, b, object.DefaultDiffTreeOptions)` |
| Walk commits | `object.NewCommitWalker` / `repo.Log(&git.LogOptions{From: hash})` |
| Read blob | `object.GetBlob(repo.Storer, hash)` then `b.Reader()` |
| Parse pack from bytes | `packfile.NewParser` with `memory.NewStorage()` |
| Write object | `repo.Storer.SetEncodedObject(obj)` |
| Update ref | `repo.Storer.SetReference(plumbing.NewHashReference(name, sha))` |

`DefaultDiffTreeOptions` enables rename detection at score 60 — keep
this for jamsesh; the auto-merger needs rename-aware diffs.

## Patterns

### Three-way merge (auto-merger merge-engine)

See `references/three-way-merge.md` for the full pattern. Skeleton:

```go
bases, _ := source.MergeBase(draft) // []*object.Commit
ancestor := bases[0]
baseTree, _ := ancestor.Tree()
oursTree, _ := draft.Tree()
theirsTree, _ := source.Tree()

baseToOurs, _ := object.DiffTreeWithOptions(ctx, baseTree, oursTree, object.DefaultDiffTreeOptions)
baseToTheirs, _ := object.DiffTreeWithOptions(ctx, baseTree, theirsTree, object.DefaultDiffTreeOptions)

// classify each path: ours-only, theirs-only, both-modified, delete-vs-modify
// for both-modified paths, write blobs to temp files, run `git merge-file --stdout`
// build merged tree, write blobs back via repo.Storer.SetEncodedObject
// classify outcome: Clean | SafeAutoResolve | HardConflict
```

`git merge-file` exit codes: 0 = clean, 1..127 = N conflicts (stdout has
markers), negative = error.

### Commit trailer parsing

go-git does NOT parse trailers. `commit.Message` is the raw string.

See `references/trailers.md` for the full Reader/Writer and the jamsesh
trailer schema (`Jam-Session`, `Jam-Turn`, `Jam-Author`,
`Resolves-Conflict`, `Auto-Merger`, `Source-Commit`, `Source-Ref`,
`Auto-Resolved`).

Minimal parse:

```go
// trailer block = lines after the LAST blank line, all matching `Key: value`.
// Folded continuation = line starting with whitespace.
// See `git interpret-trailers` for the rigorous 25%-rule definition.
```

### Pre-receive pack validation

Parse the pushed pack BEFORE invoking `git-receive-pack`. The system
`git-receive-pack` quarantines incoming objects in a directory go-git's
`dotgit` storage cannot read ([src-d/go-git#886](https://github.com/src-d/go-git/issues/886)).
So validate the request body directly:

```go
import (
    "github.com/go-git/go-git/v5/plumbing/format/packfile"
    "github.com/go-git/go-git/v5/storage/memory"
    "github.com/go-git/go-git/v5/plumbing/protocol/packp"
)

// 1. Decode the command-list (ref-update advertisements) from the body.
upd := packp.NewUpdateRequest()
_ = upd.Decode(reqBodyTee)

// 2. Parse the packfile into in-memory storage.
storer := memory.NewStorage()
parser, _ := packfile.NewParserWithStorage(scanner, storer)
_, _ = parser.Parse()

// 3. For each new ref tip, walk commits down to old tip.
//    Validate trailers + scope-glob path matching + ancestor check on
//    `base`/`draft` (no force pushes).
```

Build a `bytes.Buffer` (capped at `git.max_push_bytes`) of the body, or
use an `io.TeeReader` so the same bytes flow into both the validator
and the eventual subprocess stdin.

### Walking commits in a pushed range

```go
iter, _ := repo.Log(&git.LogOptions{From: newSha})
_ = iter.ForEach(func(c *object.Commit) error {
    if c.Hash == oldSha {
        return storer.ErrStop
    }
    // validate this commit
    return nil
})
```

## Known gotchas

- **No three-way merge.** Compose it. See above.
- **No trailer parsing.** Implement it. See `references/trailers.md`.
- **Quarantine directory invisible.** Don't open the bare repo to
  validate "what was just pushed" — parse the pack directly.
- **Rename detection is opt-in.** Use `DefaultDiffTreeOptions`, not
  `DiffTree` (which is the no-rename variant).
- **`MergeBase` returns a slice.** Multiple equally-good ancestors are
  possible. Pick deterministically (first by commit date is what go-git
  already does internally).
- **`Storer.SetEncodedObject` for writes.** Don't try to write through
  the `Worktree` — bare repos return `ErrIsBareRepository`.
- **Empty ancestor case.** Two commits with no common ancestor (e.g.
  pushed against a freshly-initialized `draft`) — `MergeBase` returns
  empty slice without error. Auto-merger must handle: treat as "all of
  source is new" and fast-forward draft.
- **Performance on large repos.** `Log` with no path filter walks the
  whole history. For per-session repos in jamsesh this is bounded, but
  if a session grows past ~10k commits consider scoping the walk by
  `Since` / commit-date.

## Foundation references

- `docs/SPEC.md` — go-git locked in the Stack section
- `docs/research/git-internals-stack.md` — full research notes
- `.work/active/epics/epic-auto-merger.md` — three-way merge requirements,
  safe-auto-resolve heuristics
- `.work/active/epics/epic-portal-git.md` — pre-receive design, bare-repo
  storage

## References (deeper material)

- `references/three-way-merge.md` — auto-merger merge-engine pattern
- `references/trailers.md` — jamsesh trailer schema + parser/composer
