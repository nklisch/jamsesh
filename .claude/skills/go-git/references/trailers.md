# Commit trailers — jamsesh schema and Go implementation

## Schema (per `docs/PROTOCOL.md`)

**Required on every session commit** (enforced by pre-receive):

| Key | Value | Notes |
|-----|-------|-------|
| `Jam-Session` | `<session-id>` | UUID |
| `Jam-Turn` | `<turn-number>` | integer |
| `Jam-Author` | `<user-id-or-handle>` | matches the authenticated push user |

**Optional, recognized:**

| Key | Value | Set by |
|-----|-------|--------|
| `Resolves-Conflict` | `<conflict-event-id>` | author of a resolution commit |
| `Auto-Merger` | `true` | auto-merger on every merge commit it creates |
| `Source-Commit` | `<sha>` | auto-merger |
| `Source-Ref` | `jam/<session>/<user>/<branch>` | auto-merger |
| `Auto-Resolved` | `whitespace` \| `additions` \| `identical` | auto-merger when a safe-auto-resolve heuristic fired |

## Format rules

Per `git interpret-trailers(1)`:

- Trailer block sits at the end of the commit message.
- Preceded by at least one blank line.
- Default separator is `: ` (colon space).
- Key character set: `[A-Za-z][A-Za-z0-9-]*`.
- Folded continuation lines start with whitespace.
- The rigorous detection rule allows mixed paragraphs at 25%+ trailer
  density. For jamsesh we use the stricter rule: the trailing block must
  be entirely well-formed trailer lines (with folded continuations) —
  this is sufficient because we emit our own trailers and reject anything
  missing required ones.

## Parser / composer

```go
package trailer

import (
    "regexp"
    "strings"

    "github.com/go-git/go-git/v5/plumbing"
    "github.com/go-git/go-git/v5/plumbing/object"
)

var trailerLine = regexp.MustCompile(`^([A-Za-z][A-Za-z0-9-]*):\s+(.+)$`)

type Trailer struct {
    Key   string
    Value string
}

// Parse extracts the trailer block from a commit message. Returns nil
// if no trailer block is present.
func Parse(msg string) []Trailer {
    lines := strings.Split(strings.TrimRight(msg, "\n"), "\n")
    // Find the last blank line.
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
    for _, l := range block {
        if l == "" {
            return nil // mid-block blank — not a trailer block
        }
        if (l[0] == ' ' || l[0] == '\t') && len(out) > 0 {
            // Folded continuation.
            out[len(out)-1].Value += "\n" + l
            continue
        }
        m := trailerLine.FindStringSubmatch(l)
        if m == nil {
            return nil // not a pure trailer block
        }
        out = append(out, Trailer{Key: m[1], Value: m[2]})
    }
    return out
}

// Find returns the first value for a key. Empty if absent.
func Find(msg, key string) string {
    for _, t := range Parse(msg) {
        if t.Key == key {
            return t.Value
        }
    }
    return ""
}

// Compose appends trailers to a body. Inserts the required blank-line
// separator. Existing trailers in body are preserved (they appear before
// the appended ones); use ParseAndStrip if you need to replace.
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

// ParseAndStrip returns (body, trailers) — body has the trailer block
// removed. Use this when composing a NEW commit that should not double
// up on existing trailers.
func ParseAndStrip(msg string) (body string, trailers []Trailer) {
    trailers = Parse(msg)
    if trailers == nil {
        return strings.TrimRight(msg, "\n"), nil
    }
    lines := strings.Split(strings.TrimRight(msg, "\n"), "\n")
    last := -1
    for i := len(lines) - 1; i >= 0; i-- {
        if strings.TrimSpace(lines[i]) == "" {
            last = i
            break
        }
    }
    body = strings.Join(lines[:last], "\n")
    return body, trailers
}

// MissingTrailerErr is returned by ValidateRequired.
type MissingTrailerErr struct {
    Commit  plumbing.Hash
    Missing []string
}

func (e *MissingTrailerErr) Error() string {
    return "commit " + e.Commit.String() + " missing trailers: " +
        strings.Join(e.Missing, ", ")
}

// ValidateRequired enforces jamsesh's required-trailer policy.
// Called by pre-receive for each commit in the pushed pack.
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
        return &MissingTrailerErr{Commit: c.Hash, Missing: missing}
    }
    return nil
}
```

## Auto-merger usage

```go
trailers := []trailer.Trailer{
    {Key: "Auto-Merger", Value: "true"},
    {Key: "Source-Commit", Value: source.Hash.String()},
    {Key: "Source-Ref", Value: sourceRef},
}
if heuristic != "" {
    trailers = append(trailers, trailer.Trailer{
        Key: "Auto-Resolved", Value: heuristic,
    })
}
msg := trailer.Compose(
    fmt.Sprintf("Merge %s into draft", source.Hash.String()[:7]),
    trailers,
)
```

## `Resolves-Conflict` auto-closure

The auto-merger's `outcomes` feature reads the trailer when a merge
succeeds:

```go
if eventID := trailer.Find(source.Message, "Resolves-Conflict"); eventID != "" {
    if err := db.CloseConflictEvent(ctx, eventID, source.Hash.String()); err != nil {
        // Silent no-op on missing event; log warning on event already
        // closed with a different resolving_commit_sha. Per
        // epic-auto-merger.md design decision.
    }
}
```
