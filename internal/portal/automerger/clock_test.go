package automerger_test

import (
	"context"
	"testing"
	"time"

	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"

	"jamsesh/internal/portal/automerger"
	"jamsesh/internal/portal/events"
)

// fakeClock is a controllable time source used to exercise the clock-
// injection path on the automerger.Applier. Mirrors the shape of
// fakeClock in internal/portal/auth/magic_link_test.go.
type fakeClock struct {
	t time.Time
}

func (f *fakeClock) Now() time.Time { return f.t }

// TestApply_CleanMerge_UsesInjectedClockForMergerSignature asserts that
// the auto-merger's committer signature timestamp (When field) reflects
// the clock supplied to NewApplierWithClock — proving the injected
// clock replaced the real time source for merger signatures.
func TestApply_CleanMerge_UsesInjectedClockForMergerSignature(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)
	sess := seedSession(t, s)
	log := events.New(s)

	repo, sourceH, draftH, ancestorH := buildApplyRepo(t)
	result := runMerge(t, repo, sourceH, draftH, ancestorH)
	if result.Kind != automerger.CleanMerge {
		t.Fatalf("expected CleanMerge, got %s", result.Kind)
	}
	if err := s.EnsureEventSeqRow(ctx, sess.ID); err != nil {
		t.Fatalf("ensure event_seq row: %v", err)
	}

	fixed := time.Date(2026, 5, 17, 12, 0, 0, 0, time.UTC)
	clk := &fakeClock{t: fixed}
	applier := automerger.NewApplierWithClock(s, log, clk)

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

	mergeHash := plumbing.NewHash(out.MergeCommitSHA)
	mc, err := object.GetCommit(repo.Storer, mergeHash)
	if err != nil {
		t.Fatalf("get merge commit: %v", err)
	}
	if !mc.Committer.When.Equal(fixed) {
		t.Errorf("Committer.When: want %v, got %v", fixed, mc.Committer.When)
	}
}
