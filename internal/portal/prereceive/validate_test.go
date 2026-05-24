package prereceive

import (
	"context"
	"testing"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"

	"jamsesh/internal/db/store"
)

// dropRefs removes all concrete refs under refs/heads/ from the test repo so
// that the base-ref empty-repo check (refs.go:107) sees an empty repository.
// Production's validation repo is built from a fresh bare repo + the pushed
// pack layered in memory; no concrete refs exist when pre-receive validates
// a first push. The unit test repo, by contrast, advances refs/heads/main
// each time makeCommit lands a commit, so this helper restores the
// production assumption.
func dropRefs(t *testing.T, repo *git.Repository) {
	t.Helper()
	iter, err := repo.References()
	if err != nil {
		t.Fatalf("dropRefs: References: %v", err)
	}
	var concrete []plumbing.ReferenceName
	_ = iter.ForEach(func(r *plumbing.Reference) error {
		if r.Type() != plumbing.SymbolicReference {
			concrete = append(concrete, r.Name())
		}
		return nil
	})
	iter.Close()
	for _, n := range concrete {
		if err := repo.Storer.RemoveReference(n); err != nil {
			t.Fatalf("dropRefs: RemoveReference %s: %v", n, err)
		}
	}
}

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

// TestValidate_BaseRefFirstPush_ExemptFromTrailerValidation verifies the
// bootstrap exemption: the first push to refs/heads/jam/<session>/base
// (OldSHA="") carries the user's pre-session commits, which by definition
// cannot have Jam-Session trailers naming the session that didn't exist when
// they were authored. The base-ref first push exists to seed the session
// repo; trailer enforcement starts with subsequent collaborative pushes.
//
// Regression for bug-playground-git-receive-pack-fails-with-200-hangup:
// `jamsesh new --playground` from a vanilla repo (no plugin commit-msg hook
// installed) was blocked because pre-receive rejected the seed commit for
// missing trailers, producing a 200 + report-status-rejection response that
// git surfaced as "fatal: the remote end hung up unexpectedly".
func TestValidate_BaseRefFirstPush_ExemptFromTrailerValidation(t *testing.T) {
	repo, dir := initTestRepo(t)

	// Vanilla "initial commit" — no Jam-Session, Jam-Turn, or Jam-Author
	// trailers. This is exactly what `git commit -m 'initial commit'`
	// produces in a freshly-init'd repo before the user has any session.
	c := makeCommit(t, repo, dir, nil,
		map[string]string{"README.md": "playground seed\n"},
		"initial commit", // no trailers
	)
	// Production parity: the validation repo built by buildValidationRepo
	// has no concrete refs when pre-receive runs (refs are only updated
	// AFTER acceptance). makeCommit leaves refs/heads/main behind, which
	// would make repoIsEmpty return false and trigger the base-ref
	// namespace rejection. Drop the ref to match the real scenario.
	dropRefs(t, repo)

	v := &Validator{MaxPackBytes: 52428800}
	result, err := v.Validate(context.Background(), ValidateInput{
		Repo:    repo,
		Session: makeSession("sess-1", `["**"]`),
		Account: makeAccount("acc-anon"),
		Updates: []RefUpdate{{
			Ref:    "refs/heads/jam/sess-1/base", // base-ref special case
			OldSHA: "",                           // first push, empty old SHA
			NewSHA: c.Hash.String(),
		}},
		PackBytes: 100,
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.OK {
		t.Errorf("expected OK=true for base-ref first push with untrailered seed commit; got rejections: %v", result.Rejections)
	}
}

// TestValidate_BaseRefSecondPush_StillValidated verifies the exemption is
// narrow — it only applies to the FIRST push (OldSHA=""). A subsequent push
// to the base ref (rebase, force-push) still goes through full trailer
// validation. (In practice base-ref force-push is also blocked by the
// force-push check, but trailer validation should still apply.)
func TestValidate_BaseRefSecondPush_StillValidated(t *testing.T) {
	repo, dir := initTestRepo(t)

	// Two-commit chain: first commit has trailers (the existing base),
	// second commit doesn't.
	c1 := makeCommit(t, repo, dir, nil,
		map[string]string{"a.txt": "1"},
		goodMsg("sess-1", "1", "alice"),
	)
	c2 := makeCommit(t, repo, dir, c1,
		map[string]string{"b.txt": "2"},
		"no trailers on update", // missing trailers
	)

	v := &Validator{MaxPackBytes: 52428800}
	result, err := v.Validate(context.Background(), ValidateInput{
		Repo:    repo,
		Session: makeSession("sess-1", `["**"]`),
		Account: makeAccount("acc-alice"),
		Updates: []RefUpdate{{
			Ref:    "refs/heads/jam/sess-1/base",
			OldSHA: c1.Hash.String(), // NOT the first push
			NewSHA: c2.Hash.String(),
		}},
		PackBytes: 100,
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.OK {
		t.Error("expected OK=false for non-first base-ref push with missing trailers; exemption must be scoped to first push only")
	}
	if !hasCode(result.Rejections, CodeMissingTrailer) {
		t.Errorf("expected %q rejection, got %v", CodeMissingTrailer, result.Rejections)
	}
}

// TestValidate_NonBaseRef_TrailerStillRequired_RegressionGuard is a control
// test proving the base-ref exemption doesn't leak to other refs. A normal
// user ref push with missing trailers must still reject — otherwise the
// exemption is too broad and trailer enforcement is silently disabled for
// collaborative work.
func TestValidate_NonBaseRef_TrailerStillRequired_RegressionGuard(t *testing.T) {
	repo, dir := initTestRepo(t)

	c := makeCommit(t, repo, dir, nil,
		map[string]string{"src/main.go": "package main"},
		"no trailers on user branch", // missing trailers
	)

	v := &Validator{MaxPackBytes: 52428800}
	result, err := v.Validate(context.Background(), ValidateInput{
		Repo:    repo,
		Session: makeSession("sess-1", `["**"]`),
		Account: makeAccount("acc-alice"),
		Updates: []RefUpdate{{
			Ref:    "refs/heads/jam/sess-1/acc-alice/main", // user ref, not base
			OldSHA: "",
			NewSHA: c.Hash.String(),
		}},
		PackBytes: 100,
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.OK {
		t.Error("expected OK=false for user-ref push with missing trailers; trailer enforcement must apply outside the base-ref exemption")
	}
	if !hasCode(result.Rejections, CodeMissingTrailer) {
		t.Errorf("expected %q rejection, got %v", CodeMissingTrailer, result.Rejections)
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
