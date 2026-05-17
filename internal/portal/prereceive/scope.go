package prereceive

import (
	"fmt"

	"github.com/gobwas/glob"
)

// ScopeMatcher holds compiled glob patterns that define the writable scope for
// a session. Globs use "/" as the path separator, so "**" matches across
// directory boundaries (e.g. "docs/**" matches "docs/foo/bar.md").
//
// gobwas/glob is used instead of path/filepath.Match because the stdlib does
// not support "**" recursive matching.
type ScopeMatcher struct {
	globs []glob.Glob
	raw   []string // kept for error messages
}

// CompileScope compiles a list of glob patterns into a ScopeMatcher. Each
// pattern is compiled with "/" as the separator, so "**" spans directories
// while "*" matches within a single path segment.
//
// Returns an error if any pattern fails to compile.
func CompileScope(patterns []string) (*ScopeMatcher, error) {
	compiled := make([]glob.Glob, 0, len(patterns))
	for _, p := range patterns {
		g, err := glob.Compile(p, '/')
		if err != nil {
			return nil, fmt.Errorf("prereceive: invalid scope glob %q: %w", p, err)
		}
		compiled = append(compiled, g)
	}
	raw := make([]string, len(patterns))
	copy(raw, patterns)
	return &ScopeMatcher{globs: compiled, raw: raw}, nil
}

// Match reports whether path is covered by at least one glob in the scope.
// path should be the slash-separated file path relative to the repo root
// (as returned by go-git tree-diff, e.g. "src/main.go" or "docs/api.md").
//
// If the ScopeMatcher has no patterns, Match returns false (deny-by-default).
func (m *ScopeMatcher) Match(path string) bool {
	for _, g := range m.globs {
		if g.Match(path) {
			return true
		}
	}
	return false
}
