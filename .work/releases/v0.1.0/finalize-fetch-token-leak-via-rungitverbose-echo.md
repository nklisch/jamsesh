---
id: finalize-fetch-token-leak-via-rungitverbose-echo
kind: story
stage: done
tags: [security, plugin]
parent: null
depends_on: []
release_binding: v0.1.0
gate_origin: null
created: 2026-05-18
updated: 2026-05-18
---

# Finalize fetch token echoes to stdout via runGitVerbose command-line print

## Origin

Found during review of `gate-security-finalize-fetch-token-in-git-url`.
That story moved the bearer token out of the git URL and into `git -c
http.extraHeader=Authorization: Bearer <token>`, which keeps it out of
`.git/config`, `ps -ef`, and shell history. However the plugin's
`runGitVerbose` helper (`cmd/jamsesh/finalizecmd/execute.go:115`) prints
the full command line including all `-c` args to the operator's stdout:

```go
fmt.Fprintf(out, "+ git %s\n", strings.Join(args, " "))
```

So during finalize, the operator's terminal (and any CI capture of that
terminal) sees:

```
+ git -c http.extraHeader=Authorization: Bearer eyJhbGc... fetch jamsesh
```

## Severity

Low. The leak target is the operator's own stdout — not a multi-tenant
channel like `ps` or `.git/config`. The token is short-TTL (5 min). But:

- CI logs may capture the line, broadening the audience
- asciinema / screen-recording captures may persist it
- Shoulder-surfing during interactive finalize sessions

These are real but bounded.

## Fix direction

Redact `-c http.extraHeader=Authorization: ...` arg pairs in
`runGitVerbose`'s printed line, while passing the unredacted arg to the
actual git subprocess. Targeted redaction (match `http.extraHeader=` and
the value following `Authorization:` case-insensitive) is preferable to
blanket `-c` redaction — operators want to see what git config the plugin
is setting, just not the secret bits.

Sketch:

```go
func runGitVerbose(out io.Writer, args ...string) error {
    fmt.Fprintf(out, "+ git %s\n", strings.Join(redactGitArgs(args), " "))
    // ...
}

func redactGitArgs(args []string) []string {
    out := make([]string, len(args))
    for i, a := range args {
        lower := strings.ToLower(a)
        if strings.HasPrefix(lower, "http.extraheader=authorization:") {
            // Find the schema portion (Bearer, Basic, ...) to preserve.
            idx := strings.Index(a, ":")
            schemeEnd := idx + 1
            for schemeEnd < len(a) && a[schemeEnd] == ' ' {
                schemeEnd++
            }
            spaceIdx := strings.IndexByte(a[schemeEnd:], ' ')
            if spaceIdx > 0 {
                out[i] = a[:schemeEnd+spaceIdx+1] + "<redacted>"
            } else {
                out[i] = a[:schemeEnd] + "<redacted>"
            }
            continue
        }
        out[i] = a
    }
    return out
}
```

Add a unit test confirming the printed line contains `<redacted>` and
does not contain the raw token.

## Acceptance

- `runGitVerbose` prints `http.extraHeader=Authorization: Bearer <redacted>`
  (or equivalent) when the `-c` arg sets an Authorization header.
- Non-Authorization `-c` args are unchanged in the printed line.
- The actual git subprocess receives the unredacted args.
- A test asserts both behaviors.

## Implementation notes

**Detection rule (case sensitivity):** `redactGitArgs` matches the prefix
`"http.extraHeader=Authorization:"` case-sensitively. `http.extraHeader`
is a literal git config key that git itself treats case-sensitively; folding
to lowercase would add false-match surface for no real benefit. The call site
in `fetchsource.go`'s `performFetch` spells it exactly as
`http.extraHeader=Authorization: Bearer <token>`, which matches the prefix.

**Redaction logic:** The scheme token (Bearer, Basic, etc.) is preserved in
the printed line so operators can see the auth type. Only the credential
value that follows the scheme is replaced with `<redacted>`. If no space
follows the scheme (malformed header), the entire remainder after `Authorization:`
is replaced.

**Test coverage:**
- `TestRedactGitArgs` — 5 sub-cases: Bearer redacted, Basic redacted,
  non-Authorization `-c` unchanged, mixed args (only Authorization redacted),
  no extraHeader args pass through.
- `TestRunGitVerbose_redactsAuthorizationInPrint` — integration test: runs
  `runGitVerbose` against a real temp repo with a token arg, asserts the
  printed line contains `<redacted>` and does not contain the raw token,
  and that the auth scheme (`Authorization: Bearer`) is preserved.

## Review (2026-05-18)

**Verdict**: Approve

**Blockers**: none
**Important**: none
**Nits**: none.

**Notes**: Targeted fix that closes the secondary leak surface identified in
the parent story's review. Case-sensitive matching is the right call (git
config keys are literal). Scheme preservation in the printed line gives
operators the auth-type signal they need without exposing the credential.
Unredacted args still flow to the subprocess unchanged, so the production
behavior is intact.
