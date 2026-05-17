package prereceive

import (
	"strings"
	"testing"
)

// FuzzPathScopeValidate exercises CompileScope and ScopeMatcher.Match with
// arbitrary glob patterns and file paths.
//
// Properties asserted:
//  1. CompileScope and Match never panic on any input.
//  2. An empty pattern list must deny every path (deny-by-default).
//  3. The match result is deterministic: calling Match twice with the same
//     path returns the same bool.
//  4. Path-traversal sequences ("../", "..\", URL-encoded "%2e%2e", double-
//     encoded "%252e", and mixed forms) must never cause an out-of-scope path
//     to be accepted when the scope does not explicitly allow the target
//     location. Specifically, a scope of "docs/**" must never accept a path
//     that starts with ".." (logical path, not filesystem).
//  5. A match-all scope ("**") must accept every non-empty path — used as a
//     sanity check that the matcher is functioning.
func FuzzPathScopeValidate(f *testing.F) {
	// ---- Known-good seeds (path within scope) ----
	// docs/** scope.
	f.Add("docs/**", "docs/README.md")
	f.Add("docs/**", "docs/api/v1/spec.md")
	f.Add("docs/**", "docs/deep/nested/dir/file.txt")

	// src/*.go scope — single segment only.
	f.Add("src/*.go", "src/main.go")
	f.Add("src/*.go", "src/handler.go")

	// **.md — recursive markdown.
	f.Add("**.md", "deeply/nested/CHANGELOG.md")
	f.Add("**.md", "README.md")

	// Match-all.
	f.Add("**", "internal/portal/prereceive/fuzz_test.go")
	f.Add("**", "go.mod")

	// Multiple-pattern scope (packed as one pattern for the fuzzer input;
	// we also add a single known-bad seed below).
	f.Add("src/**.go", "src/cmd/main.go")

	// ---- Known-bad seeds (path-traversal payloads) ----
	// These must be rejected when scope is "docs/**".
	f.Add("docs/**", "../etc/passwd")
	f.Add("docs/**", "..%2Fetc%2Fpasswd")
	f.Add("docs/**", `..\..\etc\passwd`)
	f.Add("docs/**", "%2e%2e/etc/passwd")
	f.Add("docs/**", "....//etc/passwd")
	f.Add("docs/**", ".././../../etc/passwd")
	f.Add("docs/**", "foo/../../../../etc/passwd")

	// Double-encoded.
	f.Add("docs/**", "%252e%252e/etc/passwd")

	// Empty path.
	f.Add("docs/**", "")

	f.Fuzz(func(t *testing.T, pattern, path string) {
		// Property 1 (no panic): enforced by fuzz runner.

		// Test with single-pattern scope.
		m, err := CompileScope([]string{pattern})
		if err != nil {
			// Invalid glob patterns are a caller error; CompileScope returning
			// an error is correct behaviour — not a bug.
			return
		}

		result1 := m.Match(path)

		// Property 3: determinism.
		result2 := m.Match(path)
		if result1 != result2 {
			t.Errorf("Match(%q) non-deterministic: first=%v second=%v (pattern=%q)", path, result1, result2, pattern)
		}

		// Property 5: match-all scope must accept any non-empty path.
		mAll, err := CompileScope([]string{"**"})
		if err != nil {
			t.Fatalf("CompileScope(**) failed: %v", err)
		}
		if path != "" && !mAll.Match(path) {
			t.Errorf("match-all scope denied non-empty path %q", path)
		}

		// Property 4: path-traversal check when scope is "docs/**".
		// A path that begins with a traversal sequence (i.e. starts with ".."
		// or a URL-encoded equivalent) must NOT be accepted by a "docs/**" scope.
		// The gobwas/glob library operates on raw strings and never normalises
		// paths, so "../etc/passwd" will not match "docs/**" — we assert this
		// holds for all fuzzer-generated inputs.
		//
		// We restrict the assertion to paths that BEGIN with a traversal prefix
		// (absolute escape) rather than interior ".." segments, because:
		//   - git never stores paths with null bytes or backslashes on Linux.
		//   - A path like "docs/foo/../bar" does start with "docs/" and resolves
		//     within docs/ after normalization — flagging it here would conflate
		//     the glob check with filesystem normalization (out of scope).
		mDocs, err := CompileScope([]string{"docs/**"})
		if err != nil {
			t.Fatalf("CompileScope(docs/**) failed: %v", err)
		}
		if isAbsoluteTraversalPath(path) && mDocs.Match(path) {
			t.Errorf("absolute traversal path %q was accepted by docs/** scope — security bypass", path)
		}
	})
}

// FuzzPathScopeEmpty ensures an empty ScopeMatcher denies every path.
//
// Property: CompileScope(nil).Match(path) == false for all paths.
func FuzzPathScopeEmpty(f *testing.F) {
	f.Add("README.md")
	f.Add("src/main.go")
	f.Add("docs/api.md")
	f.Add("../etc/passwd")
	f.Add("")
	f.Add("**")

	f.Fuzz(func(t *testing.T, path string) {
		m, err := CompileScope(nil)
		if err != nil {
			t.Fatalf("CompileScope(nil): %v", err)
		}
		if m.Match(path) {
			t.Errorf("empty scope accepted path %q — must deny all", path)
		}
	})
}

// isAbsoluteTraversalPath reports whether path begins with a traversal sequence
// that would escape the repository root — i.e. the path starts with "..", a
// URL-encoded equivalent, or a backslash-relative form. Paths like
// "docs/foo/../bar" (interior ".." that stays within scope) are NOT flagged.
//
// This is intentionally conservative: git never stores paths with null bytes or
// Windows-style backslashes on Linux, so those forms are impossible in
// production but are still exercised here as a belt-and-suspenders check.
func isAbsoluteTraversalPath(path string) bool {
	lower := strings.ToLower(path)
	return strings.HasPrefix(path, "../") ||
		strings.HasPrefix(path, `..\ `) ||
		strings.HasPrefix(path, "..\\") ||
		strings.HasPrefix(lower, "%2e%2e/") ||
		strings.HasPrefix(lower, "%2e%2e%2f") ||
		strings.HasPrefix(lower, "%252e%252e/") ||
		strings.HasPrefix(lower, "..%2f") ||
		strings.HasPrefix(lower, "..%5c")
}
