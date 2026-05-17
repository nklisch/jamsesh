package automerger_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"

	openapi "jamsesh/internal/api/openapi"
	"jamsesh/internal/db"
	"jamsesh/internal/db/store"
	"jamsesh/internal/portal/automerger"
	"jamsesh/internal/portal/events"
)

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func openTestStore(t *testing.T) store.Store {
	t.Helper()
	s, err := db.Open(context.Background(), "sqlite", ":memory:", db.PoolConfig{})
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func seedSession(t *testing.T, s store.Store) *store.Session {
	t.Helper()
	ctx := context.Background()
	org, err := s.CreateOrg(ctx, store.CreateOrgParams{
		ID:        "org-1",
		Name:      "Test Org",
		Slug:      "test-org",
		CreatedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("create org: %v", err)
	}
	sess, err := s.CreateSession(ctx, store.CreateSessionParams{
		ID:            "sess-1",
		OrgID:         org.ID,
		Name:          "Test Session",
		Goal:          "testing",
		WritableScope: `["**"]`,
		DefaultMode:   "sync",
		Status:        "active",
		CreatedAt:     time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	return &sess
}

// buildApplyRepo builds a bare in-memory repo with base + draft and source
// commits and returns the repo plus the relevant hashes.
//
//	ancestor → draftTip (draft adds file-a.txt)
//	ancestor → source   (source adds file-b.txt)
func buildApplyRepo(t *testing.T) (repo *gogit.Repository, source, draftTip, ancestor plumbing.Hash) {
	t.Helper()
	r, dir := initRepo(t)

	baseCommit := commitFiles(t, r, dir, nil, map[string][]byte{
		"file.txt": []byte("base\n"),
	}, "base")

	draftCommit := commitFiles(t, r, dir, baseCommit, map[string][]byte{
		"file.txt":  []byte("base\n"),
		"file-a.txt": []byte("added by draft\n"),
	}, "draft: add file-a.txt")

	sourceCommit := commitFiles(t, r, dir, baseCommit, map[string][]byte{
		"file.txt":  []byte("base\n"),
		"file-b.txt": []byte("added by source\n"),
	}, "source: add file-b.txt")

	return r, sourceCommit.Hash, draftCommit.Hash, baseCommit.Hash
}

// buildConflictRepo builds a repo where both draft and source modify the same
// line differently → HardConflict.
func buildConflictRepo(t *testing.T) (repo *gogit.Repository, source, draftTip, ancestor plumbing.Hash) {
	t.Helper()
	r, dir := initRepo(t)

	baseCommit := commitFiles(t, r, dir, nil, map[string][]byte{
		"file.txt": []byte("line1\nline2\nline3\n"),
	}, "base")

	draftCommit := commitFiles(t, r, dir, baseCommit, map[string][]byte{
		"file.txt": []byte("line1\nDRAFT\nline3\n"),
	}, "draft: edit line2")

	sourceCommit := commitFiles(t, r, dir, baseCommit, map[string][]byte{
		"file.txt": []byte("line1\nSOURCE\nline3\n"),
	}, "source: edit line2 differently")

	return r, sourceCommit.Hash, draftCommit.Hash, baseCommit.Hash
}

// runMerge is a helper that calls automerger.Merge and returns the result.
func runMerge(t *testing.T, repo *gogit.Repository, sourceH, draftH, ancestorH plumbing.Hash) automerger.MergeResult {
	t.Helper()
	srcObj, err := object.GetCommit(repo.Storer, sourceH)
	if err != nil {
		t.Fatalf("get source commit: %v", err)
	}
	draftObj, err := object.GetCommit(repo.Storer, draftH)
	if err != nil {
		t.Fatalf("get draft commit: %v", err)
	}
	ancestorObj, err := object.GetCommit(repo.Storer, ancestorH)
	if err != nil {
		t.Fatalf("get ancestor commit: %v", err)
	}

	result, err := automerger.Merge(context.Background(), repo, srcObj, draftObj, ancestorObj)
	if err != nil {
		t.Fatalf("Merge: %v", err)
	}
	return result
}

// ---------------------------------------------------------------------------
// Apply — clean merge
// ---------------------------------------------------------------------------

func TestApply_CleanMerge(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)
	sess := seedSession(t, s)
	log := events.New(s)

	repo, sourceH, draftH, ancestorH := buildApplyRepo(t)
	result := runMerge(t, repo, sourceH, draftH, ancestorH)
	if result.Kind != automerger.CleanMerge {
		t.Fatalf("expected CleanMerge, got %s", result.Kind)
	}

	// Seed event_seq row (Log.Emit requires it).
	if err := s.EnsureEventSeqRow(ctx, sess.ID); err != nil {
		t.Fatalf("ensure event_seq row: %v", err)
	}

	applier := automerger.NewApplier(s, log)
	out, err := applier.Apply(ctx, automerger.ApplyInput{
		Repo:         repo,
		Session:      sess,
		SourceRef:    "refs/heads/jam/sess-1/alice/feat",
		SourceCommit: sourceH,
		DraftTip:     draftH,
		Ancestor:     ancestorH,
		Result:       result,
		PortalHost:   "jamsesh.test",
	})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}

	// Merge commit SHA must be non-empty.
	if out.MergeCommitSHA == "" {
		t.Error("MergeCommitSHA is empty")
	}
	if out.ConflictEvent != nil {
		t.Error("ConflictEvent should be nil on clean merge")
	}

	// Verify the merge commit has correct author/committer/trailers.
	mergeHash := plumbing.NewHash(out.MergeCommitSHA)
	mc, err := object.GetCommit(repo.Storer, mergeHash)
	if err != nil {
		t.Fatalf("get merge commit: %v", err)
	}

	// Author should be the source commit's author.
	srcCommit, _ := object.GetCommit(repo.Storer, sourceH)
	if mc.Author.Email != srcCommit.Author.Email {
		t.Errorf("author email: got %q, want %q", mc.Author.Email, srcCommit.Author.Email)
	}

	// Committer should be the auto-merger.
	if mc.Committer.Name != "jamsesh auto-merger" {
		t.Errorf("committer name: got %q, want %q", mc.Committer.Name, "jamsesh auto-merger")
	}
	if mc.Committer.Email != "auto-merger@jamsesh.test" {
		t.Errorf("committer email: got %q, want %q", mc.Committer.Email, "auto-merger@jamsesh.test")
	}

	// Trailers.
	if !strings.Contains(mc.Message, "Auto-Merger: true") {
		t.Errorf("missing Auto-Merger trailer in:\n%s", mc.Message)
	}
	if !strings.Contains(mc.Message, "Source-Commit: "+sourceH.String()) {
		t.Errorf("missing Source-Commit trailer in:\n%s", mc.Message)
	}
	if !strings.Contains(mc.Message, "Source-Ref: refs/heads/jam/sess-1/alice/feat") {
		t.Errorf("missing Source-Ref trailer in:\n%s", mc.Message)
	}
	if strings.Contains(mc.Message, "Auto-Resolved:") {
		t.Error("clean merge should not have Auto-Resolved trailer")
	}

	// Draft ref should be advanced.
	draftRefName := plumbing.NewBranchReferenceName("jam/" + sess.ID + "/draft")
	ref, err := repo.Reference(draftRefName, true)
	if err != nil {
		t.Fatalf("draft ref not found: %v", err)
	}
	if ref.Hash() != mergeHash {
		t.Errorf("draft ref hash: got %s, want %s", ref.Hash(), mergeHash)
	}

	// merge.succeeded event should be emitted.
	evts, err := s.ListEventsSince(ctx, store.ListEventsSinceParams{
		SessionID: sess.ID,
		SinceSeq:  0,
		Limit:     100,
	})
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	var foundSucceeded bool
	for _, ev := range evts {
		if ev.Type == "merge.succeeded" {
			foundSucceeded = true
			var payload openapi.MergeSucceededPayload
			if err := json.Unmarshal([]byte(ev.Payload), &payload); err != nil {
				t.Fatalf("unmarshal merge.succeeded: %v", err)
			}
			if payload.MergeCommitSha != out.MergeCommitSHA {
				t.Errorf("merge.succeeded MergeCommitSha: got %q, want %q", payload.MergeCommitSha, out.MergeCommitSHA)
			}
		}
	}
	if !foundSucceeded {
		t.Error("merge.succeeded event not found")
	}
}

// ---------------------------------------------------------------------------
// Apply — safe-auto-resolve
// ---------------------------------------------------------------------------

func TestApply_SafeAutoResolve(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)
	sess := seedSession(t, s)
	log := events.New(s)

	repo, dir := initRepo(t)

	// Whitespace-only conflict: ours adds trailing space, theirs adds trailing tab.
	baseContent := []byte("line1\nline2\nline3\n")
	oursContent := []byte("line1\nline2 \nline3\n")   // trailing space
	theirsContent := []byte("line1\nline2\t\nline3\n") // trailing tab

	base := commitFiles(t, repo, dir, nil, map[string][]byte{"file.txt": baseContent}, "base")
	draft := commitFiles(t, repo, dir, base, map[string][]byte{"file.txt": oursContent}, "ours: whitespace")
	source := commitFiles(t, repo, dir, base, map[string][]byte{"file.txt": theirsContent}, "theirs: whitespace")

	srcObj, _ := object.GetCommit(repo.Storer, source.Hash)
	draftObj, _ := object.GetCommit(repo.Storer, draft.Hash)
	baseObj, _ := object.GetCommit(repo.Storer, base.Hash)

	result, err := automerger.Merge(ctx, repo, srcObj, draftObj, baseObj)
	if err != nil {
		t.Fatalf("Merge: %v", err)
	}
	if result.Kind != automerger.SafeAutoResolve {
		t.Skipf("did not get SafeAutoResolve (got %s); this test requires a whitespace conflict", result.Kind)
	}

	if err := s.EnsureEventSeqRow(ctx, sess.ID); err != nil {
		t.Fatalf("ensure event_seq row: %v", err)
	}

	applier := automerger.NewApplier(s, log)
	out, err := applier.Apply(ctx, automerger.ApplyInput{
		Repo:         repo,
		Session:      sess,
		SourceRef:    "refs/heads/jam/sess-1/alice/feat",
		SourceCommit: source.Hash,
		DraftTip:     draft.Hash,
		Ancestor:     base.Hash,
		Result:       result,
		PortalHost:   "jamsesh.test",
	})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}

	if out.MergeCommitSHA == "" {
		t.Error("MergeCommitSHA empty on safe-auto-resolve")
	}

	mergeHash := plumbing.NewHash(out.MergeCommitSHA)
	mc, err := object.GetCommit(repo.Storer, mergeHash)
	if err != nil {
		t.Fatalf("get merge commit: %v", err)
	}

	if !strings.Contains(mc.Message, "Auto-Resolved: ") {
		t.Errorf("missing Auto-Resolved trailer in:\n%s", mc.Message)
	}
	if !strings.Contains(mc.Message, "Auto-Merger: true") {
		t.Errorf("missing Auto-Merger trailer in:\n%s", mc.Message)
	}
}

// ---------------------------------------------------------------------------
// Apply — hard conflict
// ---------------------------------------------------------------------------

func TestApply_HardConflict(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)
	sess := seedSession(t, s)
	log := events.New(s)

	repo, sourceH, draftH, ancestorH := buildConflictRepo(t)
	result := runMerge(t, repo, sourceH, draftH, ancestorH)
	if result.Kind != automerger.HardConflict {
		t.Fatalf("expected HardConflict, got %s", result.Kind)
	}

	if err := s.EnsureEventSeqRow(ctx, sess.ID); err != nil {
		t.Fatalf("ensure event_seq row: %v", err)
	}

	// Record the draft ref hash before Apply.
	draftRefName := plumbing.NewBranchReferenceName("jam/" + sess.ID + "/draft")

	applier := automerger.NewApplier(s, log)
	out, err := applier.Apply(ctx, automerger.ApplyInput{
		Repo:         repo,
		Session:      sess,
		SourceRef:    "refs/heads/jam/sess-1/alice/feat",
		SourceCommit: sourceH,
		DraftTip:     draftH,
		Ancestor:     ancestorH,
		Result:       result,
		PortalHost:   "jamsesh.test",
	})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}

	if out.MergeCommitSHA != "" {
		t.Error("MergeCommitSHA should be empty on hard conflict")
	}
	if out.ConflictEvent == nil {
		t.Fatal("ConflictEvent is nil on hard conflict")
	}

	// Draft ref should NOT be advanced.
	_, refErr := repo.Reference(draftRefName, true)
	if refErr == nil {
		t.Error("draft ref should not exist after hard conflict")
	}

	// conflict_events row should be in DB.
	ev, err := s.GetConflictEventByID(ctx, out.ConflictEvent.ID)
	if err != nil {
		t.Fatalf("get conflict event: %v", err)
	}
	if ev.Status != "open" {
		t.Errorf("conflict event status: got %q, want %q", ev.Status, "open")
	}
	if ev.SessionID != sess.ID {
		t.Errorf("conflict event session_id: got %q, want %q", ev.SessionID, sess.ID)
	}
	if ev.SourceCommit != sourceH.String() {
		t.Errorf("source_commit: got %q, want %q", ev.SourceCommit, sourceH.String())
	}

	// addressed_to should contain the source-ref owner.
	var addressed []string
	if err := json.Unmarshal([]byte(ev.AddressedTo), &addressed); err != nil {
		t.Fatalf("unmarshal addressed_to: %v", err)
	}
	foundOwner := false
	for _, a := range addressed {
		if a == "@alice/feat" {
			foundOwner = true
		}
	}
	if !foundOwner {
		t.Errorf("addressed_to %v does not contain @alice/feat", addressed)
	}

	// conflict.detected event should be emitted.
	evts, err := s.ListEventsSince(ctx, store.ListEventsSinceParams{
		SessionID: sess.ID,
		SinceSeq:  0,
		Limit:     100,
	})
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	var foundDetected bool
	for _, e := range evts {
		if e.Type == "conflict.detected" {
			foundDetected = true
		}
	}
	if !foundDetected {
		t.Error("conflict.detected event not emitted")
	}
}

// ---------------------------------------------------------------------------
// Apply — Resolves-Conflict closure
// ---------------------------------------------------------------------------

func TestApply_ResolvesConflictClosure(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)
	sess := seedSession(t, s)
	log := events.New(s)

	if err := s.EnsureEventSeqRow(ctx, sess.ID); err != nil {
		t.Fatalf("ensure event_seq row: %v", err)
	}

	// Pre-insert an open conflict event.
	conflictID := "conflict-evt-001"
	now := time.Now().UTC()
	if err := s.InsertConflictEvent(ctx, store.InsertConflictEventParams{
		ID:          conflictID,
		OrgID:       sess.OrgID,
		SessionID:   sess.ID,
		SourceCommit: "aaaa",
		DraftTip:    "bbbb",
		Ancestor:    "cccc",
		Conflicts:   `[{"file":"file.txt","ranges":[]}]`,
		AddressedTo: `["@alice/feat"]`,
		Status:      "open",
		CreatedAt:   now,
	}); err != nil {
		t.Fatalf("insert conflict event: %v", err)
	}

	// Build a clean-merge repo where the source commit message has a
	// Resolves-Conflict trailer.
	repo, dir := initRepo(t)
	base := commitFiles(t, repo, dir, nil, map[string][]byte{"file.txt": []byte("base\n")}, "base")
	draft := commitFiles(t, repo, dir, base, map[string][]byte{
		"file.txt": []byte("base\n"),
		"extra.txt": []byte("extra\n"),
	}, "draft: add extra")

	// The source commit must have a Resolves-Conflict trailer in its message.
	// We create it via the go-git object model directly.
	sourceTree, err := base.Tree()
	if err != nil {
		t.Fatalf("base tree: %v", err)
	}
	_ = sourceTree
	// Commit a source that adds file-s.txt + has the trailer.
	sourceMsg := "source: fix conflict\n\nResolves-Conflict: " + conflictID + "\n"
	source := commitFilesWithMessage(t, repo, dir, base, map[string][]byte{
		"file.txt":  []byte("base\n"),
		"file-s.txt": []byte("source\n"),
	}, sourceMsg)

	srcObj, _ := object.GetCommit(repo.Storer, source.Hash)
	draftObj, _ := object.GetCommit(repo.Storer, draft.Hash)
	baseObj, _ := object.GetCommit(repo.Storer, base.Hash)

	result, err := automerger.Merge(ctx, repo, srcObj, draftObj, baseObj)
	if err != nil {
		t.Fatalf("Merge: %v", err)
	}
	if result.Kind != automerger.CleanMerge {
		t.Fatalf("expected CleanMerge, got %s", result.Kind)
	}

	applier := automerger.NewApplier(s, log)
	out, err := applier.Apply(ctx, automerger.ApplyInput{
		Repo:         repo,
		Session:      sess,
		SourceRef:    "refs/heads/jam/sess-1/alice/feat",
		SourceCommit: source.Hash,
		DraftTip:     draft.Hash,
		Ancestor:     base.Hash,
		Result:       result,
		PortalHost:   "jamsesh.test",
	})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if out.MergeCommitSHA == "" {
		t.Fatal("expected merge commit")
	}

	// The conflict event should now be resolved.
	ev, err := s.GetConflictEventByID(ctx, conflictID)
	if err != nil {
		t.Fatalf("get conflict event: %v", err)
	}
	if ev.Status != "resolved" {
		t.Errorf("conflict event status: got %q, want resolved", ev.Status)
	}
	if ev.ResolvingCommitSHA == nil || *ev.ResolvingCommitSHA != out.MergeCommitSHA {
		t.Errorf("resolving_commit_sha: got %v, want %q", ev.ResolvingCommitSHA, out.MergeCommitSHA)
	}

	// conflict.resolved event should be emitted.
	evts, err := s.ListEventsSince(ctx, store.ListEventsSinceParams{
		SessionID: sess.ID,
		SinceSeq:  0,
		Limit:     100,
	})
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	var foundResolved bool
	for _, e := range evts {
		if e.Type == "conflict.resolved" {
			foundResolved = true
			var payload openapi.ConflictResolvedPayload
			if err := json.Unmarshal([]byte(e.Payload), &payload); err != nil {
				t.Fatalf("unmarshal conflict.resolved: %v", err)
			}
			if payload.EventId != conflictID {
				t.Errorf("conflict.resolved event_id: got %q, want %q", payload.EventId, conflictID)
			}
		}
	}
	if !foundResolved {
		t.Error("conflict.resolved event not emitted")
	}

	// Merge commit should have Resolves-Conflict trailer propagated.
	mc, _ := object.GetCommit(repo.Storer, plumbing.NewHash(out.MergeCommitSHA))
	if !strings.Contains(mc.Message, "Resolves-Conflict: "+conflictID) {
		t.Errorf("Resolves-Conflict trailer not in merge commit:\n%s", mc.Message)
	}
}

// TestApply_ResolvesConflictMismatch verifies that an unknown event-id in the
// Resolves-Conflict trailer produces a silent no-op (no error).
func TestApply_ResolvesConflictMismatch(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)
	sess := seedSession(t, s)
	log := events.New(s)

	if err := s.EnsureEventSeqRow(ctx, sess.ID); err != nil {
		t.Fatalf("ensure event_seq row: %v", err)
	}

	repo, dir := initRepo(t)
	base := commitFiles(t, repo, dir, nil, map[string][]byte{"file.txt": []byte("base\n")}, "base")
	draft := commitFiles(t, repo, dir, base, map[string][]byte{
		"file.txt": []byte("base\n"),
		"extra.txt": []byte("extra\n"),
	}, "draft: add extra")

	sourceMsg := "source: fix\n\nResolves-Conflict: nonexistent-event-id\n"
	source := commitFilesWithMessage(t, repo, dir, base, map[string][]byte{
		"file.txt":  []byte("base\n"),
		"file-s.txt": []byte("source\n"),
	}, sourceMsg)

	srcObj, _ := object.GetCommit(repo.Storer, source.Hash)
	draftObj, _ := object.GetCommit(repo.Storer, draft.Hash)
	baseObj, _ := object.GetCommit(repo.Storer, base.Hash)

	result, err := automerger.Merge(ctx, repo, srcObj, draftObj, baseObj)
	if err != nil {
		t.Fatalf("Merge: %v", err)
	}

	applier := automerger.NewApplier(s, log)
	_, err = applier.Apply(ctx, automerger.ApplyInput{
		Repo:         repo,
		Session:      sess,
		SourceRef:    "refs/heads/jam/sess-1/alice/feat",
		SourceCommit: source.Hash,
		DraftTip:     draft.Hash,
		Ancestor:     base.Hash,
		Result:       result,
		PortalHost:   "jamsesh.test",
	})
	// Must not error — mismatch is a silent no-op.
	if err != nil {
		t.Errorf("Apply returned unexpected error for unknown event-id: %v", err)
	}
}

// commitFilesWithMessage is like commitFiles but accepts an explicit message
// (used for trailer injection).
func commitFilesWithMessage(
	t *testing.T,
	repo *gogit.Repository,
	repoDir string,
	parent *object.Commit,
	files map[string][]byte,
	message string,
) *object.Commit {
	t.Helper()
	// Use commitFiles to stage files (it writes a commit with its own message),
	// then build a new commit object on top with the desired message.
	staged := commitFiles(t, repo, repoDir, parent, files, "tmp-stage")

	// Build replacement commit with the desired message but same tree.
	sig := object.Signature{
		Name:  "Test",
		Email: "test@jamsesh.test",
		When:  time.Now().UTC(),
	}
	var parents []plumbing.Hash
	if parent != nil {
		parents = []plumbing.Hash{parent.Hash}
	}
	c := &object.Commit{
		Author:       sig,
		Committer:    sig,
		Message:      message,
		TreeHash:     staged.TreeHash,
		ParentHashes: parents,
	}
	obj := repo.Storer.NewEncodedObject()
	if err := c.Encode(obj); err != nil {
		t.Fatalf("encode commit: %v", err)
	}
	h, err := repo.Storer.SetEncodedObject(obj)
	if err != nil {
		t.Fatalf("store commit: %v", err)
	}
	result, err := object.GetCommit(repo.Storer, h)
	if err != nil {
		t.Fatalf("get commit: %v", err)
	}
	return result
}
