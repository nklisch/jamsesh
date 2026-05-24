package prereceive

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
)

// ValidateWritableScope parses the JSON-encoded writable_scope payload and
// compiles each glob through CompileScope. It returns ("", true) when the
// payload is acceptable (including the deny-all empty-string and "[]"
// cases), or (message, false) when the payload is unparseable JSON or
// contains a malformed glob. The message is suitable for the body of a
// `session.invalid_writable_scope` 400 envelope.
//
// Callers: sessions.CreateSession (front door), sessions.PatchSession
// (mutation), playground.CreatePlaygroundSession (front door). All three
// must give identical answers for identical inputs so the front-door
// contract is consistent regardless of which surface the session was
// created through.
//
// This export is intentionally a superset of the package-internal
// parseWritableScope helper used by Validator.Validate — that one returns
// the parsed glob list (which the validator hot-path needs), whereas this
// export folds parse-error and compile-error reporting into a single
// caller-friendly (message, ok) shape.
func ValidateWritableScope(raw string) (msg string, ok bool) {
	if raw == "" {
		return "", true
	}
	var globs []string
	if err := json.Unmarshal([]byte(raw), &globs); err != nil {
		return fmt.Sprintf("writable_scope must be a JSON array of strings: %v", err), false
	}
	if _, err := CompileScope(globs); err != nil {
		return err.Error(), false
	}
	return "", true
}

// normalizeForDoublestar rewrites gobwas/glob-style patterns to their
// doublestar equivalents. In gobwas/glob (with '/' separator), "**" always
// spans directory boundaries regardless of surrounding context. In doublestar,
// "**" must be surrounded by '/' to act as a recursive wildcard; a "**" that
// is immediately followed by a non-'/' character is treated as a single-segment
// "*" by doublestar (matching bash globstar behavior).
//
// To preserve the gobwas semantics expected by callers, any "**" followed by a
// non-'/' character is rewritten so that the non-'/' suffix becomes its own
// segment: "**.ext" → "**/*.ext", "src/**.go" → "src/**/*.go".
// Patterns where "**" is already at end-of-string or followed by '/' are left
// unchanged ("docs/**", "**").
func normalizeForDoublestar(p string) string {
	if !strings.Contains(p, "**") {
		return p
	}
	var b strings.Builder
	i := 0
	for i < len(p) {
		if i+1 < len(p) && p[i] == '*' && p[i+1] == '*' {
			b.WriteString("**")
			i += 2
			// Insert "/*" so the suffix becomes a new path segment.
			if i < len(p) && p[i] != '/' {
				b.WriteString("/*")
			}
		} else {
			b.WriteByte(p[i])
			i++
		}
	}
	return b.String()
}

// ScopeMatcher holds validated, normalized glob patterns that define the
// writable scope for a session. Patterns use '/' as the path separator, so
// "**" matches across directory boundaries (e.g. "docs/**" matches
// "docs/foo/bar.md").
//
// github.com/bmatcuk/doublestar is used instead of path/filepath.Match because
// the stdlib does not support "**" recursive matching. Unlike the former
// gobwas/glob dependency, doublestar validates patterns at parse time and
// never panics on malformed input.
type ScopeMatcher struct {
	patterns []string // normalized, validated doublestar patterns
}

// CompileScope compiles a list of glob patterns into a ScopeMatcher. Each
// pattern is normalized (see normalizeForDoublestar) and validated at compile
// time; malformed patterns (e.g. unclosed "{") cause an immediate error rather
// than a deferred panic.
//
// Returns an error if any pattern fails validation.
func CompileScope(patterns []string) (*ScopeMatcher, error) {
	normalized := make([]string, 0, len(patterns))
	for _, p := range patterns {
		n := normalizeForDoublestar(p)
		if !doublestar.ValidatePattern(n) {
			return nil, fmt.Errorf("prereceive: invalid scope glob %q: bad pattern syntax", p)
		}
		normalized = append(normalized, n)
	}
	return &ScopeMatcher{patterns: normalized}, nil
}

// Match reports whether path is covered by at least one glob in the scope.
// path should be the slash-separated file path relative to the repo root
// (as returned by go-git tree-diff, e.g. "src/main.go" or "docs/api.md").
//
// If the ScopeMatcher has no patterns, Match returns false (deny-by-default).
func (m *ScopeMatcher) Match(path string) bool {
	for _, p := range m.patterns {
		matched, err := doublestar.Match(p, path)
		if err == nil && matched {
			return true
		}
	}
	return false
}
