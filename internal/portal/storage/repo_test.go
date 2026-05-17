package storage_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"jamsesh/internal/portal/storage"
)

// requireGit skips the test if git is not found on PATH.
func requireGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found on PATH; skipping git-dependent tests")
	}
}

// newService creates a Service rooted at a fresh temporary directory.
// store.Store is nil — it is only used by the archive-and-stub story methods.
func newService(t *testing.T) (storage.Service, string) {
	t.Helper()
	root := t.TempDir()
	svc := storage.New(root, nil)
	return svc, root
}

// ---------------------------------------------------------------------------
// Path resolution
// ---------------------------------------------------------------------------

func TestRepoPath(t *testing.T) {
	svc, root := newService(t)

	got := svc.RepoPath("my-org", "my-session")
	want := filepath.Join(root, "orgs", "my-org", "sessions", "my-session.git")

	if got != want {
		t.Errorf("RepoPath = %q; want %q", got, want)
	}
}

func TestRepoPath_DifferentIDs(t *testing.T) {
	svc, root := newService(t)

	cases := []struct {
		orgID     string
		sessionID string
	}{
		{"o", "s"},
		{"org-uuid-123", "sess-uuid-456"},
		{"01HX5Y6Z7A", "01HX5Y6Z7B"},
	}

	for _, tc := range cases {
		got := svc.RepoPath(tc.orgID, tc.sessionID)
		want := filepath.Join(root, "orgs", tc.orgID, "sessions", tc.sessionID+".git")
		if got != want {
			t.Errorf("RepoPath(%q, %q) = %q; want %q", tc.orgID, tc.sessionID, got, want)
		}
	}
}

// ---------------------------------------------------------------------------
// RepoExists — before any repo creation
// ---------------------------------------------------------------------------

func TestRepoExists_MissingReturnsFalse(t *testing.T) {
	svc, _ := newService(t)

	exists, err := svc.RepoExists("org1", "session1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if exists {
		t.Error("RepoExists returned true for non-existent repo")
	}
}

// ---------------------------------------------------------------------------
// CreateRepo
// ---------------------------------------------------------------------------

func TestCreateRepo_CreatesValidBareRepo(t *testing.T) {
	requireGit(t)
	svc, _ := newService(t)
	ctx := context.Background()

	if err := svc.CreateRepo(ctx, "org1", "sess1"); err != nil {
		t.Fatalf("CreateRepo: %v", err)
	}

	p := svc.RepoPath("org1", "sess1")

	// Must be a directory.
	info, err := os.Stat(p)
	if err != nil {
		t.Fatalf("Stat repo path: %v", err)
	}
	if !info.IsDir() {
		t.Fatalf("repo path is not a directory: %s", p)
	}

	// A valid bare repo must contain HEAD, objects/, and refs/.
	for _, entry := range []string{"HEAD", "objects", "refs"} {
		ep := filepath.Join(p, entry)
		if _, err := os.Stat(ep); os.IsNotExist(err) {
			t.Errorf("bare repo missing expected entry: %s", ep)
		}
	}
}

func TestCreateRepo_CreatesParentDirs(t *testing.T) {
	requireGit(t)
	svc, root := newService(t)
	ctx := context.Background()

	// No orgs/ or sessions/ subdirs exist yet.
	if err := svc.CreateRepo(ctx, "new-org", "new-session"); err != nil {
		t.Fatalf("CreateRepo: %v", err)
	}

	expected := filepath.Join(root, "orgs", "new-org", "sessions", "new-session.git")
	if _, err := os.Stat(expected); err != nil {
		t.Fatalf("expected repo at %s, got error: %v", expected, err)
	}
}

func TestCreateRepo_ExistingRepoReturnsError(t *testing.T) {
	requireGit(t)
	svc, _ := newService(t)
	ctx := context.Background()

	if err := svc.CreateRepo(ctx, "org1", "sess1"); err != nil {
		t.Fatalf("first CreateRepo: %v", err)
	}

	// Second call must return an error — not silently overwrite.
	if err := svc.CreateRepo(ctx, "org1", "sess1"); err == nil {
		t.Fatal("expected error when creating an already-existing repo, got nil")
	}
}

// ---------------------------------------------------------------------------
// RepoExists — after CreateRepo
// ---------------------------------------------------------------------------

func TestRepoExists_TrueAfterCreate(t *testing.T) {
	requireGit(t)
	svc, _ := newService(t)
	ctx := context.Background()

	if err := svc.CreateRepo(ctx, "org2", "sess2"); err != nil {
		t.Fatalf("CreateRepo: %v", err)
	}

	exists, err := svc.RepoExists("org2", "sess2")
	if err != nil {
		t.Fatalf("RepoExists: %v", err)
	}
	if !exists {
		t.Error("RepoExists returned false after CreateRepo")
	}
}

// ---------------------------------------------------------------------------
// RemoveRepo
// ---------------------------------------------------------------------------

func TestRemoveRepo_RemovesRepo(t *testing.T) {
	requireGit(t)
	svc, _ := newService(t)
	ctx := context.Background()

	if err := svc.CreateRepo(ctx, "org3", "sess3"); err != nil {
		t.Fatalf("CreateRepo: %v", err)
	}

	if err := svc.RemoveRepo(ctx, "org3", "sess3"); err != nil {
		t.Fatalf("RemoveRepo: %v", err)
	}

	exists, err := svc.RepoExists("org3", "sess3")
	if err != nil {
		t.Fatalf("RepoExists after remove: %v", err)
	}
	if exists {
		t.Error("RepoExists returned true after RemoveRepo")
	}
}

func TestRemoveRepo_NonExistentIsNoop(t *testing.T) {
	svc, _ := newService(t)
	ctx := context.Background()

	// Removing a repo that never existed must not return an error.
	if err := svc.RemoveRepo(ctx, "org-gone", "sess-gone"); err != nil {
		t.Fatalf("RemoveRepo on non-existent path: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Round-trip: create → exists → remove → not exists
// ---------------------------------------------------------------------------

func TestRoundTrip(t *testing.T) {
	requireGit(t)
	svc, _ := newService(t)
	ctx := context.Background()

	const org, sess = "rt-org", "rt-sess"

	// Not exists before create.
	exists, err := svc.RepoExists(org, sess)
	if err != nil || exists {
		t.Fatalf("pre-create: exists=%v err=%v", exists, err)
	}

	// Create.
	if err := svc.CreateRepo(ctx, org, sess); err != nil {
		t.Fatalf("CreateRepo: %v", err)
	}

	// Exists after create.
	exists, err = svc.RepoExists(org, sess)
	if err != nil || !exists {
		t.Fatalf("post-create: exists=%v err=%v", exists, err)
	}

	// Remove.
	if err := svc.RemoveRepo(ctx, org, sess); err != nil {
		t.Fatalf("RemoveRepo: %v", err)
	}

	// Not exists after remove.
	exists, err = svc.RepoExists(org, sess)
	if err != nil || exists {
		t.Fatalf("post-remove: exists=%v err=%v", exists, err)
	}
}
