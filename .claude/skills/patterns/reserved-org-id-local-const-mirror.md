# Pattern: Reserved-org-ID local-const mirror

Cross-cutting constants whose canonical home (`playground.ReservedOrgID
= "org_playground"`) would create an import cycle for downstream
packages are mirrored as **package-local** `const` declarations in each
consumer, with a comment block pinning the local copy to the canonical
source. Test files do the same inline (`const playgroundOrgID =
"org_playground"` at the top of the function) so subtests don't need
to import the consumer either.

## Rationale

`playground.ReservedOrgID` lives in the `playground` package because
that package owns the lifecycle of the reserved org (provisioning,
destruction, sweep). But `githttp`, `sessions`, `comments`, and
`prereceive` all need to *check* the orgID without depending on the
playground package — depending would either create a cycle (playground
imports sessions, sessions can't import playground) or unnecessarily
widen the dependency graph (prereceive shouldn't pull in the full
playground handler tree to compare one string).

The mirror solves it: a one-line `const playgroundOrgID =
"org_playground"` with a comment naming the canonical source. If the
canonical value ever changes, grep finds every mirror at once; the
comment block makes the dependency relationship explicit so reviewers
don't mistake the duplication for accident.

The convention is strict: the constant is **always lowercase**
(`playgroundOrgID`, not exported), **always named after the role**
(`playgroundOrgID`, not `reservedOrgID`), and **always carries a
comment** naming the canonical source and the import-cycle rationale.

## Examples

### Example 1: sessions package

**File**: `internal/portal/sessions/handler.go:31`
```go
// playgroundOrgID is the hard-coded org_id for the reserved playground org.
// Defined locally to avoid an import cycle. Value must match playground.ReservedOrgID.
const playgroundOrgID = "org_playground"
```

### Example 2: comments package

**File**: `internal/portal/comments/service.go:21`
```go
// playgroundOrgID is the hard-coded org_id for the reserved playground org.
// Defined locally to avoid an import cycle (comments → playground would be
// cyclic). Value must match playground.ReservedOrgID.
const playgroundOrgID = "org_playground"
```

### Example 3: githttp package

**File**: `internal/portal/githttp/handler.go:23`
```go
// playgroundOrgID is the hard-coded org_id for the reserved playground org.
// Defined locally to avoid an import cycle (githttp → playground would be
// cyclic). Value must match playground.ReservedOrgID.
const playgroundOrgID = "org_playground"
```

### Example 4: prereceive package — slightly different local name

**File**: `internal/portal/prereceive/playground_caps.go:14`
```go
// reservedPlaygroundOrgID is the org_id for the playground org. We hard-code
// it here to avoid an import cycle (prereceive → playground would be cyclic);
// the value is also pinned as playground.ReservedOrgID.
const reservedPlaygroundOrgID = "org_playground"
```

(Production occurrences: 4 — sessions, comments, githttp, prereceive.
Test-scope inline mirrors at `internal/portal/sessions/handler_test.go:888`,
`internal/portal/githttp/receive_pack_test.go:1146`,
`internal/portal/comments/service_test.go:938` use the same shape inside
their test function scopes.)

## When to Use

- A new consumer package needs to compare a session's `orgID` against
  the reserved playground value (or any other reserved/system identity
  later added to the same family).
- The canonical owner of the value is already imported by something
  that imports the consumer (the cycle test); or the canonical owner
  carries a heavy dependency graph the consumer doesn't need.

## When NOT to Use

- **The canonical package is import-safe** — just import the constant
  directly: `playground.ReservedOrgID`. Mirroring only when it's
  needed to break a cycle or trim dependency weight.
- **The value is config-driven or per-deployment** — those go through
  `Config` structs, not package consts.
- **Bare value duplication without the comment block** — a naked
  `const playgroundOrgID = "org_playground"` reads as a magic string.
  The comment is what makes the duplication a *pattern* rather than a
  smell.

## Common Violations

- **Re-deriving the value from a config field** at the call site
  (`if orgID == cfg.PlaygroundOrgID { ... }`) — the value isn't
  configurable in practice; threading it through config adds wiring
  for no flexibility.
- **Forgetting the canonical-source comment** when adding a new
  mirror. Without the comment, the next reader can't tell whether the
  duplication is intentional or a copy-paste oversight, and grep loses
  half its value.
- **Capitalizing or exporting the mirror** (`PlaygroundOrgID`). The
  mirror is a workaround, not a public API; exporting it tempts
  consumers to depend on the *mirror* rather than the canonical
  source, defeating the purpose.
- **Drifting the local name and value out of step** — if you must use
  a different local identifier (e.g. `reservedPlaygroundOrgID` in
  prereceive), the value still has to be byte-identical to
  `playground.ReservedOrgID`. A `_ = playground.ReservedOrgID` blank
  reference at package-init time is overkill; the comment block is
  the discipline.

#### Index entry
- **reserved-org-id-local-const-mirror**: Cross-cutting reserved identifiers (`playgroundOrgID = "org_playground"`) are mirrored as lowercase package-local `const` in each consumer with a comment pinning them to `playground.ReservedOrgID` to break import cycles without widening the dependency graph.
