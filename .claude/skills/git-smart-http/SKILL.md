---
name: git-smart-http
description: Smart-HTTP git serving via subprocess invocation of git-upload-pack and git-receive-pack (the Gitea/Forgejo pattern). Handler skeleton, content-type negotiation, protocol-v2 propagation, streaming with io.Pipe + http.Flusher, pre-receive hook in-process, post-receive event emission. Auto-loads when working with the portal's git HTTP routes or any handler under /git/*. Trigger keywords - git-receive-pack, git-upload-pack, git-http-backend, smart-http, info/refs, pre-receive, post-receive, protocol-v2, GIT_PROTOCOL, stateless-rpc, smart-http subprocess, packfile streaming.
user-invocable: false
---

# Smart-HTTP subprocess pattern for jamsesh

**Locked design** (`.work/active/epics/epic-portal-git.md`): the portal
spawns `git-upload-pack` and `git-receive-pack` as subprocesses directly.
NOT via `git http-backend` CGI. Pre-receive validation happens in the Go
handler BEFORE invoking the subprocess.

**Reference implementation** to mirror:
[Gitea `routers/web/repo/githttp.go`](https://github.com/go-gitea/gitea/blob/main/routers/web/repo/githttp.go).
Read `httpBase` (auth + repo lookup) and `serviceRPC` (the stream piping).

## The three endpoints

```
GET  /git/<org>/<session>.git/info/refs?service=git-{upload,receive}-pack
POST /git/<org>/<session>.git/git-upload-pack
POST /git/<org>/<session>.git/git-receive-pack
```

Auth: HTTP Basic, password = user OAuth token (per `docs/SPEC.md`).

## Content-type headers (canonical, do not deviate)

| Direction | upload-pack | receive-pack |
|-----------|-------------|--------------|
| info/refs response | `application/x-git-upload-pack-advertisement` | `application/x-git-receive-pack-advertisement` |
| Request | `application/x-git-upload-pack-request` | `application/x-git-receive-pack-request` |
| Response | `application/x-git-upload-pack-result` | `application/x-git-receive-pack-result` |

Always set `Cache-Control: no-cache` on responses. Never set
`Content-Length` (the body is open-ended).

## Subprocess invocation

All three endpoints use `--stateless-rpc` so the subprocess handles
framing and the portal can act as a pure pipe:

```go
// info/refs
exec.CommandContext(ctx, "git", svc, "--stateless-rpc", "--advertise-refs", repoDir)
// upload-pack / receive-pack
exec.CommandContext(ctx, "git", svc, "--stateless-rpc", repoDir)
```

`repoDir` is the absolute path to the bare repo
(`<storage>/orgs/<org>/sessions/<session>.git`). Pass via positional arg
AND `GIT_DIR=<repoDir>` env for belt-and-suspenders.

## Protocol v2

Client signals via `Git-Protocol: version=2`. Propagate to subprocess:

```go
var gitProtocolRE = regexp.MustCompile(`^[0-9a-zA-Z:=_.-]+$`)

if v := r.Header.Get("Git-Protocol"); v != "" && gitProtocolRE.MatchString(v) {
    cmd.Env = append(cmd.Env, "GIT_PROTOCOL="+v)
}
```

VALIDATE the value before propagating — Gitea regex-checks it to prevent
env-var injection. With `--stateless-rpc`, the subprocess handles all v2
capability negotiation and framing; portal does not need to parse v2.

## Receive-pack handler skeleton

```go
func ServiceReceivePack(repoDir string, validator PreReceiveValidator,
                        emitter PostReceiveEmitter) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        if r.Header.Get("Content-Type") != "application/x-git-receive-pack-request" {
            http.Error(w, "bad content type", http.StatusBadRequest)
            return
        }
        body := http.MaxBytesReader(w, r.Body, maxPushBytes)

        // Tee for validation; keep bytes for the subprocess.
        var buf bytes.Buffer
        if err := validator.Validate(r.Context(),
            io.TeeReader(body, &buf), authedUser); err != nil {
            writeProtocolError(w, err) // synthesized report-status `ng`
            return
        }

        cmd := exec.CommandContext(r.Context(),
            "git", "receive-pack", "--stateless-rpc", repoDir)
        cmd.Env = append(cmd.Environ(), "GIT_DIR="+repoDir)
        if v := r.Header.Get("Git-Protocol"); v != "" && gitProtocolRE.MatchString(v) {
            cmd.Env = append(cmd.Env, "GIT_PROTOCOL="+v)
        }

        stdin, _ := cmd.StdinPipe()
        stdout, _ := cmd.StdoutPipe()
        if err := cmd.Start(); err != nil {
            http.Error(w, "spawn", http.StatusInternalServerError)
            return
        }
        go func() { defer stdin.Close(); _, _ = io.Copy(stdin, &buf) }()

        w.Header().Set("Content-Type", "application/x-git-receive-pack-result")
        w.Header().Set("Cache-Control", "no-cache")
        w.WriteHeader(http.StatusOK)
        streamWithFlush(w, stdout) // helper below

        if err := cmd.Wait(); err == nil {
            emitter.OnSuccess(r.Context(), pushedRefs)
        }
    }
}
```

`streamWithFlush` reads from `stdout` in 32k chunks, writes to `w`, and
calls `Flush()` after each chunk if the response writer implements
`http.Flusher`. See `references/protocol.md` for the full helper, the
`info/refs` skeleton, the `git-upload-pack` skeleton, and the
pkt-line/report-status format.

## Pre-receive validation

Runs in the Go handler before spawning `git-receive-pack`. The validator
parses the pushed pack directly from the request body — see the `go-git`
skill for the pack-parsing pattern.

Responsibilities (per `epic-portal-git.md`):

1. Required commit trailers (`Jam-Session`, `Jam-Turn`, `Jam-Author`).
2. Writable scope — every changed path matches a session-scope glob.
3. Ref namespace — pushed ref is in `refs/heads/jam/<session>/<user>/*`
   matching the authenticated user.
4. No force-pushes on shared refs (`base`, `draft`) — old-sha must be
   ancestor of new-sha.
5. Pack size ≤ `git.max_push_bytes`.
6. Special-case the base-push during session creation:
   `jam/<session>/base` is writable exactly once, when the session row's
   `base_sha` is null.

Rejection format: synthesize a `report-status` `ng` packet — see
`references/protocol.md`.

## Post-receive event emission

After `cmd.Wait()` returns success, walk the accepted ref updates and
emit `commit.arrived` events into the events table. The auto-merger and
WebSocket gateway both subscribe.

**Invariant** (`epic-auto-merger.md`): the event-emit must be in the
SAME database transaction as anything that depends on the post-receive
having fired. Crash between subprocess success and event-emit = lost
commit from replay's view.

## Concurrent push handling

Locked: rely on git's native ref locking (`<refname>.lock` files in the
bare repo's `refs/`). No portal-level lock layer.

- Different refs in the same repo → parallel.
- Same ref → serialized via OS file locking inside the subprocess.
- Loser sees `ng <ref> reference already locked` in their report-status
  and the git client retries.

## Streaming gotchas

- **No `Content-Length`** — subprocess output is open-ended.
- **Flush every chunk** — long fetches stall without flushing.
- **Cancel propagation** — `exec.CommandContext(r.Context(), ...)` plus
  `cmd.Cancel = func() error { return cmd.Process.Kill() }`. When the
  HTTP client disconnects, the subprocess dies.
- **Goroutine leak risk** — always `defer stdin.Close()` in the stdin
  copier. If stdout reading panics, the stdin goroutine wedges.
- **Gzip-encoded requests** — the git client may send
  `Content-Encoding: gzip`. Wrap the body with `gzip.NewReader` before
  teeing.
- **`GIT_QUARANTINE_PATH`** — DO NOT set this in env; `git-receive-pack`
  sets it internally. (Relevant only when reasoning about why go-git's
  storage can't see incoming objects — see the `go-git` skill.)

## Foundation references

- `docs/SPEC.md` — git smart-HTTP in the Stack section
- `docs/PROTOCOL.md` — git smart-HTTP routes
- `docs/SECURITY.md` — git push authorization
- `docs/ARCHITECTURE.md` — Git smart-HTTP component
- `docs/research/git-internals-stack.md` — full research notes
- `.work/active/epics/epic-portal-git.md` — design decisions

## References

- `references/protocol.md` — pkt-line framing, content-type table, v2
  capability advertisement basics, info/refs skeleton, report-status
  format, sideband-64k
