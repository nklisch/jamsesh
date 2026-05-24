package prereceive

import (
	"testing"
)

func TestCompileScope(t *testing.T) {
	t.Run("valid patterns compile", func(t *testing.T) {
		_, err := CompileScope([]string{"docs/**", "src/*.go", "*.md"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("empty pattern list", func(t *testing.T) {
		m, err := CompileScope(nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if m.Match("anything.txt") {
			t.Error("empty scope should deny all paths")
		}
	})

	// Regression: gobwas/glob@v0.2.3 silently compiles patterns with an
	// unclosed "{" that has a literal prefix (e.g. "0{", "src/{") without
	// returning an error. The resulting matcher panics on Match when the input
	// matches the literal prefix. probeGlob must surface these as compile-time
	// errors so the portal never stores a pattern that will panic on push.
	//
	// Original fuzz trigger: seed fc37b996e5096fc7 — pattern "0{", path "0".
	// The panic: runtime error: slice bounds out of range [:2] with length 1,
	// inside gobwas/glob Row.matchAll.
	//
	// Note: a bare "{" (no literal prefix) does NOT panic — gobwas/glob treats
	// it as an empty alternatives group that matches only the empty string.
	// Only patterns of the form "<literal-prefix>{" trigger the panic, because
	// the literal prefix produces a two-element matcher list that matchAll then
	// accesses out of bounds.
	malformedPatterns := []struct {
		name    string
		pattern string
	}{
		// Primary fuzz trigger.
		{name: "unclosed brace after digit", pattern: "0{"},
		// Same class: alpha literal prefix.
		{name: "unclosed brace after alpha", pattern: "a{"},
		// Path prefix before unclosed brace — exercises longer byte-prefix probe.
		{name: "unclosed brace after path prefix", pattern: "src/{"},
	}

	for _, tc := range malformedPatterns {
		t.Run("rejects malformed: "+tc.name, func(t *testing.T) {
			_, err := CompileScope([]string{tc.pattern})
			if err == nil {
				t.Errorf("CompileScope(%q) returned nil error; want error for malformed glob (would panic on Match)", tc.pattern)
			}
		})
	}
}

func TestScopeMatcher_Match(t *testing.T) {
	cases := []struct {
		name     string
		globs    []string
		path     string
		wantMatch bool
	}{
		// docs/** — recursive under docs/
		{name: "docs/** matches docs/foo/bar.md", globs: []string{"docs/**"}, path: "docs/foo/bar.md", wantMatch: true},
		{name: "docs/** matches docs/README.md", globs: []string{"docs/**"}, path: "docs/README.md", wantMatch: true},
		{name: "docs/** does not match src/foo.go", globs: []string{"docs/**"}, path: "src/foo.go", wantMatch: false},
		{name: "docs/** does not match README.md", globs: []string{"docs/**"}, path: "README.md", wantMatch: false},

		// *.md — single segment only
		{name: "*.md matches README.md", globs: []string{"*.md"}, path: "README.md", wantMatch: true},
		{name: "*.md does NOT match src/x.md", globs: []string{"*.md"}, path: "src/x.md", wantMatch: false},

		// **.md — recursive with .md suffix
		{name: "**.md matches src/x.md", globs: []string{"**.md"}, path: "src/x.md", wantMatch: true},
		{name: "**.md matches deeply/nested/doc.md", globs: []string{"**.md"}, path: "deeply/nested/doc.md", wantMatch: true},
		{name: "**.md does not match src/main.go", globs: []string{"**.md"}, path: "src/main.go", wantMatch: false},

		// src/**.go — recursive .go under src/
		{name: "src/**.go matches src/main.go", globs: []string{"src/**.go"}, path: "src/main.go", wantMatch: true},
		{name: "src/**.go matches src/sub/pkg.go", globs: []string{"src/**.go"}, path: "src/sub/pkg.go", wantMatch: true},
		{name: "src/**.go does not match docs/foo.go", globs: []string{"src/**.go"}, path: "docs/foo.go", wantMatch: false},

		// Multiple patterns — union
		{name: "union: docs/** or src/*.go matches docs/a.md", globs: []string{"docs/**", "src/*.go"}, path: "docs/a.md", wantMatch: true},
		{name: "union: docs/** or src/*.go matches src/main.go", globs: []string{"docs/**", "src/*.go"}, path: "src/main.go", wantMatch: true},
		{name: "union: docs/** or src/*.go does not match README.md", globs: []string{"docs/**", "src/*.go"}, path: "README.md", wantMatch: false},

		// Exact file
		{name: "exact file match", globs: []string{"go.mod"}, path: "go.mod", wantMatch: true},
		{name: "exact file no match", globs: []string{"go.mod"}, path: "go.sum", wantMatch: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m, err := CompileScope(tc.globs)
			if err != nil {
				t.Fatalf("CompileScope: %v", err)
			}
			got := m.Match(tc.path)
			if got != tc.wantMatch {
				t.Errorf("Match(%q) with globs %v: got %v, want %v", tc.path, tc.globs, got, tc.wantMatch)
			}
		})
	}
}

func TestValidateWritableScope(t *testing.T) {
	cases := []struct {
		name      string
		raw       string
		wantOK    bool
		msgContains string
	}{
		{name: "empty string -> ok (deny-all)", raw: "", wantOK: true},
		{name: "empty json array -> ok (deny-all)", raw: "[]", wantOK: true},
		{name: "well-formed src/** -> ok", raw: `["src/**"]`, wantOK: true},
		{name: "multiple well-formed globs -> ok", raw: `["docs/**","src/*.go"]`, wantOK: true},
		{name: "non-json payload -> err with parse message", raw: "not json", wantOK: false, msgContains: "writable_scope must be a JSON array of strings"},
		{name: "json string (not array) -> err with parse message", raw: `"src/**"`, wantOK: false, msgContains: "writable_scope must be a JSON array of strings"},
		{name: "malformed glob (unclosed brace after path prefix) -> err with bad pattern syntax", raw: `["docs/{"]`, wantOK: false, msgContains: "bad pattern syntax"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			msg, ok := ValidateWritableScope(tc.raw)
			if ok != tc.wantOK {
				t.Errorf("ValidateWritableScope(%q): ok=%v, want %v (msg=%q)", tc.raw, ok, tc.wantOK, msg)
			}
			if !tc.wantOK && tc.msgContains != "" {
				if msg == "" {
					t.Errorf("ValidateWritableScope(%q): want non-empty msg containing %q, got empty", tc.raw, tc.msgContains)
				} else if !contains(msg, tc.msgContains) {
					t.Errorf("ValidateWritableScope(%q): msg=%q, want to contain %q", tc.raw, msg, tc.msgContains)
				}
			}
			if tc.wantOK && msg != "" {
				t.Errorf("ValidateWritableScope(%q): want empty msg, got %q", tc.raw, msg)
			}
		})
	}
}

// contains reports whether substr is a substring of s. Avoids the strings
// import dependency in this test file just for one call site.
func contains(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
