package prereceive

import (
	"context"
	"strings"
	"testing"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/storage/memory"
)

// FuzzRefNamespaceValidate exercises checkRefNamespace with arbitrary ref
// strings, sessionIDs, and accountKeys.
//
// Properties asserted:
//  1. checkRefNamespace never panics on any input.
//  2. Any ref that is allowed (allowed=true) must begin with the expected
//     prefix "refs/heads/jam/<sessionID>/<accountKey>/" or be the special
//     base ref "refs/heads/jam/<sessionID>/base" — never any other form.
//  3. A ref that does not have the "refs/heads/jam/" prefix is always denied.
//  4. A ref containing a path-traversal segment (".." or URL-encoded variants)
//     must never be allowed.
//  5. If isBase is true the ref must end in "/base" (and only two segments
//     after the jam/ prefix).
//
// Note: checkRefNamespace is unexported; this file is in package prereceive
// (not prereceive_test) so it has direct access. An empty in-memory repository
// is used so the base-ref path (which calls repoIsEmpty) does not panic.
func FuzzRefNamespaceValidate(f *testing.F) {
	// Seed: known-good user ref.
	f.Add("refs/heads/jam/sess-001/acc-alice/main", "sess-001", "acc-alice")
	f.Add("refs/heads/jam/sess-abc/user-xyz/feature/my-feature", "sess-abc", "user-xyz")

	// Seed: known-good base ref on empty repo (allowed=true, isBase=true).
	// We can't test with empty repo here — checkRefNamespace is called directly
	// and the empty-repo check is done inside; nil repo returns repoIsEmpty error
	// → returns (false, true). Seeding for namespace coverage.
	f.Add("refs/heads/jam/sess-001/base", "sess-001", "acc-alice")

	// Seed: server-managed draft ref — always rejected.
	f.Add("refs/heads/jam/sess-001/draft", "sess-001", "acc-alice")

	// Seed: wrong session.
	f.Add("refs/heads/jam/sess-OTHER/acc-alice/main", "sess-001", "acc-alice")

	// Seed: wrong owner.
	f.Add("refs/heads/jam/sess-001/acc-bob/main", "sess-001", "acc-alice")

	// Seed: missing branch segment.
	f.Add("refs/heads/jam/sess-001/acc-alice/", "sess-001", "acc-alice")

	// Seed: completely off-namespace.
	f.Add("refs/heads/main", "sess-001", "acc-alice")
	f.Add("refs/tags/v1.0", "sess-001", "acc-alice")
	f.Add("HEAD", "sess-001", "acc-alice")
	f.Add("", "sess-001", "acc-alice")

	// Seed: path-traversal in ref name.
	f.Add("refs/heads/jam/sess-001/acc-alice/../acc-bob/main", "sess-001", "acc-alice")
	f.Add("refs/heads/jam/../sess-001/acc-alice/main", "sess-001", "acc-alice")
	f.Add("refs/heads/jam/sess-001/acc-alice/%2e%2e/main", "sess-001", "acc-alice")
	f.Add("refs/heads/jam/sess-001/acc-alice/..%2Fmain", "sess-001", "acc-alice")

	// Seed: null bytes and control characters.
	f.Add("refs/heads/jam/sess-001/acc-alice/main\x00evil", "sess-001", "acc-alice")
	f.Add("refs/heads/jam/sess-001\x00/acc-alice/main", "sess-001", "acc-alice")

	// Seed: session/account contain slashes.
	f.Add("refs/heads/jam/sess/sub/acc/sub/branch", "sess/sub", "acc/sub")

	f.Fuzz(func(t *testing.T, ref, sessionID, accountKey string) {
		// Property 1 (no panic): enforced by fuzz runner.
		// Use an empty in-memory repo so repoIsEmpty doesn't panic on nil.
		emptyRepo, err := git.Init(memory.NewStorage(), nil)
		if err != nil {
			t.Fatalf("git.Init memory: %v", err)
		}
		ctx := context.Background()
		allowed, isBase := checkRefNamespace(ctx, emptyRepo, sessionID, accountKey, ref)

		if !allowed {
			// Nothing more to assert for rejected refs.
			return
		}

		// Property 2: allowed ref must have expected structure.
		const prefix = "refs/heads/jam/"
		if !strings.HasPrefix(ref, prefix) {
			t.Errorf("allowed ref %q does not start with %q", ref, prefix)
		}

		rest := ref[len(prefix):]
		parts := strings.SplitN(rest, "/", 3)

		if isBase {
			// Property 5: isBase=true means exactly two segments after prefix.
			if len(parts) != 2 || parts[1] != "base" {
				t.Errorf("isBase=true but ref %q is not the base ref form", ref)
			}
		} else {
			// Normal user ref: <sessionID>/<accountKey>/<branch>.
			if len(parts) < 3 {
				t.Errorf("allowed non-base ref %q has fewer than 3 segments after prefix", ref)
			}
			if parts[0] != sessionID {
				t.Errorf("allowed ref %q has sessionID %q, expected %q", ref, parts[0], sessionID)
			}
			if parts[1] != accountKey {
				t.Errorf("allowed ref %q has owner %q, expected accountKey %q", ref, parts[1], accountKey)
			}
			if parts[2] == "" {
				t.Errorf("allowed ref %q has empty branch segment", ref)
			}
		}

		// Property 3: already covered by prefix check above.

		// Property 4: path-traversal in the raw ref must never slip through.
		// We check the original (non-decoded) string for ".." segments.
		for _, seg := range strings.Split(ref, "/") {
			if seg == ".." {
				t.Errorf("allowed ref %q contains '..' segment — path-traversal bypass", ref)
			}
		}
		// URL-encoded variants: %2e%2e or %2E%2E (case-insensitive) should not
		// appear in a ref that was allowed, because no URL decoding is performed
		// and gobwas/glob operates on the raw string — but we flag it anyway.
		lower := strings.ToLower(ref)
		if strings.Contains(lower, "%2e%2e") || strings.Contains(lower, "%252e") {
			t.Errorf("allowed ref %q contains URL-encoded path-traversal sequence", ref)
		}
	})
}
