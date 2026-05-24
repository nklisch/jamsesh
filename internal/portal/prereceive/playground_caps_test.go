package prereceive

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"jamsesh/internal/db/store"
)

// makePlaygroundSession returns a minimal *store.Session for the playground org.
func makePlaygroundSession(id string) *store.Session {
	return &store.Session{
		ID:            id,
		OrgID:         reservedPlaygroundOrgID,
		WritableScope: `["**"]`,
	}
}

// makeDurableSession returns a minimal *store.Session for a non-playground org.
func makeDurableSession(id, orgID string) *store.Session {
	return &store.Session{
		ID:            id,
		OrgID:         orgID,
		WritableScope: `["**"]`,
	}
}

// writeRepoFile writes a file of the given size under repoPath, creating
// intermediate directories as needed. Used to simulate an existing repo.
func writeRepoFile(t *testing.T, repoPath, relName string, size int) {
	t.Helper()
	full := filepath.Join(repoPath, relName)
	if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(full), err)
	}
	if err := os.WriteFile(full, make([]byte, size), 0o644); err != nil {
		t.Fatalf("write %s: %v", full, err)
	}
}

// ---------------------------------------------------------------------------
// Unit tests for CheckPlaygroundCaps
// ---------------------------------------------------------------------------

// TestCheckPlaygroundCaps_DurableSessionPassesThrough verifies that pushes to
// non-playground sessions are never checked — the fast-path org_id guard must
// return (Rejection{}, true) before any size measurement occurs.
func TestCheckPlaygroundCaps_DurableSessionPassesThrough(t *testing.T) {
	v := &Validator{PlaygroundMaxContentBytes: 100}

	in := ValidateInput{
		Session:   makeDurableSession("sess-durable", "org-mycompany"),
		Account:   makeAccount("acc-1"),
		PackBytes: 999999, // would fail the size check if applied
	}

	r, ok := v.CheckPlaygroundCaps(context.Background(), in)
	if !ok {
		t.Errorf("durable session push should always pass through; got rejection: %+v", r)
	}
}

// TestCheckPlaygroundCaps_NilSession verifies that a nil session is a
// no-op (treated as non-playground).
func TestCheckPlaygroundCaps_NilSession(t *testing.T) {
	v := &Validator{PlaygroundMaxContentBytes: 100}

	in := ValidateInput{
		Session:   nil,
		PackBytes: 999999,
	}

	_, ok := v.CheckPlaygroundCaps(context.Background(), in)
	if !ok {
		t.Error("nil session should be treated as non-playground (pass-through)")
	}
}

// TestCheckPlaygroundCaps_NoCap verifies that a zero PlaygroundMaxContentBytes
// skips the check and allows all pushes.
func TestCheckPlaygroundCaps_NoCap(t *testing.T) {
	v := &Validator{PlaygroundMaxContentBytes: 0}

	in := ValidateInput{
		Session:   makePlaygroundSession("sess-pg-1"),
		Account:   makeAccount("acc-1"),
		PackBytes: 99999999,
	}

	_, ok := v.CheckPlaygroundCaps(context.Background(), in)
	if !ok {
		t.Error("cap=0 should skip the check and allow all pushes")
	}
}

// TestCheckPlaygroundCaps_WithinCap verifies that a push whose total
// (current repo size + pack) is at or below the cap is allowed.
func TestCheckPlaygroundCaps_WithinCap(t *testing.T) {
	repoPath := t.TempDir()
	// Existing repo has 1000 bytes of content.
	writeRepoFile(t, repoPath, "objects/pack/pack-abc.pack", 1000)

	v := &Validator{PlaygroundMaxContentBytes: 2000}

	in := ValidateInput{
		Session:   makePlaygroundSession("sess-pg-2"),
		Account:   makeAccount("acc-1"),
		PackBytes: 999, // 1000 + 999 = 1999 ≤ 2000: allowed
		RepoPath:  repoPath,
	}

	_, ok := v.CheckPlaygroundCaps(context.Background(), in)
	if !ok {
		t.Error("push within cap should be allowed")
	}
}

// TestCheckPlaygroundCaps_ExactlyAtCap verifies that a push that brings the
// total exactly to the cap is allowed (boundary: total == max is NOT exceeded).
func TestCheckPlaygroundCaps_ExactlyAtCap(t *testing.T) {
	repoPath := t.TempDir()
	writeRepoFile(t, repoPath, "objects/pack/pack-abc.pack", 1000)

	v := &Validator{PlaygroundMaxContentBytes: 2000}

	in := ValidateInput{
		Session:   makePlaygroundSession("sess-pg-3"),
		Account:   makeAccount("acc-1"),
		PackBytes: 1000, // 1000 + 1000 = 2000 == cap: allowed
		RepoPath:  repoPath,
	}

	_, ok := v.CheckPlaygroundCaps(context.Background(), in)
	if !ok {
		t.Error("push exactly at cap should be allowed (cap is the maximum, not exclusive)")
	}
}

// TestCheckPlaygroundCaps_Exceeded verifies that a push that would push the
// total above the cap is rejected with playground.size_exceeded.
func TestCheckPlaygroundCaps_Exceeded(t *testing.T) {
	repoPath := t.TempDir()
	// Existing repo has 1500 bytes.
	writeRepoFile(t, repoPath, "objects/pack/pack-abc.pack", 1500)

	v := &Validator{PlaygroundMaxContentBytes: 2000}

	in := ValidateInput{
		Session:   makePlaygroundSession("sess-pg-4"),
		Account:   makeAccount("acc-1"),
		PackBytes: 600, // 1500 + 600 = 2100 > 2000: rejected
		RepoPath:  repoPath,
	}

	r, ok := v.CheckPlaygroundCaps(context.Background(), in)
	if ok {
		t.Error("push exceeding cap should be rejected")
	}
	if r.Code != CodePlaygroundSizeExceeded {
		t.Errorf("rejection code: want %q, got %q", CodePlaygroundSizeExceeded, r.Code)
	}
	if r.Message == "" {
		t.Error("rejection message should be non-empty")
	}
}

// TestCheckPlaygroundCaps_EmptyRepo verifies that a playground push into an
// empty or non-existent repo (first push) is correctly handled. The repo-size
// walk returns 0, so only the pack size is compared against the cap.
func TestCheckPlaygroundCaps_EmptyRepo(t *testing.T) {
	v := &Validator{PlaygroundMaxContentBytes: 5000}

	in := ValidateInput{
		Session:   makePlaygroundSession("sess-pg-5"),
		Account:   makeAccount("acc-1"),
		PackBytes: 1000,
		RepoPath:  "", // empty repo path → treat current size as 0
	}

	_, ok := v.CheckPlaygroundCaps(context.Background(), in)
	if !ok {
		t.Error("push into a fresh playground session (no repo yet) should be allowed")
	}
}

// TestCheckPlaygroundCaps_EmptyRepo_ExceedsCap verifies that even on a fresh
// playground session, an oversized first push is rejected.
func TestCheckPlaygroundCaps_EmptyRepo_ExceedsCap(t *testing.T) {
	v := &Validator{PlaygroundMaxContentBytes: 500}

	in := ValidateInput{
		Session:   makePlaygroundSession("sess-pg-6"),
		Account:   makeAccount("acc-1"),
		PackBytes: 600, // 0 + 600 > 500: rejected
		RepoPath:  "",
	}

	r, ok := v.CheckPlaygroundCaps(context.Background(), in)
	if ok {
		t.Error("oversized first push should be rejected even when repo is empty")
	}
	if r.Code != CodePlaygroundSizeExceeded {
		t.Errorf("rejection code: want %q, got %q", CodePlaygroundSizeExceeded, r.Code)
	}
}

// TestCheckPlaygroundCaps_NonExistentRepoPath verifies that a non-existent
// repo path (WalkDir returns an error) is treated as 0 bytes rather than
// failing the push.
func TestCheckPlaygroundCaps_NonExistentRepoPath(t *testing.T) {
	v := &Validator{PlaygroundMaxContentBytes: 5000}

	in := ValidateInput{
		Session:   makePlaygroundSession("sess-pg-7"),
		Account:   makeAccount("acc-1"),
		PackBytes: 1000,
		RepoPath:  "/does/not/exist/session.git",
	}

	_, ok := v.CheckPlaygroundCaps(context.Background(), in)
	if !ok {
		t.Error("non-existent repo path should be treated as 0 bytes (first push allowed)")
	}
}

// ---------------------------------------------------------------------------
// Integration with Validator.Validate
// ---------------------------------------------------------------------------

// TestValidate_PlaygroundSizeExceeded verifies that an oversized push to a
// playground session is rejected through the full Validate path. This ensures
// CheckPlaygroundCaps is wired as the last check in Validate.
func TestValidate_PlaygroundSizeExceeded(t *testing.T) {
	repo, dir := initTestRepo(t)

	// Write a "large" existing repo under a temp path.
	repoPath := t.TempDir()
	writeRepoFile(t, repoPath, "objects/pack/pack-large.pack", 900)

	c := makeCommit(t, repo, dir, nil,
		map[string]string{"main.go": "package main"},
		goodMsg("sess-pg-8", "1", "alice"),
	)

	v := &Validator{
		MaxPackBytes:              52428800,
		PlaygroundMaxContentBytes: 1000, // cap at 1000 bytes
	}

	result, err := v.Validate(context.Background(), ValidateInput{
		Repo:    repo,
		Session: &store.Session{ID: "sess-pg-8", OrgID: reservedPlaygroundOrgID, WritableScope: `["**"]`},
		Account: makeAccount("acc-alice"),
		Updates: []RefUpdate{{
			Ref:    "refs/heads/jam/sess-pg-8/acc-alice/main",
			OldSHA: "",
			NewSHA: c.Hash.String(),
		}},
		PackBytes: 200, // 900 + 200 = 1100 > 1000: rejected
		RepoPath:  repoPath,
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.OK {
		t.Error("expected OK=false due to playground size cap")
	}
	if !hasCode(result.Rejections, CodePlaygroundSizeExceeded) {
		t.Errorf("expected %q rejection, got %v", CodePlaygroundSizeExceeded, result.Rejections)
	}
}

// TestValidate_DurableSessionUnaffectedByPlaygroundCap verifies that the
// playground size-cap check does NOT fire for non-playground sessions even
// when PlaygroundMaxContentBytes is set.
func TestValidate_DurableSessionUnaffectedByPlaygroundCap(t *testing.T) {
	repo, dir := initTestRepo(t)

	c := makeCommit(t, repo, dir, nil,
		map[string]string{"src/main.go": "package main"},
		goodMsg("sess-durable-1", "1", "alice"),
	)

	v := &Validator{
		MaxPackBytes:              52428800,
		PlaygroundMaxContentBytes: 1, // absurdly small cap that would reject everything
	}

	result, err := v.Validate(context.Background(), ValidateInput{
		Repo:    repo,
		Session: makeDurableSession("sess-durable-1", "org-mycompany"),
		Account: makeAccount("acc-alice"),
		Updates: []RefUpdate{{
			Ref:    "refs/heads/jam/sess-durable-1/acc-alice/main",
			OldSHA: "",
			NewSHA: c.Hash.String(),
		}},
		PackBytes: 100,
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.OK {
		t.Errorf("durable session should pass even with a tiny playground cap; got rejections: %v", result.Rejections)
	}
}
