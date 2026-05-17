---
id: epic-portal-git-pre-receive
kind: feature
stage: done
tags: [portal, security]
parent: epic-portal-git
depends_on: [epic-portal-git-storage]
release_binding: null
gate_origin: null
created: 2026-05-16
updated: 2026-05-16
---

# Portal Git — Pre-Receive Policy

## Brief

The policy-enforcement library invoked by the smart-HTTP receive-pack handler
BEFORE the pushed pack is accepted into the bare repo. Validates every aspect
of a push and either accepts (handler proceeds with `git-receive-pack`) or
rejects with a structured git-protocol error message listing offending
commits, paths, or refs.

**Validations** (per `docs/SECURITY.md > Git push authorization` and
`docs/SPEC.md > Hard constraints`):

- **Ref namespace**: the ref being updated must be in the authenticated
  user's namespace (`jam/<session>/<user>/*`). Sole exception: the
  session-creation base-push (first push to `jam/<session>/base` by the
  session creator, when the bare repo has no refs yet).
- **No force-push on shared refs**: pushes to `base` (after creation) and
  `draft` are rejected. Force-pushes are detected by checking that the
  new sha is a descendant of the old sha for the ref.
- **Commit trailer presence**: every commit in the pack must carry
  `Jam-Session: <session-id>`, `Jam-Turn: <turn-number>`, and
  `Jam-Author: <user-handle-or-id>`. Optional trailers (`Resolves-Conflict`,
  `Auto-Merger`, `Source-Commit`) are recognized but not required.
- **Writable scope**: for every commit, every changed path must match
  at least one glob from the session's declared writable scope. Path
  walking uses `go-git` to enumerate the commit tree diff.
- **Pack size limit**: 50 MB per push by default (configurable). Rejected
  with `push.size_limit` if exceeded.

**Execution model** (locked at epic-design): validation runs in-process in
Go in the receive-pack HTTP handler, NOT via a shell `hooks/pre-receive`
script that calls back into the portal. The handler reads the pushed pack
into a temp area (or streams it into a temporary objects directory),
walks the proposed updates with `go-git`, runs validations, then either
hands off to `git-receive-pack` to apply OR returns the git-protocol error.

**Rejection message format**: git-protocol-compatible (the receive-pack
report-status format) so `git push` displays them inline. Structured
content for the portal API: an error envelope listing offending
commits/paths/refs per the `docs/PROTOCOL.md > HTTP error contract`
codes (`push.scope_violation`, `push.ref_namespace_violation`,
`push.missing_trailer`, `push.size_limit`, `push.force_push_rejected`).

Does NOT cover the HTTP handler itself (`smart-http` feature). Does NOT
cover event emission after acceptance (`post-receive`).

## Epic context

- Parent epic: `epic-portal-git`
- Position in epic: meatiest feature in the epic; consumed by smart-http
  as a library. Storage feature is its only intra-epic dep (for bare
  repo opening and session lookups).

## Foundation references

- `docs/SECURITY.md` — Git push authorization (the canonical validation
  list), Trust model for participants (Mistaken or buggy participants)
- `docs/SPEC.md` — Ref structure, Hard constraints (multi-tenant, writable
  scope), Session shape
- `docs/PROTOCOL.md` — Commit trailer conventions (required vs optional
  trailers), HTTP error contract (`push.*` codes)
- `docs/ARCHITECTURE.md` — Data flow: a turn > Pre-receive validates

## Inherited epic design decisions

- **Execution model**: in-process Go validation in the HTTP handler,
  using `go-git` for object walking. No shell-hook callback pattern.
- **Pack size limit**: 50 MB default, configurable, `push.size_limit`
  error code.

## Decomposition risks

- Pre-receive is the highest-risk feature in this epic. Wire-protocol
  validation has been a long tail of edge cases historically. Mitigation:
  use `go-git` rather than rolling our own pack parser; lean on its
  object-walk APIs; design pass produces a thorough test plan covering
  the trailer / scope / namespace / force-push / size matrix.

## Design decisions

Resolved at feature-design time (autopilot, judgment branch):

- **Public API surface**: a `Validator` type at
  `internal/portal/prereceive/`. Single entry point:
  `Validate(ctx, in ValidateInput) (ValidateResult, error)`
  where `ValidateInput` carries `{repo *git.Repository, session
  *store.Session, account *store.Account, updates []RefUpdate,
  packBytes int64}` and `ValidateResult` is either ok or carries
  the structured `[]Rejection`.
- **Rejection shape**:
  ```go
  type Rejection struct {
      Code    string         // "push.scope_violation" | ...
      Message string         // human-readable
      Details map[string]any // {paths, missing, ref, etc.}
  }
  type ValidateResult struct {
      OK         bool
      Rejections []Rejection
  }
  ```
- **Trailer parsing**: use `go-git`'s commit.Message access +
  manual trailer parsing (last paragraph, lines matching
  `Key: value`). Required: `Jam-Session`, `Jam-Turn`,
  `Jam-Author`. Each must be present + non-empty.
- **Scope glob matching**: use `path/filepath.Match` for each
  declared glob; check each changed path against the union.
  Globs support `*`, `?`, `[a-z]`, and `**` for recursive
  matching. `**` isn't in stdlib's `filepath.Match`, so we add a
  tiny helper or use `github.com/gobwas/glob`. Picking
  `gobwas/glob` for `**` support — used widely, small.
- **Ref namespace check**: parse ref name against the pattern
  `refs/heads/jam/<session-id>/<owner>/<branch>` where
  `<session-id>` matches the session being pushed to and
  `<owner>` matches the authenticated account's identifier (which
  is `account.ID` since usernames are derived from email — TBD,
  per design). For the first-push exception:
  `refs/heads/jam/<session-id>/base` when the repo has no refs.
- **Force-push detection**: for each ref update, if the ref
  currently exists, check `git merge-base --is-ancestor old new`
  (or equivalent via go-git's `commitGraph` — verify in
  implementation). If old isn't an ancestor of new, it's a
  force-push.
- **Pack size limit**: 50 MB default; `cfg.Git.MaxPackBytes`
  (added to config). The HTTP handler reads `Content-Length`
  before streaming and rejects early if it exceeds.
- **Story decomposition**: 2 stories.
  1. `commit-validators` — Rejection types, trailer parser,
     scope-glob matcher, per-commit validation. depends_on: []
  2. `ref-and-size-validators` — ref-namespace check, force-push
     detection, pack size guard, top-level Validate() entry.
     depends_on: [commit-validators]

## Implementation Units

### Unit 1: Rejection types and error codes

**File**: `internal/portal/prereceive/types.go`
**Story**: `epic-portal-git-pre-receive-commit-validators`

```go
package prereceive

const (
    CodeScopeViolation       = "push.scope_violation"
    CodeRefNamespaceViolation = "push.ref_namespace_violation"
    CodeMissingTrailer       = "push.missing_trailer"
    CodeSizeLimit            = "push.size_limit"
    CodeForcePushRejected    = "push.force_push_rejected"
)

type Rejection struct {
    Code    string
    Message string
    Details map[string]any
}

type RefUpdate struct {
    Ref      string  // "refs/heads/jam/..."
    OldSHA   string  // empty if new ref
    NewSHA   string
}

type ValidateInput struct {
    Repo       *git.Repository
    Session    *store.Session
    Account    *store.Account
    Updates    []RefUpdate
    PackBytes  int64
}

type ValidateResult struct {
    OK         bool
    Rejections []Rejection
}
```

### Unit 2: Trailer parser

**File**: `internal/portal/prereceive/trailers.go`
**Story**: `epic-portal-git-pre-receive-commit-validators`

```go
// Trailers returns a map[Key]Value of trailers found in the last
// paragraph of the commit message. Per the git-interpret-trailers
// convention: lines of "Key: value" form, in the final block.
func Trailers(message string) map[string]string

// CheckRequiredTrailers returns the names of any missing required
// trailers, plus the names of empty-value trailers.
func CheckRequiredTrailers(message string, required []string) (missing []string)
```

### Unit 3: Scope matcher

**File**: `internal/portal/prereceive/scope.go`

```go
// ScopeMatcher compiles a list of globs (one per writable_scope entry)
// and provides Match(path) bool.
type ScopeMatcher struct { /* compiled globs */ }

func CompileScope(globs []string) (*ScopeMatcher, error)

func (m *ScopeMatcher) Match(path string) bool

// gobwas/glob with `Compile(pattern, '/')` to use forward-slash as
// separator. ** matches across separators.
```

### Unit 4: Commit walker + per-commit validation

**File**: `internal/portal/prereceive/commits.go`

```go
// WalkAndValidate visits every NEW commit in the proposed update
// (commits reachable from NewSHA but not from OldSHA). For each
// commit:
//   1. Check required trailers present
//   2. Collect changed paths vs first parent (or empty tree for root)
//   3. Check every path matches the scope
//
// Returns rejections grouped by code.
func WalkAndValidate(ctx context.Context, repo *git.Repository, update RefUpdate, scope *ScopeMatcher) []Rejection
```

### Unit 5: Ref namespace + force-push (story 2)

**File**: `internal/portal/prereceive/refs.go`
**Story**: `epic-portal-git-pre-receive-ref-and-size-validators`

```go
// ValidateRef checks ref namespace + force-push semantics.
//   - Ref name must be refs/heads/jam/<sessionID>/<accountKey>/<branch>
//     OR refs/heads/jam/<sessionID>/base when repo is empty
//   - Force-push detection: if OldSHA != "" and !ancestor(OldSHA, NewSHA),
//     reject with push.force_push_rejected
//   - "Shared refs" (base after first push, draft) reject any non-fast-forward
//     update.
func ValidateRef(ctx context.Context, repo *git.Repository, sessionID, accountKey string, update RefUpdate) []Rejection
```

`accountKey` resolution: for v1, use `account.ID` (ULID). Future
work could allow username/email-prefix as a more readable
namespace; documented as a follow-up.

### Unit 6: Pack size guard

**File**: `internal/portal/prereceive/size.go`

```go
func CheckPackSize(packBytes int64, maxBytes int64) (Rejection, bool)
```

### Unit 7: Top-level Validate

**File**: `internal/portal/prereceive/validate.go`

```go
type Validator struct {
    MaxPackBytes int64
}

func (v *Validator) Validate(ctx context.Context, in ValidateInput) (ValidateResult, error) {
    var rej []Rejection
    if r, ok := CheckPackSize(in.PackBytes, v.MaxPackBytes); !ok { rej = append(rej, r) }
    scope, err := CompileScope(parseScope(in.Session.WritableScope))
    if err != nil { return ValidateResult{}, err }
    for _, u := range in.Updates {
        rej = append(rej, ValidateRef(ctx, in.Repo, in.Session.ID, in.Account.ID, u)...)
        rej = append(rej, WalkAndValidate(ctx, in.Repo, u, scope)...)
    }
    return ValidateResult{OK: len(rej) == 0, Rejections: rej}, nil
}
```

## Story decomposition

- `epic-portal-git-pre-receive-commit-validators`: Units 1-4 + tests
- `epic-portal-git-pre-receive-ref-and-size-validators`: Units 5-7 + tests

## Testing

- Trailer parser: messages with no trailers, with trailers, with trailers in wrong block, with empty values
- Scope matcher: `docs/**`, `src/**.go`, `*.md`, mixed; non-matching paths rejected
- Commit walker: chain of 3 commits, mid-commit missing trailer, mid-commit out-of-scope path
- Ref validator: in-namespace ok, wrong-owner rejected, base after creation rejected, draft rejected, force-push rejected
- Pack size: exact, under, over the limit

## go.mod additions

- `github.com/gobwas/glob@latest` for `**` glob support

## Risks

- **Trailer parser edge cases**: `git-interpret-trailers` has
  intricate rules (line-folding, separator characters, etc.).
  Mitigation: implement a simple "last paragraph, key:value lines"
  parser for v1; document the simplification. The CC plugin's
  teaching skill instructs agents to author trailers in the simple
  form, so this is acceptable.
- **Force-push detection cost**: walking ancestry for each ref
  update can be slow on large repos. Mitigation: bounded walk
  (limit depth); use `go-git`'s commit-graph cache.

## Implementation summary

Both child stories done. Push policy validators landed: trailers, scope, ref namespace, force-push, pack size, top-level Validator.

### Verification
- `go build ./...` clean
- `go test ./internal/portal/prereceive/...` green

## Review (2026-05-16)

**Verdict**: Approve

**Notes**: Capability complete. The smart-http feature can now import `Validator` and call it on every receive-pack invocation.
