# Smart-HTTP protocol reference

## pkt-line framing

Every line in the smart-HTTP wire format is a "pkt-line":

```
<4-hex-len><payload>
```

- `<4-hex-len>` is the four-lowercase-hex-digit length of the line
  INCLUDING the four-digit prefix itself (so a single-byte payload =
  `0005<byte>`).
- Special: `0000` is the "flush packet" — end-of-section marker.
- Special: `0001` is the "delimiter packet" (protocol v2).
- Payloads typically end with `\n` for human readability but it's not
  required.

In Go:

```go
func pktLine(payload string) string {
    return fmt.Sprintf("%04x%s", len(payload)+4, payload)
}
const flushPkt = "0000"
```

## Content-type table (canonical, do not deviate)

| Direction | upload-pack | receive-pack |
|-----------|-------------|--------------|
| Request body | `application/x-git-upload-pack-request` | `application/x-git-receive-pack-request` |
| Response body | `application/x-git-upload-pack-result` | `application/x-git-receive-pack-result` |
| info/refs response | `application/x-git-upload-pack-advertisement` | `application/x-git-receive-pack-advertisement` |

Always: `Cache-Control: no-cache`. Never set `Content-Length` on a
streamed response.

## info/refs advertisement (smart-HTTP v0/v1)

```
S: 200 OK
S: Content-Type: application/x-git-upload-pack-advertisement
S: Cache-Control: no-cache
S:
S: 001e# service=git-upload-pack\n
S: 0000
S: <subprocess --advertise-refs output>
```

The first pkt-line is the service announcement. The `0000` is its
terminating flush. The subprocess output already contains its own
ref advertisement and final flush.

### info/refs handler skeleton

```go
func InfoRefs(repoDir string) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        service := r.URL.Query().Get("service")
        if service != "git-upload-pack" && service != "git-receive-pack" {
            http.Error(w, "bad service", http.StatusBadRequest)
            return
        }
        svc := strings.TrimPrefix(service, "git-")

        cmd := exec.CommandContext(r.Context(),
            "git", svc, "--stateless-rpc", "--advertise-refs", repoDir)
        cmd.Env = append(cmd.Environ(), "GIT_DIR="+repoDir)
        if v := r.Header.Get("Git-Protocol"); v != "" && gitProtocolRE.MatchString(v) {
            cmd.Env = append(cmd.Env, "GIT_PROTOCOL="+v)
        }

        out, err := cmd.Output()
        if err != nil {
            http.Error(w, "git", http.StatusInternalServerError)
            return
        }

        w.Header().Set("Content-Type",
            "application/x-"+service+"-advertisement")
        w.Header().Set("Cache-Control", "no-cache")
        w.WriteHeader(http.StatusOK)

        // Prepend the pkt-line service header.
        prefix := fmt.Sprintf("# service=%s\n", service)
        fmt.Fprintf(w, "%04x%s0000", len(prefix)+4, prefix)
        _, _ = w.Write(out)
    }
}
```

### upload-pack handler

Same shape as receive-pack (see the SKILL) but with NO pre-receive
validation (fetches don't mutate state) and NO post-receive emission.
Just auth → spawn → pipe stdin → stream stdout. For gigabyte-scale
fetches (cloning a large session), the streaming discipline keeps
memory bounded.

### streamWithFlush helper

```go
func streamWithFlush(w http.ResponseWriter, r io.Reader) {
    flusher, _ := w.(http.Flusher)
    buf := make([]byte, 32*1024)
    for {
        n, err := r.Read(buf)
        if n > 0 {
            _, _ = w.Write(buf[:n])
            if flusher != nil {
                flusher.Flush()
            }
        }
        if err != nil {
            return
        }
    }
}
```

## Protocol v2

Client signals: `Git-Protocol: version=2` request header.

Server must:
1. Validate the header value matches `^[0-9a-zA-Z:=_.-]+$`.
2. Propagate as `GIT_PROTOCOL=<value>` env var to the subprocess.

With `--stateless-rpc`, the subprocess handles everything from there:
capability negotiation, command framing (`command=fetch\n` vs
`command=ls-refs\n`), and the new delimiter-packet (`0001`) framing.

The portal does NOT need to understand the v2 protocol body.

## Request flow — push (receive-pack)

```
C: GET /info/refs?service=git-receive-pack
C: Git-Protocol: version=2
S: 200 OK
S: Content-Type: application/x-git-receive-pack-advertisement
S: <advertisement>

C: POST /git-receive-pack
C: Content-Type: application/x-git-receive-pack-request
C: <command-list><packfile>
S: 200 OK
S: Content-Type: application/x-git-receive-pack-result
S: <report-status>
```

The `<command-list>` is one pkt-line per ref update:

```
<old-sha> SP <new-sha> SP <ref-name> NUL <capabilities>
<old-sha> SP <new-sha> SP <ref-name>
...
0000
```

Followed immediately by the packfile bytes (PACK magic + version +
object count + objects + SHA-1 checksum).

## Request flow — fetch (upload-pack)

```
C: GET /info/refs?service=git-upload-pack
S: 200 OK
S: <advertisement>

C: POST /git-upload-pack
C: <want lines + haves + done>
S: 200 OK
S: <ack/nak negotiation + packfile>
```

The negotiation may take multiple rounds of POST in v0/v1; v2 collapses
much of this into a single request/response.

## report-status (push response)

After processing a push, receive-pack writes:

```
000eunpack ok\n          (or "unpack <error>\n")
0019ok refs/heads/foo\n  (per accepted ref)
0019ng refs/heads/bar reason\n  (per rejected ref)
0000
```

When the Go handler rejects the push BEFORE invoking receive-pack
(pre-receive failure), it must synthesize an equivalent error response
itself — typically:

```
0026error: <human-readable reason>\n
0000
```

This is what `git push` displays as `remote: error: ...`.

## Sideband progress (sideband-64k)

When the client advertises `side-band-64k` capability, receive-pack and
upload-pack interleave three streams in the response body:

- band 1 (`\x01`): packfile data / report-status
- band 2 (`\x02`): progress messages (shown as `remote: ...`)
- band 3 (`\x03`): fatal error messages

Each pkt-line in the response is prefixed by the band byte. The
subprocess handles this entirely; the portal pipes bytes.

## References

- [Git HTTP protocol docs](https://git-scm.com/docs/http-protocol)
- [Git protocol-v2 docs](https://git-scm.com/docs/protocol-v2)
- [Git protocol-capabilities docs](https://git-scm.com/docs/protocol-capabilities)
- [`git-receive-pack(1)`](https://git-scm.com/docs/git-receive-pack)
- [`git-upload-pack(1)`](https://git-scm.com/docs/git-upload-pack)
- [Gitea reference impl](https://github.com/go-gitea/gitea/blob/main/routers/web/repo/githttp.go)
