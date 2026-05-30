---
id: feature-cli-jam-open-in-browser
kind: feature
stage: implementing
tags: [plugin]
parent: null
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-30
updated: 2026-05-30
---

# Open a jam session in the browser (`--open` flag + agent offer)

## Brief

After `jamsesh jam new` (durable or `--playground`) and `jamsesh jam join`, the
CLI prints a session URL but provides no way to actually open it. The human at
the terminal must copy-paste the URL into a browser. `finalize` and `auth`
already auto-launch the browser (`xdg-open`/`open`/`rundll32` via
`defaultOpenURL`), so the building block exists — it was simply never wired into
the session-creation/join path.

This feature adds a non-interactive `--open` boolean flag to `jam new` /
`new` and `jam join` / `join`. When set, after the session is created/joined
(push + state-write + summary print all succeed), the CLI launches the browser
at the session's portal URL. If the browser cannot launch (headless, no
`DISPLAY`, exec error), it degrades gracefully — prints the URL for the agent or
human to paste — and the command still succeeds. The `/jamsesh:jam` skill is
updated so the **agent offers** to open the session when `jam` is invoked and
passes `--open` when the user agrees; the CLI itself stays non-interactive (no
`[Y/n]` prompt).

Open targets by session kind:
- Playground (`new --playground`, `join` of a playground session) →
  `{portalURL}/playground/s/{id}/join`
- Durable (`new --org …`, `join` of a durable session) →
  `{portalURL}/orgs/{orgId}/sessions/{sessionId}` (the `session-view` route)

## Background / evidence (investigation, 2026-05-30)

- This affordance was **designed and then dropped**. The v0.4.0 story
  `cli-playground-flag.md:74-78` (now in `.work/releases/v0.4.0/`) specified the
  playground summary should end with an *"Open in browser? [Y/n]"* prompt
  reusing the `auth/` browser-open helper. The shipped
  `printPlaygroundSummary` (`cmd/jamsesh/sessioncmd/new.go:511`) printed only
  "share URL + nickname + ends-in"; the review approved without flagging the
  missing browser-open piece.
- The browser join flow itself **works** — verified live: the deployed SPA
  client-routes `/playground/s/{id}/join` to the JoinerPicker (nickname form),
  and `POST /api/playground/sessions/{id}/join` returns `200` with a bearer.
  The gap is purely the missing terminal-side open.
- Re-framed as new capability rather than restoring the literal prompt: the new
  workflow is **agent-driven / non-TTY**, so an interactive `[Y/n]` would never
  fire. Hence a flag + an agent-layer offer.

## Strategic decisions

(Locked by the user at scope time; `feature-design` should inherit these and not
re-litigate.)

- **Affordance shape**: a non-interactive `--open` flag on `jam new`/`new` and
  `jam join`/`join` — *not* a standalone `jamsesh open` subcommand and *not* a
  CLI `[Y/n]` prompt. Rationale: fits the agent-driven non-TTY flow; the offer
  lives at the agent layer.
- **Agent offers**: `plugins/jamsesh/skills/jam/SKILL.md` instructs the agent to
  offer to open the session when `jam` is invoked and to pass `--open` on the
  user's assent. The offer is conversational (the skill/agent), never a binary
  prompt.
- **Session-type scope**: both playground and durable, covering `new` (primary)
  and `join` (symmetric).
- **Failure behavior**: graceful — on any launch failure print the URL so it can
  be pasted, and return success. Reuse the existing `defaultOpenURL` semantics.

## Design inputs (for feature-design)

- **Helper reuse / promotion**: `--open` becomes the **third** consumer of the
  inlined browser opener (`auth`, `finalize`, now `new`/`join`). The note at
  `cmd/jamsesh/finalizecmd/browseropen.go:14` explicitly says to promote to a
  shared package on a third consumer. Design should extract `defaultOpenURL`
  into `cmd/jamsesh/internal/osopen` (with an injectable `openURL` function var
  for hermetic tests, mirroring finalize's indirection) and migrate `auth` and
  `finalize` to it. Confirm whether the `auth` opener and the `finalize` opener
  are byte-identical before collapsing.
- **Wiring sites**:
  - `cmd/jamsesh/sessioncmd/new.go` — add `--open` to `NewCommand` flags; honor
    it at the end of both `newAction` (durable) and `newPlaygroundAction`
    (playground), after `printSummary` / `printPlaygroundSummary`.
  - `cmd/jamsesh/sessioncmd/join.go` — add `--open` to `JoinCommand`; honor it
    after the join summary; resolve the right URL (playground vs durable
    session-view) from the resolved org/session.
  - `cmd/jamsesh/jamcmd/jam.go` — `jam` builds fresh instances of the new/join
    commands, so the flag is inherited automatically; verify it surfaces under
    `jam new`/`jam join` help.
- **URL construction**: reuse the existing share-URL builders where present
  (`printPlaygroundSummary` already computes `{baseURL}/playground/s/{id}/join`).
  Durable session-view URL shape is `/orgs/{orgId}/sessions/{sessionId}` per
  `frontend/src/lib/router.svelte.ts:21`.
- **Skill copy**: keep the `/jamsesh:jam` instructions terse; add an "offer to
  open" step and document the `--open` flag under both `jam new` and `jam join`.

## Acceptance criteria (sketch — feature-design refines + splits into stories)

1. `jam new --playground --open` opens `{portalURL}/playground/s/{id}/join`
   after the session is created and the base ref pushed.
2. `jam new --org <id> --open` opens `{portalURL}/orgs/{orgId}/sessions/{id}`.
3. `jam join <id-or-url> --open` opens the correct URL for the joined session
   (playground join URL vs durable session-view).
4. With `--open` omitted, behavior is unchanged (no browser launch).
5. Browser-launch failure prints the URL and the command still exits `0`.
6. The opener is hermetically testable via an injectable function var; no real
   browser launches in tests. `auth` and `finalize` use the same shared helper.
7. `/jamsesh:jam` instructs the agent to offer to open the session and pass
   `--open` on assent; no interactive CLI prompt is introduced.
8. `docs/UX.md` reflects the `--open` affordance in the create/join flows.

## Out of scope

- A standalone `jamsesh open [session-id]` reopen-anytime subcommand (considered
  and explicitly set aside in favor of the flag; revisit only if a "reopen a
  session I already created" need surfaces).
- Any change to the web join landing page / JoinerPicker (it already works).
- Durable-session web join flow (invite-only today; unrelated to this gap).

## Architectural choice

**Chosen: shared `osopen` package + per-package injectable `openURL` seam +
`--open` flag wiring.** Promote the browser-opener into
`cmd/jamsesh/internal/osopen` (Go's `internal/` rule lets every
`cmd/jamsesh/...` package import it), migrate the two existing inlined copies
(`auth`, `finalizecmd`) to delegate to it, and add `--open` to `new`/`join` with
a thin `openURL` package var in `sessioncmd` mirroring finalize's test seam.

Rejected alternatives:
- **Inline a third copy in `sessioncmd`.** Violates DRY and the explicit
  promote-on-third-consumer note at `browseropen.go:14`; three drifting copies.
- **Standalone `jamsesh open` subcommand.** User-locked against (flag shape) and
  out of scope.

## Design decisions

- **`join --open` open target**: durable session-view URL
  (`/orgs/{orgId}/sessions/{id}`) only — CLI `join` is durable-only (requires
  OAuth; `parseSessionArg` cannot extract an id from a playground share URL), so
  the playground-join-via-CLI branch in the AC sketch is unreachable. A one-line
  comment documents this rather than adding a dead branch. Refines AC #3.
- **Promoted opener signature**: `osopen.Open(rawURL string, errOut io.Writer)
  error` (finalize's parameterized shape, not auth's `os.Stderr`-hardcoded one)
  — keeps the graceful-degradation output testable.
- **Migration preserves seams**: `auth` keeps its `cfg.openURL` config-field
  injection and `finalizecmd` keeps its `var openURL`; only their bodies change
  to call `osopen.Open(url, os.Stderr)`. No existing test breaks.
- **`--open` UX**: print `Opening in browser: <url>` then launch (matches
  `finalizeBrowser`). The summary already prints the URL, and `osopen.Open`
  reprints it on failure, so a headless user always has a copyable string.

## Implementation Units

### Unit 1: `osopen` shared package + migrate auth/finalize
**Files**: `cmd/jamsesh/internal/osopen/osopen.go` (new),
`cmd/jamsesh/internal/osopen/osopen_test.go` (new),
`cmd/jamsesh/auth/auth.go` (modify), `cmd/jamsesh/finalizecmd/browseropen.go` (modify)
**Story**: `feature-cli-jam-open-in-browser-osopen-shared-helper`

```go
// Package osopen launches URLs in the user's default browser with graceful
// degradation. Single home for the platform xdg-open/open/rundll32 logic
// previously inlined in cmd/jamsesh/auth and cmd/jamsesh/finalizecmd.
package osopen

import (
	"fmt"
	"io"
	"os/exec"
	"runtime"
)

// execCommand is overridable in tests to avoid launching a real browser.
var execCommand = exec.Command

// Open launches rawURL in the user's default browser. Any failure —
// unsupported platform, missing launcher, exec error — degrades gracefully:
// rawURL is written to errOut so the caller can copy it, and nil is returned.
// The launched process is detached.
func Open(rawURL string, errOut io.Writer) error {
	argv := platformArgv(rawURL)
	if argv == nil {
		fmt.Fprintf(errOut, "Cannot open browser automatically on %s.\nPlease visit: %s\n", runtime.GOOS, rawURL)
		return nil
	}
	cmd := execCommand(argv[0], argv[1:]...)
	cmd.Stdout, cmd.Stderr = nil, nil
	if err := cmd.Start(); err != nil {
		fmt.Fprintf(errOut, "Could not launch browser: %v\nPlease visit: %s\n", err, rawURL)
		return nil
	}
	go func() { _ = cmd.Wait() }()
	return nil
}

// platformArgv returns the launcher argv for the current OS, or nil if
// unsupported.
func platformArgv(rawURL string) []string {
	switch runtime.GOOS {
	case "linux":
		return []string{"xdg-open", rawURL}
	case "darwin":
		return []string{"open", rawURL}
	case "windows":
		return []string{"rundll32", "url.dll,FileProtocolHandler", rawURL}
	default:
		return nil
	}
}
```

Migration (no behavior change for auth/finalize):
- `auth/auth.go`: replace the inlined `defaultOpenURL` body with
  `return osopen.Open(rawURL, os.Stderr)` (keep the function so `openURL:
  defaultOpenURL` default still wires), or set the config default directly to
  `func(u string) error { return osopen.Open(u, os.Stderr) }` and delete
  `defaultOpenURL`. Either keeps the `cfg.openURL` seam.
- `finalizecmd/browseropen.go`: collapse the file to
  `var openURL = func(rawURL string) error { return osopen.Open(rawURL, os.Stderr) }`;
  delete the local `defaultOpenURL`. `finalize.go`'s call site and its
  `openURL`-override tests are unchanged.

**Implementation Notes**:
- `internal/osopen` is importable by `auth`, `finalizecmd`, `sessioncmd` (all
  under `cmd/jamsesh/`). This is the first `cmd/jamsesh/internal/` package.
- Drop the now-unused `os/exec`, `runtime` imports from auth/finalize if no
  other use remains; run `goimports`.

**Acceptance Criteria**:
- [ ] `osopen.Open` returns nil and writes `Please visit: <url>` to `errOut`
      when the launcher fails to start (tested via `execCommand` override).
- [ ] `platformArgv` returns the correct argv for the host GOOS.
- [ ] `auth` and `finalizecmd` no longer contain a platform `xdg-open`/`open`/
      `rundll32` switch; both delegate to `osopen.Open`.
- [ ] `go build ./...`, `go vet ./cmd/jamsesh/...` clean; existing auth +
      finalizecmd tests still pass unchanged.

---

### Unit 2: `--open` flag on `new` and `join`
**Files**: `cmd/jamsesh/sessioncmd/new.go`, `cmd/jamsesh/sessioncmd/join.go`,
`cmd/jamsesh/sessioncmd/new_test.go`, `cmd/jamsesh/sessioncmd/join_test.go`
**Story**: `feature-cli-jam-open-in-browser-cli-open-flag`

Shared sessioncmd additions:
```go
import "jamsesh/cmd/jamsesh/internal/osopen"

// openURL is the browser-open seam; tests override it to avoid a real launch.
var openURL = func(rawURL string) error { return osopen.Open(rawURL, os.Stderr) }

// openInBrowser prints the URL then launches it; launch failures degrade
// inside osopen.Open (URL reprinted), so --open never fails the command.
func openInBrowser(rawURL string) {
	fmt.Printf("Opening in browser: %s\n", rawURL)
	_ = openURL(rawURL)
}

// URL builders (extracted from printSuccessSummary / printPlaygroundSummary so
// the opened URL is provably the printed one).
func sessionViewURL(baseURL, orgID, sessionID string) string {
	return strings.TrimRight(baseURL, "/") + "/orgs/" + orgID + "/sessions/" + sessionID
}
func playgroundJoinURL(baseURL, sessionID string) string {
	return strings.TrimRight(baseURL, "/") + "/playground/s/" + sessionID + "/join"
}
```

Wiring:
- `NewCommand` + `JoinCommand` flags gain
  `&cli.BoolFlag{Name: "open", Usage: "Open the session in your browser after creating/joining"}`.
- `newAction` (durable): after `printSuccessSummary(...)`, if `cmd.Bool("open")`
  → `openInBrowser(sessionViewURL(pc.BaseURL, session.OrgId, session.Id))`.
- `newPlaygroundAction` (playground): after `printPlaygroundSummary(...)`, if
  `cmd.Bool("open")` → `openInBrowser(playgroundJoinURL(baseURL, resp.Session.Id))`.
- `joinAction`: after the summary print, if `cmd.Bool("open")` →
  `openInBrowser(sessionViewURL(portalURL, orgID, sessionID))`. One-line comment:
  CLI join is durable-only, so the session-view URL is always correct.
- Refactor `printSuccessSummary` / `printPlaygroundSummary` to call the new
  builders (no output change).

**Implementation Notes**:
- `jamcmd/jam.go` builds fresh `NewCommand()`/`JoinCommand()` instances, so
  `--open` surfaces under `jam new`/`jam join` automatically — verify in help,
  no code change.

**Acceptance Criteria**:
- [ ] `new --playground --open` calls `openURL` with `{base}/playground/s/{id}/join`.
- [ ] `new --org X --open` calls `openURL` with `{base}/orgs/{org}/sessions/{id}`.
- [ ] `join <id> --open` calls `openURL` with `{base}/orgs/{org}/sessions/{id}`.
- [ ] Omitting `--open` never calls `openURL` (asserted in each path).
- [ ] `--open` with a failing opener still returns nil / exit 0.
- [ ] Summary output for both paths is byte-identical to before the builder
      extraction (regression guard).

---

### Unit 3: skill offer + doc roll-forward
**Files**: `plugins/jamsesh/skills/jam/SKILL.md`, `docs/UX.md`
**Story**: `feature-cli-jam-open-in-browser-skill-and-docs`

- `SKILL.md`: add `--open` under "Optional flags for `jam new`" and under "For
  `jam join`"; add an "Opening in the browser" note instructing the agent to
  **offer** to open the session when `jam` is invoked (fold the offer into the
  questions the agent already asks for org/goal) and pass `--open` on assent.
  Keep it terse; the CLI never prompts.
- `docs/UX.md`: reflect the `--open` affordance in the create flow and the
  playground create flow (rolling-foundation: describe the present, no
  "previously" prose).

**Acceptance Criteria**:
- [ ] `SKILL.md` documents `--open` for both `jam new` and `jam join` and the
      agent-offer behavior; no interactive CLI prompt described.
- [ ] `docs/UX.md` mentions `--open` in the relevant create/join flow(s).

## Implementation Order

1. Unit 1 — `osopen` shared package + auth/finalize migration
   (`...-osopen-shared-helper`). Prerequisite: the consumers import it.
2. Unit 2 — `--open` on `new` + `join` (`...-cli-open-flag`). Depends on Unit 1.
3. Unit 3 — skill + docs (`...-skill-and-docs`). Depends on Unit 2 (docs
   describe shipped flags).

Mostly sequential (Units 2/3 share package `sessioncmd` and depend on Unit 1);
no parallel fan-out. Stories chosen for dependency visibility, heterogeneous
acceptance (refactor vs new-behavior vs docs), and resumability.

## Testing

- **Unit 1 — `osopen_test.go`**: override `execCommand` to return a command
  that fails to start (nonexistent binary) → assert `Open` returns nil and the
  `bytes.Buffer` errOut contains `Please visit: <url>`. Assert `platformArgv`
  argv for the host GOOS. Existing `auth/browser_test.go` and finalize tests
  re-run unchanged to prove the migration kept behavior.
- **Unit 2 — `new_test.go` / `join_test.go`**: override the `sessioncmd.openURL`
  var with a capture func; drive each action with `--open` set and assert the
  captured URL; drive without `--open` and assert the capture func was not
  called. Reuse the existing `testEnv`/httptest harness used by the playground
  tests. Add a golden-string check that summaries are unchanged after the
  builder extraction.
- **Unit 3**: no automated test; reviewer confirms skill/doc copy.

## Risks

- **`internal/osopen` import boundary** — mitigated: all consumers live under
  `cmd/jamsesh/`, so Go's `internal/` visibility permits the import. Verified by
  `go build ./...` in Unit 1's AC.
- **Migration regressing auth/finalize** — mitigated by keeping each package's
  existing injection seam and re-running their unchanged tests; the only change
  is the delegated body.
- **Low**: `--open` launching a browser in CI/headless — never happens in tests
  (the `openURL`/`execCommand` seams are overridden); in real headless use the
  graceful path prints the URL.
