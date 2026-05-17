# Three-way merge composition

go-git has no native three-way merge. This is the jamsesh pattern.

## Inputs

- `source *object.Commit` — the just-pushed commit on a sync ref
- `draft *object.Commit` — current `jam/<session>/draft` tip
- `repo *git.Repository` — opened on the session bare repo

## Outputs

```go
type Result struct {
    Kind       Kind          // Clean | SafeAutoResolve | HardConflict
    MergedTree plumbing.Hash // populated unless HardConflict
    Heuristic  string        // "whitespace" | "additions" | "identical" | ""
    Conflicts  []FileConflict
}
```

The auto-merger's `outcomes` feature consumes `Result`:

- `Clean` or `SafeAutoResolve` → create merge commit with the trailers
  documented below, advance `draft`, emit `merge.succeeded`.
- `HardConflict` → emit `conflict.detected` with `Conflicts` as the
  payload's `conflicts` field, do NOT advance `draft`.

## Steps

### 1. Find ancestor

```go
bases, err := source.MergeBase(draft)
if err != nil { return nil, err }
if len(bases) == 0 {
    // No common ancestor — treat source as fast-forwardable onto draft.
    // (This happens on the very first push to a session.)
    return &Result{Kind: Clean, MergedTree: source.TreeHash}, nil
}
ancestor := bases[0]
```

### 2. Trees

```go
baseTree, _ := ancestor.Tree()
oursTree, _ := draft.Tree()   // "ours" = draft tip
theirsTree, _ := source.Tree() // "theirs" = the incoming commit
```

Convention matches `git merge-file` argument order: ours / base / theirs.

### 3. Pairwise diffs

```go
baseToOurs, err := object.DiffTreeWithOptions(
    ctx, baseTree, oursTree, object.DefaultDiffTreeOptions)
baseToTheirs, err := object.DiffTreeWithOptions(
    ctx, baseTree, theirsTree, object.DefaultDiffTreeOptions)
```

### 4. Per-path classification

Build a map `path → (ourChange, theirChange)`. For each path:

- **Only theirs changed** → take theirs.
- **Only ours changed** → take ours.
- **Both changed, both deletes** → take delete.
- **Both add the same content** → take either (identical edits
  heuristic — emit `Auto-Resolved: identical`).
- **Both modified the file** → run per-file content merge (step 5).
- **One side deleted, other modified** → hard conflict, no heuristic.
- **Rename + modify on the other side** → hard conflict (let
  rename-detection surface it).

### 5. Per-file content merge

For both-modified files:

```go
func mergeFile(base, ours, theirs []byte) (merged []byte, conflicts int, err error) {
    bF, _ := writeTemp(base)
    oF, _ := writeTemp(ours)
    tF, _ := writeTemp(theirs)
    defer os.Remove(bF)
    defer os.Remove(oF)
    defer os.Remove(tF)

    cmd := exec.Command("git", "merge-file",
        "--stdout",
        "-L", "ours", "-L", "base", "-L", "theirs",
        oF, bF, tF)
    out, err := cmd.Output()
    if err == nil {
        return out, 0, nil
    }
    if ee, ok := err.(*exec.ExitError); ok && ee.ExitCode() > 0 {
        return out, ee.ExitCode(), nil // N conflicts; out has markers
    }
    return nil, -1, err
}
```

Exit code semantics (from `git-merge-file(1)`):

- `0` → clean
- `1..127` → that-many conflict hunks (output contains
  `<<<<<<<`/`=======`/`>>>>>>>` markers)
- negative / non-exitError → real error (treat as `HardConflict`)

### 6. Heuristic classification on conflict markers

When `mergeFile` returns conflicts > 0, the auto-merger inspects the
marker blocks and decides whether they're safe to auto-resolve:

- **whitespace-only** — `ours` block and `theirs` block differ only in
  whitespace (trailing whitespace, LF vs CRLF, tab vs space NOT
  changing indentation depth). Auto-resolve to either side (pick
  ours by convention).
- **non-overlapping additions** — both sides ADD lines with neither
  modifying or deleting a shared line. Interleave both side's added
  lines preserving order.
- **identical edits** — both sides produced the same text. Take either.

Anything else → `HardConflict`. Parse the conflict markers' line
positions to build `FileConflict.Ranges` for the event payload.

### 7. Materialize the merged tree

For each path in the merge plan:
- Write the merged blob via `repo.Storer.SetEncodedObject` and capture
  the new hash.
- Assemble the tree by walking `baseTree` and substituting changed
  entries. Subdirectories are recursive — easiest to build the full
  tree by walking the union of all changed paths and constructing
  parent trees bottom-up.

Write each tree object via `repo.Storer.SetEncodedObject`. The root
tree hash is `MergedTree`.

### 8. Compose the merge commit (in `outcomes` feature)

```go
import "<jamsesh>/internal/trailer"

trailers := []trailer.Trailer{
    {Key: "Auto-Merger", Value: "true"},
    {Key: "Source-Commit", Value: source.Hash.String()},
    {Key: "Source-Ref", Value: sourceRef},
}
if result.Heuristic != "" {
    trailers = append(trailers, trailer.Trailer{
        Key: "Auto-Resolved", Value: result.Heuristic,
    })
}

mergeCommit := &object.Commit{
    Author:    source.Author,    // human author preserved
    Committer: object.Signature{
        Name:  "jamsesh auto-merger",
        Email: "auto-merger@" + portalHost,
        When:  time.Now().UTC(),
    },
    Message:      trailer.Compose(fmt.Sprintf("Merge %s into draft",
                                              source.Hash.String()[:7]),
                                  trailers),
    TreeHash:     result.MergedTree,
    ParentHashes: []plumbing.Hash{draft.Hash, source.Hash},
}

obj := repo.Storer.NewEncodedObject()
_ = mergeCommit.Encode(obj)
newSha, _ := repo.Storer.SetEncodedObject(obj)

// Advance draft.
_ = repo.Storer.SetReference(plumbing.NewHashReference(
    plumbing.ReferenceName("refs/heads/jam/"+sessionID+"/draft"),
    newSha,
))
```

## Test corpus

`epic-auto-merger.md` mandates an adversarial corpus locked at design
time. Include at minimum:

- Pure whitespace-only conflict (trailing space + LF/CRLF mix)
- Whitespace-only but indentation-depth-changing (must NOT auto-resolve)
- Non-overlapping adds in the same hunk
- Overlapping adds where one side also modifies a shared line (must NOT
  auto-resolve)
- Identical edits (both sides made the exact same change)
- Identical edits with surrounding context drift
- Delete-vs-modify (always hard)
- Rename + modify (always hard)
- Binary file conflict (always hard; `git merge-file` errors on binary)
- Empty common ancestor (first push to fresh session)
