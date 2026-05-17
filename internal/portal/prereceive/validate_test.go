package prereceive

import (
	"context"
	"testing"

	"jamsesh/internal/db/store"
)

// makeSession returns a minimal *store.Session with the given id and
// writable_scope JSON.
func makeSession(id, writableScope string) *store.Session {
	return &store.Session{
		ID:            id,
		WritableScope: writableScope,
	}
}

// makeAccount returns a minimal *store.Account with the given id.
func makeAccount(id string) *store.Account {
	return &store.Account{ID: id}
}

// TestValidate_AllGreen verifies that a well-formed push with valid namespace,
// no force-push, good trailers, and in-scope paths is accepted.
func TestValidate_AllGreen(t *testing.T) {
	repo, dir := initTestRepo(t)
	scope := `["**"]` // allow everything

	c := makeCommit(t, repo, dir, nil,
		map[string]string{"src/main.go": "package main"},
		goodMsg("sess-1", "1", "alice"),
	)

	v := &Validator{MaxPackBytes: 52428800}
	result, err := v.Validate(context.Background(), ValidateInput{
		Repo:      repo,
		Session:   makeSession("sess-1", scope),
		Account:   makeAccount("acc-alice"),
		Updates: []RefUpdate{{
			Ref:    "refs/heads/jam/sess-1/acc-alice/main",
			OldSHA: "",
			NewSHA: c.Hash.String(),
		}},
		PackBytes: 1024,
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.OK {
		t.Errorf("expected OK=true, got rejections: %v", result.Rejections)
	}
}

// TestValidate_PackSizeExceeded verifies that an oversized pack is rejected.
func TestValidate_PackSizeExceeded(t *testing.T) {
	repo, dir := initTestRepo(t)

	c := makeCommit(t, repo, dir, nil,
		map[string]string{"a.txt": "1"},
		goodMsg("sess-1", "1", "alice"),
	)

	v := &Validator{MaxPackBytes: 100} // very small limit
	result, err := v.Validate(context.Background(), ValidateInput{
		Repo:      repo,
		Session:   makeSession("sess-1", `["**"]`),
		Account:   makeAccount("acc-alice"),
		Updates: []RefUpdate{{
			Ref:    "refs/heads/jam/sess-1/acc-alice/main",
			OldSHA: "",
			NewSHA: c.Hash.String(),
		}},
		PackBytes: 200, // exceeds 100-byte limit
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.OK {
		t.Error("expected OK=false due to pack size")
	}
	if !hasCode(result.Rejections, CodeSizeLimit) {
		t.Errorf("expected %q rejection, got %v", CodeSizeLimit, result.Rejections)
	}
}

// TestValidate_RefViolation verifies that a wrong-namespace ref causes a
// ref_namespace_violation rejection.
func TestValidate_RefViolation(t *testing.T) {
	repo, dir := initTestRepo(t)

	c := makeCommit(t, repo, dir, nil,
		map[string]string{"a.txt": "1"},
		goodMsg("sess-1", "1", "alice"),
	)

	v := &Validator{MaxPackBytes: 52428800}
	result, err := v.Validate(context.Background(), ValidateInput{
		Repo:    repo,
		Session: makeSession("sess-1", `["**"]`),
		Account: makeAccount("acc-alice"),
		Updates: []RefUpdate{{
			Ref:    "refs/heads/jam/sess-1/acc-bob/main", // wrong owner
			OldSHA: "",
			NewSHA: c.Hash.String(),
		}},
		PackBytes: 100,
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.OK {
		t.Error("expected OK=false for namespace violation")
	}
	if !hasCode(result.Rejections, CodeRefNamespaceViolation) {
		t.Errorf("expected %q rejection, got %v", CodeRefNamespaceViolation, result.Rejections)
	}
}

// TestValidate_MissingTrailer verifies that a commit missing required trailers
// propagates the rejection through the top-level Validate.
func TestValidate_MissingTrailer(t *testing.T) {
	repo, dir := initTestRepo(t)

	c := makeCommit(t, repo, dir, nil,
		map[string]string{"a.txt": "1"},
		"no trailers here", // missing all required trailers
	)

	v := &Validator{MaxPackBytes: 52428800}
	result, err := v.Validate(context.Background(), ValidateInput{
		Repo:    repo,
		Session: makeSession("sess-1", `["**"]`),
		Account: makeAccount("acc-alice"),
		Updates: []RefUpdate{{
			Ref:    "refs/heads/jam/sess-1/acc-alice/main",
			OldSHA: "",
			NewSHA: c.Hash.String(),
		}},
		PackBytes: 100,
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.OK {
		t.Error("expected OK=false for missing trailers")
	}
	if !hasCode(result.Rejections, CodeMissingTrailer) {
		t.Errorf("expected %q rejection, got %v", CodeMissingTrailer, result.Rejections)
	}
}

// TestValidate_AggregatesRejections verifies that rejections from multiple
// updates are all collected rather than halting at the first failure.
func TestValidate_AggregatesRejections(t *testing.T) {
	repo, dir := initTestRepo(t)

	c1 := makeCommit(t, repo, dir, nil,
		map[string]string{"a.txt": "1"},
		"no trailers", // missing trailers
	)
	c2 := makeCommit(t, repo, dir, c1,
		map[string]string{"b.txt": "2"},
		"still no trailers", // missing trailers
	)

	v := &Validator{MaxPackBytes: 52428800}
	result, err := v.Validate(context.Background(), ValidateInput{
		Repo:    repo,
		Session: makeSession("sess-1", `["**"]`),
		Account: makeAccount("acc-alice"),
		Updates: []RefUpdate{
			{Ref: "refs/heads/jam/sess-1/acc-alice/feature-a", OldSHA: "", NewSHA: c1.Hash.String()},
			{Ref: "refs/heads/jam/sess-1/acc-alice/feature-b", OldSHA: c1.Hash.String(), NewSHA: c2.Hash.String()},
		},
		PackBytes: 100,
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.OK {
		t.Error("expected OK=false")
	}
	// Both updates have bad commits — expect at least 2 rejections.
	if len(result.Rejections) < 2 {
		t.Errorf("expected at least 2 rejections, got %d: %v", len(result.Rejections), result.Rejections)
	}
}

// TestValidate_ScopeViolation verifies that a commit modifying files outside
// the writable scope is rejected with push.scope_violation.
func TestValidate_ScopeViolation(t *testing.T) {
	repo, dir := initTestRepo(t)

	// Commit changes src/main.go but scope only allows docs/**
	c := makeCommit(t, repo, dir, nil,
		map[string]string{"src/main.go": "package main"},
		goodMsg("sess-1", "1", "alice"),
	)

	v := &Validator{MaxPackBytes: 52428800}
	result, err := v.Validate(context.Background(), ValidateInput{
		Repo:    repo,
		Session: makeSession("sess-1", `["docs/**"]`), // only docs allowed
		Account: makeAccount("acc-alice"),
		Updates: []RefUpdate{{
			Ref:    "refs/heads/jam/sess-1/acc-alice/main",
			OldSHA: "",
			NewSHA: c.Hash.String(),
		}},
		PackBytes: 100,
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.OK {
		t.Error("expected OK=false for out-of-scope path")
	}
	if !hasCode(result.Rejections, CodeScopeViolation) {
		t.Errorf("expected %q rejection, got %v", CodeScopeViolation, result.Rejections)
	}
}

// TestValidate_InvalidWritableScopeJSON verifies that a malformed
// writable_scope JSON returns an error (not a rejection).
func TestValidate_InvalidWritableScopeJSON(t *testing.T) {
	repo, dir := initTestRepo(t)

	c := makeCommit(t, repo, dir, nil,
		map[string]string{"a.txt": "1"},
		goodMsg("sess-1", "1", "alice"),
	)

	v := &Validator{MaxPackBytes: 52428800}
	_, err := v.Validate(context.Background(), ValidateInput{
		Repo:    repo,
		Session: makeSession("sess-1", `not-valid-json`),
		Account: makeAccount("acc-alice"),
		Updates: []RefUpdate{{
			Ref:    "refs/heads/jam/sess-1/acc-alice/main",
			OldSHA: "",
			NewSHA: c.Hash.String(),
		}},
		PackBytes: 100,
	})

	if err == nil {
		t.Error("expected error for invalid writable_scope JSON, got nil")
	}
}

// hasCode reports whether any rejection in the list has the given code.
func hasCode(rejections []Rejection, code string) bool {
	for _, r := range rejections {
		if r.Code == code {
			return true
		}
	}
	return false
}
