package comments_test

// TestCommentsFanoutCarriesSeq verifies that both Create and Resolve fan out
// the tx-allocated seq and eventID on the ws event, not the zero-value defaults.
//
// Without the fix: FanOut was called with Seq=0, ID="" so the SPA's
// lastSeenSeq cursor never advanced, causing duplicate redelivery on reconnect.

import (
	"context"
	"strings"
	"testing"
	"time"

	"jamsesh/internal/portal/comments"
	"jamsesh/internal/portal/events"
)

func TestCommentsFanoutCarriesSeqOnCreate(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	// Subscribe to comment.added events before creating.
	ch, unsub := env.svc.Log.Subscribe("comment.added")
	defer unsub()

	comment, err := env.svc.Create(ctx, comments.CreateParams{
		OrgID:           env.orgID,
		SessionID:       env.sessID,
		AuthorAccountID: env.accID,
		AuthorKind:      "human",
		AnchorCommitSHA: strings.Repeat("a", 40),
		Body:            "hello",
		Kind:            "fyi",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	var got events.Event
	select {
	case got = <-ch:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout: no comment.added event received on fan-out channel")
	}

	if got.Seq <= 0 {
		t.Errorf("fanned-out event Seq = %d, want > 0 (tx-allocated seq)", got.Seq)
	}
	if got.ID == "" {
		t.Errorf("fanned-out event ID is empty, want non-empty eventID")
	}
	if got.Type != "comment.added" {
		t.Errorf("fanned-out event Type = %q, want %q", got.Type, "comment.added")
	}
	if got.SessionID != env.sessID {
		t.Errorf("fanned-out event SessionID = %q, want %q", got.SessionID, env.sessID)
	}
	_ = comment
}

func TestCommentsFanoutCarriesSeqOnResolve(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	// First create the comment.
	comment, err := env.svc.Create(ctx, comments.CreateParams{
		OrgID:           env.orgID,
		SessionID:       env.sessID,
		AuthorAccountID: env.accID,
		AuthorKind:      "human",
		AnchorCommitSHA: strings.Repeat("b", 40),
		Body:            "to be resolved",
		Kind:            "question",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Subscribe to comment.resolved before resolving.
	ch, unsub := env.svc.Log.Subscribe("comment.resolved")
	defer unsub()

	_, err = env.svc.Resolve(ctx, comments.ResolveParams{
		OrgID:     env.orgID,
		SessionID: env.sessID,
		CommentID: comment.ID,
		AccountID: env.accID,
	})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	var got events.Event
	select {
	case got = <-ch:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout: no comment.resolved event received on fan-out channel")
	}

	if got.Seq <= 0 {
		t.Errorf("fanned-out event Seq = %d, want > 0", got.Seq)
	}
	if got.ID == "" {
		t.Errorf("fanned-out event ID is empty, want non-empty eventID")
	}
	if got.Type != "comment.resolved" {
		t.Errorf("fanned-out event Type = %q, want %q", got.Type, "comment.resolved")
	}
}
