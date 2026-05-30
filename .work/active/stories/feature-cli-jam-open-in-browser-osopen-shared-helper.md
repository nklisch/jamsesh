---
id: feature-cli-jam-open-in-browser-osopen-shared-helper
kind: story
stage: implementing
tags: [plugin]
parent: feature-cli-jam-open-in-browser
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-30
updated: 2026-05-30
---

# Promote the browser-opener into `cmd/jamsesh/internal/osopen`

Implements **Unit 1** of `feature-cli-jam-open-in-browser`. See the feature body
for the full design and the exact `osopen.Open` source.

## Scope

- Create `cmd/jamsesh/internal/osopen/osopen.go` exporting
  `Open(rawURL string, errOut io.Writer) error` (the parameterized,
  graceful-degradation opener) plus a package-private `var execCommand =
  exec.Command` test seam and `platformArgv`.
- Migrate `cmd/jamsesh/auth/auth.go` and `cmd/jamsesh/finalizecmd/browseropen.go`
  to delegate to `osopen.Open(url, os.Stderr)`. Preserve each package's existing
  injection seam (`auth`'s `cfg.openURL` config field; `finalizecmd`'s
  `var openURL`). Delete the two inlined platform switches.
- Add `cmd/jamsesh/internal/osopen/osopen_test.go`.

This is a no-behavior-change extraction for `auth`/`finalize` — it exists to give
`new`/`join` (Unit 2) a shared, tested opener. It must land first: the consumers
import it.

## Acceptance criteria

- [ ] `osopen.Open` returns nil and writes `Please visit: <url>` to `errOut`
      when the launcher fails to start (override `execCommand` with a nonexistent
      binary).
- [ ] `platformArgv` returns the correct argv for the host GOOS.
- [ ] Neither `auth` nor `finalizecmd` still contains an
      `xdg-open`/`open`/`rundll32` switch; both call `osopen.Open`.
- [ ] `go build ./...` and `go vet ./cmd/jamsesh/...` clean; existing
      `auth/browser_test.go` and `finalizecmd` tests pass unchanged.
- [ ] No leftover unused imports (`goimports` clean).
