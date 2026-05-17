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
