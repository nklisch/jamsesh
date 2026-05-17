# Git Internals Stack — Research Notes

Status: research-only, captured 2026-05-16. Stack choices already locked in
`docs/SPEC.md` (go-git for in-process operations) and
`.work/active/epics/epic-portal-git.md` (subprocess invocation of
`git-receive-pack` / `git-upload-pack`). This document captures the current
API surface, the critical gotcha around three-way merge, and ready-to-paste
patterns for the auto-merger, pre-receive, and smart-HTTP code paths.

---

## 1. go-git — current state

**Module path:** `github.com/go-git/go-git/v5`
**Pinned version:** `v5.19.0` (released 2026-05-06).
**Maintenance health:** Actively maintained. Backed by GitSight and Entire.
4,979 known importers per pkg.go.dev. Still receiving releases on roughly a
monthly cadence in 2025–2026.
**Avoid:** `v6.0.0-alpha.3` exists but is explicitly pre-release. The
plumbing transport API is being refactored under v6 — lock to v5.19.x for
jamsesh v1.

### CRITICAL: go-git does NOT implement three-way merge

This is the single biggest finding of this research. go-git's
`Repository.Merge(ref, MergeOptions{Strategy: ...})` accepts a
`MergeStrategy`, but the only implemented strategy is `FastForwardMerge`.
There is no recursive / three-way merge strategy. The README itself says:

> "go-git aims to reach the completeness of libgit2 or jgit, nowadays
> covers the majority of the plumbing read operations and some of the main
> write operations, **but lacks the main porcelain operations such as
> merges**."

The upstream tracking issue is
[go-git#942 — Add Merge method](https://github.com/go-git/go-git/issues/942),
open since November 2023 with no committed implementation.

**Implication for jamsesh.** The auto-merger cannot call a `Merge` method
and get a real three-way merge. We must compose one ourselves from the
primitives go-git does provide, plus a per-file content merge that calls
out to `git merge-file` as a subprocess. This is the same shape Gitaly,
Gerrit, and almost every other Go-ecosystem git server has landed on.

**The composition:**

1. **Find the ancestor.** Use `(*object.Commit).MergeBase(other)` — go-git
   ships this in `plumbing/object/merge_base.go`. It mirrors
   `git merge-base` and returns `[]*Commit` (multiple ancestors are
   possible; auto-merger picks the first deterministically).
2. **Diff trees pairwise.** `object.DiffTreeWithOptions(ctx, base, ours,
   opts)` and `object.DiffTreeWithOptions(ctx, base, theirs, opts)`
   classify path-level changes (add/modify/delete/rename). Use
   `DefaultDiffTreeOptions` (rename detection on, score 60).
3. **Classify per-path interactions.** For each path appearing in either
   diff: ours-only or theirs-only → take the change; both-modified →
   needs per-file content merge; delete-vs-modify → hard conflict.
4. **Per-file content merge.** Write `ancestor`, `ours`, `theirs` blobs to
   temp files, invoke `git merge-file --stdout -L ours -L base -L theirs`
   as a subprocess. Exit code semantics:
   - `0` = clean merge, stdout is the merged content
   - `1..127` = N conflicts, stdout contains conflict markers
   - negative = error (treat as hard conflict)
5. **Safe-auto-resolve detection.** Run the conflict markers through
   jamsesh's heuristic classifier (whitespace-only / non-overlapping
   additions / identical edits per `epic-auto-merger.md`). Only those
   three cases auto-resolve; everything else emits `conflict.detected`.
6. **Materialize the merge commit.** Build the merged tree by calling
   `repo.Storer.SetEncodedObject(...)` for each merged blob, then
   composing the tree via `object.Tree` construction (or the
   `filemode/treeentry` helpers). Create the commit object with two
   parents (`theirs` then `ours` — source commit first by convention) and
   the trailers documented below.

### Three-way merge — code snippet (in-process)

```go
package merge

import (
    "context"
    "errors"
    "fmt"
    "os"
    "os/exec"

    "github.com/go-git/go-git/v5"
    "github.com/go-git/go-git/v5/plumbing"
    "github.com/go-git/go-git/v5/plumbing/object"
)

// Result is what the auto-merger's merge-engine returns.
type Result struct {
    Kind       Kind                     // Clean | SafeAutoResolve | HardConflict
    MergedTree plumbing.Hash            // populated unless HardConflict
    Heuristic  string                   // "whitespace" | "additions" | "identical" | ""
    Conflicts  []FileConflict           // populated on HardConflict
}

type Kind int

const (
    Clean Kind = iota
    SafeAutoResolve
    HardConflict
)

type FileConflict struct {
    Path   string
    Ranges []LineRange
}

type LineRange struct{ Start, End int }

// ThreeWay runs the auto-merger algorithm. Caller owns the repository
// handle and the storage. All blob/tree/commit objects are read via
// the repo's storer; the merged blobs are written back via the storer.
func ThreeWay(
    ctx context.Context,
    repo *git.Repository,
    source, draft *object.Commit,
) (*Result, error) {
    bases, err := source.MergeBase(draft)
    if err != nil {
        return nil, fmt.Errorf("merge-base: %w", err)
    }
    if len(bases) == 0 {
        return nil, errors.New("no common ancestor")
    }
    ancestor := bases[0]

    aTree, _ := ancestor.Tree()
    sTree, _ := source.Tree()
    dTree, _ := draft.Tree()

    // Pairwise diffs to classify which paths interact.
    fromBaseToSource, err := object.DiffTreeWithOptions(
        ctx, aTree, sTree, object.DefaultDiffTreeOptions)
    if err != nil {
        return nil, fmt.Errorf("diff base→source: %w", err)
    }
    fromBaseToDraft, err := object.DiffTreeWithOptions(
        ctx, aTree, dTree, object.DefaultDiffTreeOptions)
    if err != nil {
        return nil, fmt.Errorf("diff base→draft: %w", err)
    }

    plan := classify(fromBaseToSource, fromBaseToDraft) // returns per-path action

    // For each path with both-sides-modified, invoke git merge-file.
    // For ours-only / theirs-only paths, take the side that changed.
    // Build the merged tree as we go; track conflicts; track heuristic.
    return materialize(ctx, repo, plan, ancestor, source, draft)
}

// mergeFile runs `git merge-file --stdout` on three blobs.
// Returns merged content and number of conflicts.
func mergeFile(base, ours, theirs []byte) ([]byte, int, error) {
    baseF, _ := writeTemp(base)
    oursF, _ := writeTemp(ours)
    theirsF, _ := writeTemp(theirs)
    defer os.Remove(baseF)
    defer os.Remove(oursF)
    defer os.Remove(theirsF)

    cmd := exec.Command("git", "merge-file",
        "--stdout",
        "-L", "ours", "-L", "base", "-L", "theirs",
        oursF, baseF, theirsF)
    out, err := cmd.Output()
    if err == nil {
        return out, 0, nil
    }
    if ee, ok := err.(*exec.ExitError); ok {
        code := ee.ExitCode()
        if code > 0 {
            return out, code, nil // N conflicts, output has markers
        }
    }
    return nil, -1, err
}

func writeTemp(content []byte) (string, error) {
    f, err := os.CreateTemp("", "jam-merge-*")
    if err != nil {
        return "", err
    }
    _, _ = f.Write(content)
    _ = f.Close()
    return f.Name(), nil
}
```

`classify` and `materialize` are jamsesh-specific glue documented inline
in `epic-auto-merger-merge-engine`'s implementation; they are NOT shown
here in full to keep the snippet load-bearing.

### Commit trailer parsing & composition

go-git does NOT parse trailers. The `Commit.Message` field carries the
full message verbatim. We must parse and compose trailers ourselves.

**Trailer block definition** (per `git interpret-trailers` docs):

- A group of one or more lines at the end of the message.
- Preceded by at least one blank line.
- Either every line is `Key: value`, or the group is at least 25% trailers
  and contains at least one well-known trailer.
- Folded continuation lines start with whitespace.
- Default separator is `: ` (colon space).

**Reader/Writer pattern.**

```go
package trailer

import (
    "regexp"
    "strings"

    "github.com/go-git/go-git/v5/plumbing/object"
)

var trailerRE = regexp.MustCompile(`^([A-Za-z][A-Za-z0-9-]*):\s+(.+)$`)

type Trailer struct {
    Key   string
    Value string
}

// Parse extracts the trailer block from a commit message.
// Returns trailers in order; duplicate keys preserved.
func Parse(msg string) []Trailer {
    lines := strings.Split(strings.TrimRight(msg, "\n"), "\n")
    // Find the last blank line; everything after must be all-trailer
    // lines (with folded-continuation support) to qualify.
    last := -1
    for i := len(lines) - 1; i >= 0; i-- {
        if strings.TrimSpace(lines[i]) == "" {
            last = i
            break
        }
    }
    if last < 0 || last == len(lines)-1 {
        return nil
    }
    block := lines[last+1:]
    var out []Trailer
    var cur *Trailer
    for _, l := range block {
        if (l[0] == ' ' || l[0] == '\t') && cur != nil {
            cur.Value += "\n" + l
            continue
        }
        m := trailerRE.FindStringSubmatch(l)
        if m == nil {
            return nil // not a pure trailer block; bail
        }
        out = append(out, Trailer{Key: m[1], Value: m[2]})
        cur = &out[len(out)-1]
    }
    return out
}

// Compose returns a message with trailers appended (blank line before).
// Existing trailers in the message are preserved as-is; new ones are
// concatenated. Use Parse + filter if you need to replace.
func Compose(body string, trailers []Trailer) string {
    body = strings.TrimRight(body, "\n")
    var sb strings.Builder
    sb.WriteString(body)
    sb.WriteString("\n\n")
    for _, t := range trailers {
        sb.WriteString(t.Key)
        sb.WriteString(": ")
        sb.WriteString(t.Value)
        sb.WriteString("\n")
    }
    return sb.String()
}

// Find returns the first value for a key, or empty string.
func Find(msg, key string) string {
    for _, t := range Parse(msg) {
        if t.Key == key {
            return t.Value
        }
    }
    return ""
}

// Validate enforces jamsesh's required trailers.
func ValidateRequired(c *object.Commit) error {
    required := []string{"Jam-Session", "Jam-Turn", "Jam-Author"}
    have := map[string]bool{}
    for _, t := range Parse(c.Message) {
        have[t.Key] = true
    }
    var missing []string
    for _, r := range required {
        if !have[r] {
            missing = append(missing, r)
        }
    }
    if len(missing) > 0 {
        return &MissingTrailerErr{Missing: missing, Commit: c.Hash}
    }
    return nil
}
```

The auto-merger composes its merge commits via `Compose`:

```go
auto := []trailer.Trailer{
    {Key: "Auto-Merger", Value: "true"},
    {Key: "Source-Commit", Value: source.Hash.String()},
    {Key: "Source-Ref", Value: sourceRef}, // "jam/<session>/<user>/<branch>"
}
if heuristic != "" {
    auto = append(auto, trailer.Trailer{Key: "Auto-Resolved", Value: heuristic})
}
msg := trailer.Compose(
    fmt.Sprintf("Merge %s into draft", source.Hash.String()[:7]),
    auto,
)
```

### Pre-receive validation pattern

`epic-portal-git-pre-receive` runs validation in the Go handler BEFORE
invoking `git-receive-pack` (locked design decision). The handler must
walk the pack being received without first accepting it. go-git supports
this via `plumbing/format/packfile.NewParser` with an in-memory storage.

**Gotcha:** the system `git-receive-pack` uses a quarantine directory for
incoming objects (see
[722ff7f876](https://github.com/git/git/commit/722ff7f876c8a2ad99c42434f58af098e61b96e8)).
go-git's `dotgit` storage does NOT look inside the quarantine path — see
[src-d/go-git#886](https://github.com/src-d/go-git/issues/886). Since
jamsesh's pre-receive runs in the Go handler BEFORE spawning the
subprocess, this is not directly a blocker — we parse the pushed packfile
ourselves from the request body. But it does mean any post-receive
validation that opens the bare repo through `git.PlainOpen` will not see
quarantined objects; we must validate before the subprocess accepts them.

**The flow:**

1. Read the pkt-line `command-list` from the request body — this is the
   `<old-sha> <new-sha> <ref>` advertisement before the packfile bytes.
   Use `plumbing/protocol/packp.UpdateRequest` to decode.
2. Validate ref namespace: every ref must match
   `refs/heads/jam/<session>/<user>/*` for the authenticated user.
3. Validate non-force-push on shared refs: for `base` and `draft`,
   require old-sha to be ancestor of new-sha (use
   `Commit.IsAncestor(other)`).
4. Parse the packfile body via `packfile.NewParser` into a scratch
   in-memory storer (`storage/memory.NewStorage()`).
5. Walk every commit in the pack via `object.NewCommitWalker` from the
   new-sha down to the old-sha. For each commit:
   - `trailer.ValidateRequired` — missing trailer = reject.
   - Walk the commit's tree-diff vs its parent; every changed path must
     match the session's writable-scope globs.
6. Enforce `git.max_push_bytes` (default 50 MB) by counting bytes
   streamed.
7. On any failure, respond with a `report-status` packet listing the
   offending commits/paths — see `plumbing/protocol/packp.ReportStatus`.

After validation passes, the handler spawns `git-receive-pack` and
forwards the same bytes (need to `io.TeeReader` the body so we both
validate and forward, OR buffer up to `max_push_bytes`).

---

## 2. Git smart-HTTP — subprocess invocation pattern

**Locked.** Per `epic-portal-git.md`: invoke `git-receive-pack` and
`git-upload-pack` as subprocesses directly (the Gitea/Forgejo pattern).
NOT via `git http-backend` CGI.

**Canonical reference:** Gitea's
[`routers/web/repo/githttp.go`](https://github.com/go-gitea/gitea/blob/main/routers/web/repo/githttp.go).
Read `httpBase` (auth + repo lookup) and `serviceRPC` (the actual stream
piping) for the production pattern.

### The three endpoints

| Path | Method | Handler responsibility |
|------|--------|------------------------|
| `/git/<org>/<session>.git/info/refs?service=git-{upload,receive}-pack` | GET | Auth, then run `git <service> --stateless-rpc --advertise-refs <dir>`, wrap with `# service=git-<service>\n` pkt-line + flush, set content-type `application/x-git-<service>-advertisement` |
| `/git/<org>/<session>.git/git-upload-pack` | POST | Auth, validate content-type `application/x-git-upload-pack-request`, pipe body into `git upload-pack --stateless-rpc <dir>`, stream stdout to response with content-type `application/x-git-upload-pack-result` |
| `/git/<org>/<session>.git/git-receive-pack` | POST | Auth, parse + validate pack (pre-receive), if accepted pipe body into `git receive-pack --stateless-rpc <dir>`, stream stdout to response, emit post-receive events on success |

### Content-type headers (canonical)

- Request (push): `application/x-git-receive-pack-request`
- Request (fetch): `application/x-git-upload-pack-request`
- Response (info/refs): `application/x-git-<service>-advertisement`
- Response (push): `application/x-git-receive-pack-result`
- Response (fetch): `application/x-git-upload-pack-result`
- Always `Cache-Control: no-cache` on responses.

### Protocol-v2

Client signals v2 via the `Git-Protocol: version=2` request header.
Handler must propagate this to the subprocess as the `GIT_PROTOCOL` env
var:

```go
if v := r.Header.Get("Git-Protocol"); v != "" {
    cmd.Env = append(cmd.Env, "GIT_PROTOCOL="+v)
}
```

Validate the value before passing — Gitea regex-checks it against
`^[0-9a-zA-Z:=_.-]+$` to prevent env-var injection.

Protocol-v2 is designed for stateless HTTP. With `--stateless-rpc`, the
subprocess handles framing; the portal does not need to parse the v2
capability advertisement itself.

### Streaming discipline

Gigabyte-scale fetches require true streaming. The pattern:

- Use `io.Copy(stdin, r.Body)` in a goroutine that closes stdin when
  done.
- Use `io.Copy(w, stdout)` on the response.
- Wrap the response writer in a check for `http.Flusher` and flush on a
  timer for very slow operations.
- Set NO `Content-Length` on the response — the body is open-ended until
  the subprocess closes its stdout.
- Honor request context cancellation; `cmd.Cancel = func() error { return
  cmd.Process.Kill() }` plus pass `exec.CommandContext(r.Context(), ...)`.

### Handler skeleton

```go
package smarthttp

import (
    "fmt"
    "io"
    "net/http"
    "os/exec"
    "regexp"
)

var gitProtocolRE = regexp.MustCompile(`^[0-9a-zA-Z:=_.-]+$`)

// ServiceReceivePack is the POST /git/.../git-receive-pack handler.
// AuthOK and PreReceiveValidate are jamsesh-specific upstream concerns.
func ServiceReceivePack(repoDir string) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        const service = "receive-pack"

        if r.Header.Get("Content-Type") !=
            fmt.Sprintf("application/x-git-%s-request", service) {
            http.Error(w, "bad content type", http.StatusBadRequest)
            return
        }

        // Pre-receive validation happens upstream: it has tee'd the body
        // and confirmed pack + trailers + scope. The validated body is
        // now passed back as r.Body (buffered or re-streamed).

        cmd := exec.CommandContext(r.Context(),
            "git", service, "--stateless-rpc", repoDir)
        cmd.Env = append(cmd.Environ(),
            "GIT_DIR="+repoDir,
            // GIT_QUARANTINE_PATH is set by git itself; do not set it.
        )
        if p := r.Header.Get("Git-Protocol"); p != "" && gitProtocolRE.MatchString(p) {
            cmd.Env = append(cmd.Env, "GIT_PROTOCOL="+p)
        }

        stdin, err := cmd.StdinPipe()
        if err != nil {
            http.Error(w, "stdin pipe", http.StatusInternalServerError)
            return
        }
        stdout, err := cmd.StdoutPipe()
        if err != nil {
            http.Error(w, "stdout pipe", http.StatusInternalServerError)
            return
        }

        if err := cmd.Start(); err != nil {
            http.Error(w, "spawn", http.StatusInternalServerError)
            return
        }

        // Pipe request body → subprocess stdin in a goroutine.
        go func() {
            defer stdin.Close()
            _, _ = io.Copy(stdin, r.Body)
        }()

        w.Header().Set("Content-Type",
            fmt.Sprintf("application/x-git-%s-result", service))
        w.Header().Set("Cache-Control", "no-cache")
        w.WriteHeader(http.StatusOK)

        // Stream subprocess stdout → response. Flush every chunk so the
        // client sees progress on long-running ops.
        flusher, _ := w.(http.Flusher)
        buf := make([]byte, 32*1024)
        for {
            n, err := stdout.Read(buf)
            if n > 0 {
                _, _ = w.Write(buf[:n])
                if flusher != nil {
                    flusher.Flush()
                }
            }
            if err != nil {
                break
            }
        }

        if err := cmd.Wait(); err != nil {
            // The protocol report-status (already streamed) carries
            // success/failure. A non-zero exit at this point means the
            // pack was rejected post-validation — log and emit a
            // post-receive failure event.
        } else {
            // Success — fire commit.arrived events for each ref update.
        }
    }
}
```

The `info/refs` and `git-upload-pack` handlers follow the same shape.
For `info/refs`, the subprocess invocation is:

```go
cmd := exec.CommandContext(r.Context(),
    "git", service, "--stateless-rpc", "--advertise-refs", repoDir)
```

and the response prepends the pkt-line framing:

```
001e# service=git-upload-pack\n
0000
<subprocess stdout>
```

(The `001e` is the four-hex-digit length of the next 30 bytes including
trailing LF; `0000` is a flush packet.)

### Concurrent push handling

Locked decision: rely on git's native ref locking. No portal-level locks.
Native git uses `<refname>.lock` files in the repo's `refs/` tree; the
last-writer-wins is a hard error from the subprocess that surfaces in the
`report-status` packet as `ng <ref> reference already locked` and the
client retries.

For different refs in the same repo, git locks are per-ref so concurrent
pushes proceed in parallel. Same-ref pushes serialize through the OS
file-locking. No special handling needed in the Go layer.

---

## 3. Red flags & open questions

- **Three-way merge is not native go-git.** The composition pattern above
  works but is jamsesh's most correctness-critical surface. The
  adversarial test corpus called out in `epic-auto-merger.md` is
  non-negotiable.
- **Quarantine directory is invisible to go-git's storage.** Validate
  packfiles via in-memory parsing before invoking the subprocess; do not
  rely on opening the bare repo's storage to inspect "what was just
  pushed."
- **Trailer parsing has no go-git API.** We own the implementation. The
  25%-rule from `git interpret-trailers` is the rigorous definition; a
  simpler "must be all-trailer lines after a blank line" rule is
  sufficient for jamsesh because we emit our own trailers and reject
  commits missing them.
- **go-git v6 is alpha.** Do not adopt. Lock to v5.19.x.

---

## References

- [go-git/go-git](https://github.com/go-git/go-git) — pure-Go git library
- [go-git#942 — Add Merge method](https://github.com/go-git/go-git/issues/942)
- [Gitea `routers/web/repo/githttp.go`](https://github.com/go-gitea/gitea/blob/main/routers/web/repo/githttp.go) — reference smart-HTTP handler
- [Git HTTP protocol docs](https://git-scm.com/docs/http-protocol)
- [Git protocol v2 docs](https://git-scm.com/docs/protocol-v2)
- [`git interpret-trailers`](https://git-scm.com/docs/git-interpret-trailers)
- [`git merge-file`](https://git-scm.com/docs/git-merge-file)
- [Quarantine commit in git](https://github.com/git/git/commit/722ff7f876c8a2ad99c42434f58af098e61b96e8)
