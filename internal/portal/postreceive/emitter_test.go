package postreceive_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	_ "modernc.org/sqlite"

	"jamsesh/internal/api/openapi"
	"jamsesh/internal/db"
	"jamsesh/internal/db/store"
	"jamsesh/internal/portal/events"
	"jamsesh/internal/portal/postreceive"
)

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

var dbCounter atomic.Int64

// openStore opens a fresh in-memory SQLite store with all migrations applied.
func openStore(t *testing.T) store.Store {
	t.Helper()
	n := dbCounter.Add(1)
	dsn := fmt.Sprintf("file:postreceive_test_%d?mode=memory&cache=shared", n)
	s, err := db.Open(context.Background(), "sqlite", dsn)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	type rawDBer interface{ RawDB() *sql.DB }
	if r, ok := s.(rawDBer); ok {
		r.RawDB().SetMaxOpenConns(1)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

// mustSetupSession creates an org, account, and session row and returns them.
func mustSetupSession(t *testing.T, ctx context.Context, s store.Store) (*store.Session, *store.Account) {
	t.Helper()
	n := dbCounter.Add(1)
	orgID := fmt.Sprintf("org-%04d", n)
	accountID := fmt.Sprintf("acc-%04d", n)
	sessionID := fmt.Sprintf("sess-%04d", n)
	now := time.Now().UTC()

	_, err := s.CreateOrg(ctx, store.CreateOrgParams{
		ID: orgID, Name: "Org " + orgID, Slug: orgID, CreatedAt: now,
	})
	if err != nil {
		t.Fatalf("CreateOrg: %v", err)
	}
	acc, err := s.CreateAccount(ctx, store.CreateAccountParams{
		ID: accountID, Email: accountID + "@example.com", DisplayName: "Test", CreatedAt: now,
	})
	if err != nil {
		t.Fatalf("CreateAccount: %v", err)
	}
	sess, err := s.CreateSession(ctx, store.CreateSessionParams{
		ID:            sessionID,
		OrgID:         orgID,
		Name:          "Test Session",
		Goal:          "goal",
		WritableScope: `["**"]`,
		DefaultMode:   "sync",
		Status:        "active",
		CreatedAt:     now,
	})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	return &sess, &acc
}

// initTestRepo creates a fresh non-bare git repo in a temp directory.
func initTestRepo(t *testing.T) (*git.Repository, string) {
	t.Helper()
	dir := t.TempDir()
	runCmd(t, dir, "git", "init", "-b", "main")
	runCmd(t, dir, "git", "config", "user.email", "test@jamsesh.test")
	runCmd(t, dir, "git", "config", "user.name", "Test")
	repo, err := git.PlainOpen(dir)
	if err != nil {
		t.Fatalf("PlainOpen: %v", err)
	}
	return repo, dir
}

// makeCommit creates a commit with the given files and message.
// parent may be nil for the root commit.
func makeCommit(t *testing.T, repo *git.Repository, dir string, parent *object.Commit, files map[string]string, message string) *object.Commit {
	t.Helper()
	for name, content := range files {
		full := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(full, []byte(content), 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	runCmd(t, dir, "git", "add", "-A")

	sig := object.Signature{Name: "Test", Email: "test@jamsesh.test", When: time.Now()}
	opts := &git.CommitOptions{Author: &sig, Committer: &sig}
	if parent != nil {
		opts.Parents = []plumbing.Hash{parent.Hash}
	}

	wt, err := repo.Worktree()
	if err != nil {
		t.Fatalf("Worktree: %v", err)
	}
	hash, err := wt.Commit(message, opts)
	if err != nil {
		opts2 := &git.CommitOptions{
			Author: &sig, Committer: &sig, AllowEmptyCommits: true,
		}
		if parent != nil {
			opts2.Parents = []plumbing.Hash{parent.Hash}
		}
		hash, err = wt.Commit(message, opts2)
		if err != nil {
			t.Fatalf("commit %q: %v", message, err)
		}
	}
	c, err := repo.CommitObject(hash)
	if err != nil {
		t.Fatalf("CommitObject: %v", err)
	}
	return c
}

// runCmd runs a command in dir and fails the test on error.
func runCmd(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("run %v: %v\n%s", args, err, out)
	}
}

// msgWithAuthor returns a commit message with a Jam-Author trailer.
func msgWithAuthor(subject, authorID string) string {
	return subject + "\n\nJam-Session: sess-1\nJam-Turn: 1\nJam-Author: " + authorID
}

// decodePayload unmarshals the event payload into CommitArrivedPayload.
func decodePayload(t *testing.T, raw json.RawMessage) openapi.CommitArrivedPayload {
	t.Helper()
	var p openapi.CommitArrivedPayload
	if err := json.Unmarshal(raw, &p); err != nil {
		t.Fatalf("unmarshal CommitArrivedPayload: %v", err)
	}
	return p
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

// TestEmitForUpdates_ThreeCommitChain verifies that a 3-commit chain emits
// exactly 3 events with correct seqs, types, and payload fields.
func TestEmitForUpdates_ThreeCommitChain(t *testing.T) {
	ctx := context.Background()
	s := openStore(t)
	sess, acc := mustSetupSession(t, ctx, s)
	log := events.New(s)
	emitter := &postreceive.Emitter{Log: log}

	repo, dir := initTestRepo(t)
	ref := "refs/heads/jam/sess-1/alice/main"

	c1 := makeCommit(t, repo, dir, nil, map[string]string{"a.txt": "1"},
		msgWithAuthor("first commit", "alice@example.com"))
	c2 := makeCommit(t, repo, dir, c1, map[string]string{"b.txt": "2"},
		msgWithAuthor("second commit", "alice@example.com"))
	c3 := makeCommit(t, repo, dir, c2, map[string]string{"c.txt": "3"},
		msgWithAuthor("third commit", "alice@example.com"))

	update := postreceive.RefUpdate{
		Ref:    ref,
		OldSHA: c1.Hash.String(), // c2 and c3 are new
		NewSHA: c3.Hash.String(),
	}
	err := emitter.EmitForUpdates(ctx, repo, sess, acc, []postreceive.RefUpdate{update})
	if err != nil {
		t.Fatalf("EmitForUpdates: %v", err)
	}

	evs, err := log.ListSince(ctx, sess.ID, 0, 100)
	if err != nil {
		t.Fatalf("ListSince: %v", err)
	}
	if len(evs) != 2 {
		t.Fatalf("want 2 events (c2+c3), got %d", len(evs))
	}

	// Contiguous seqs starting from 1.
	for i, e := range evs {
		if e.Seq != int64(i+1) {
			t.Errorf("evs[%d].Seq: want %d, got %d", i, i+1, e.Seq)
		}
		if e.Type != "commit.arrived" {
			t.Errorf("evs[%d].Type: want commit.arrived, got %s", i, e.Type)
		}
	}

	// Oldest first: evs[0] = c2, evs[1] = c3.
	p0 := decodePayload(t, evs[0].Payload)
	if p0.Sha != c2.Hash.String() {
		t.Errorf("evs[0].Sha: want %s, got %s", c2.Hash.String(), p0.Sha)
	}
	if p0.Ref != ref {
		t.Errorf("evs[0].Ref: want %s, got %s", ref, p0.Ref)
	}
	if p0.Summary != "second commit" {
		t.Errorf("evs[0].Summary: want 'second commit', got %q", p0.Summary)
	}
	if p0.AuthorId != "alice@example.com" {
		t.Errorf("evs[0].AuthorId: want alice@example.com, got %s", p0.AuthorId)
	}

	p1 := decodePayload(t, evs[1].Payload)
	if p1.Sha != c3.Hash.String() {
		t.Errorf("evs[1].Sha: want %s, got %s", c3.Hash.String(), p1.Sha)
	}
}

// TestEmitForUpdates_EmptyRange verifies that OldSHA == NewSHA emits zero events.
func TestEmitForUpdates_EmptyRange(t *testing.T) {
	ctx := context.Background()
	s := openStore(t)
	sess, acc := mustSetupSession(t, ctx, s)
	log := events.New(s)
	emitter := &postreceive.Emitter{Log: log}

	repo, dir := initTestRepo(t)
	c1 := makeCommit(t, repo, dir, nil, map[string]string{"a.txt": "1"},
		msgWithAuthor("root", "alice@example.com"))

	update := postreceive.RefUpdate{
		Ref:    "refs/heads/main",
		OldSHA: c1.Hash.String(),
		NewSHA: c1.Hash.String(),
	}
	err := emitter.EmitForUpdates(ctx, repo, sess, acc, []postreceive.RefUpdate{update})
	if err != nil {
		t.Fatalf("EmitForUpdates: %v", err)
	}

	evs, _ := log.ListSince(ctx, sess.ID, 0, 100)
	if len(evs) != 0 {
		t.Errorf("empty range: want 0 events, got %d", len(evs))
	}
}

// TestEmitForUpdates_NewRef verifies that OldSHA=="" emits events for all
// reachable commits (new ref creation).
func TestEmitForUpdates_NewRef(t *testing.T) {
	ctx := context.Background()
	s := openStore(t)
	sess, acc := mustSetupSession(t, ctx, s)
	log := events.New(s)
	emitter := &postreceive.Emitter{Log: log}

	repo, dir := initTestRepo(t)
	c1 := makeCommit(t, repo, dir, nil, map[string]string{"a.txt": "1"},
		msgWithAuthor("root", "bob@example.com"))
	c2 := makeCommit(t, repo, dir, c1, map[string]string{"b.txt": "2"},
		msgWithAuthor("second", "bob@example.com"))
	c3 := makeCommit(t, repo, dir, c2, map[string]string{"c.txt": "3"},
		msgWithAuthor("third", "bob@example.com"))

	update := postreceive.RefUpdate{
		Ref:    "refs/heads/main",
		OldSHA: "",
		NewSHA: c3.Hash.String(),
	}
	err := emitter.EmitForUpdates(ctx, repo, sess, acc, []postreceive.RefUpdate{update})
	if err != nil {
		t.Fatalf("EmitForUpdates: %v", err)
	}

	evs, err := log.ListSince(ctx, sess.ID, 0, 100)
	if err != nil {
		t.Fatalf("ListSince: %v", err)
	}
	// All 3 commits are new (new ref).
	if len(evs) != 3 {
		t.Fatalf("new ref: want 3 events, got %d", len(evs))
	}
	// Oldest first: evs[0] = c1.
	p := decodePayload(t, evs[0].Payload)
	if p.Sha != c1.Hash.String() {
		t.Errorf("evs[0].Sha: want %s (oldest), got %s", c1.Hash.String(), p.Sha)
	}
}

// TestEmitForUpdates_JamAuthorTrailer verifies that Jam-Author trailer takes
// precedence over commit.Author.Email.
func TestEmitForUpdates_JamAuthorTrailer(t *testing.T) {
	ctx := context.Background()
	s := openStore(t)
	sess, acc := mustSetupSession(t, ctx, s)
	log := events.New(s)
	emitter := &postreceive.Emitter{Log: log}

	repo, dir := initTestRepo(t)
	// Message has Jam-Author set to a different ID than the git author email.
	c1 := makeCommit(t, repo, dir, nil, map[string]string{"a.txt": "1"},
		"feat: do thing\n\nJam-Session: sess-1\nJam-Turn: 1\nJam-Author: account-xyz-123")

	update := postreceive.RefUpdate{
		Ref:    "refs/heads/main",
		OldSHA: "",
		NewSHA: c1.Hash.String(),
	}
	err := emitter.EmitForUpdates(ctx, repo, sess, acc, []postreceive.RefUpdate{update})
	if err != nil {
		t.Fatalf("EmitForUpdates: %v", err)
	}

	evs, _ := log.ListSince(ctx, sess.ID, 0, 100)
	if len(evs) != 1 {
		t.Fatalf("want 1 event, got %d", len(evs))
	}
	p := decodePayload(t, evs[0].Payload)
	if p.AuthorId != "account-xyz-123" {
		t.Errorf("AuthorId: want account-xyz-123 (trailer), got %s", p.AuthorId)
	}
}

// TestEmitForUpdates_NoJamAuthorFallback verifies fallback to commit.Author.Email
// when the Jam-Author trailer is absent.
func TestEmitForUpdates_NoJamAuthorFallback(t *testing.T) {
	ctx := context.Background()
	s := openStore(t)
	sess, acc := mustSetupSession(t, ctx, s)
	log := events.New(s)
	emitter := &postreceive.Emitter{Log: log}

	repo, dir := initTestRepo(t)
	// Plain message with no trailers.
	c1 := makeCommit(t, repo, dir, nil, map[string]string{"a.txt": "1"},
		"fix: no trailers here")

	update := postreceive.RefUpdate{
		Ref:    "refs/heads/main",
		OldSHA: "",
		NewSHA: c1.Hash.String(),
	}
	err := emitter.EmitForUpdates(ctx, repo, sess, acc, []postreceive.RefUpdate{update})
	if err != nil {
		t.Fatalf("EmitForUpdates: %v", err)
	}

	evs, _ := log.ListSince(ctx, sess.ID, 0, 100)
	if len(evs) != 1 {
		t.Fatalf("want 1 event, got %d", len(evs))
	}
	p := decodePayload(t, evs[0].Payload)
	// Falls back to git author email set in initTestRepo ("test@jamsesh.test").
	if p.AuthorId != "test@jamsesh.test" {
		t.Errorf("AuthorId fallback: want test@jamsesh.test, got %s", p.AuthorId)
	}
}

// TestEmitForUpdates_EmitBatchErrorPropagates verifies that errors from
// EmitBatch propagate to the caller. We simulate this by closing the store
// before emitting.
func TestEmitForUpdates_EmitBatchErrorPropagates(t *testing.T) {
	ctx := context.Background()
	s := openStore(t)
	sess, acc := mustSetupSession(t, ctx, s)

	// Close the store now so that EmitBatch will fail.
	_ = s.Close()

	log := events.New(s)
	emitter := &postreceive.Emitter{Log: log}

	repo, dir := initTestRepo(t)
	c1 := makeCommit(t, repo, dir, nil, map[string]string{"a.txt": "1"},
		msgWithAuthor("root", "alice@example.com"))

	update := postreceive.RefUpdate{
		Ref:    "refs/heads/main",
		OldSHA: "",
		NewSHA: c1.Hash.String(),
	}
	err := emitter.EmitForUpdates(ctx, repo, sess, acc, []postreceive.RefUpdate{update})
	if err == nil {
		t.Error("expected error from EmitBatch after store close, got nil")
	}
}
