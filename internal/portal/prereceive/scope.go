package prereceive

import (
	"fmt"

	"github.com/gobwas/glob"
)

// probeGlob tries a series of short strings against g with a deferred recover,
// returning an error if any Match call panics. This is a workaround for
// gobwas/glob@v0.2.3 silently compiling malformed patterns (e.g. unclosed "{")
// without returning an error, then panicking on the first Match call.
//
// The probe inputs cover the known-bad trigger: a string that equals the
// literal prefix before an unclosed "{", causing the internal Row.matchAll to
// access a slice out of bounds. We also probe with the raw pattern text itself
// and common short strings to catch other variants.
//
// See upstream issue: gobwas/glob silently accepts "X{" patterns.
// Filed in jamsesh as: bug-gobwas-glob-panic-on-malformed-pattern.
func probeGlob(pattern string, g glob.Glob) (probeErr error) {
	probe := func(s string) (err error) {
		defer func() {
			if r := recover(); r != nil {
				err = fmt.Errorf("glob pattern %q panicked on Match(%q): %v", pattern, s, r)
			}
		}()
		g.Match(s)
		return nil
	}

	// Probe inputs: empty string, the raw pattern text, each byte-prefix of the
	// pattern (up to the first 8 bytes), and a few common path strings.
	//
	// We iterate by byte (not rune) so we never slice the string at an invalid
	// UTF-8 boundary — even if the pattern contains arbitrary bytes the fuzzer
	// generated.
	candidates := []string{"", pattern}
	for i := 1; i <= len(pattern) && i <= 8; i++ {
		candidates = append(candidates, pattern[:i])
	}
	candidates = append(candidates, "a", "z", "/", "0", ".")

	for _, s := range candidates {
		if err := probe(s); err != nil {
			return err
		}
	}
	return nil
}

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
		// gobwas/glob@v0.2.3 silently compiles some malformed patterns (e.g.
		// unclosed "{") without error, then panics on Match. Probe immediately
		// to surface such patterns as compile-time errors.
		if probeErr := probeGlob(p, g); probeErr != nil {
			return nil, fmt.Errorf("prereceive: invalid scope glob %q: %w", p, probeErr)
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
