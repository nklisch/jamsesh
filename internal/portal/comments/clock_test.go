package comments_test

import (
	"context"
	"testing"
	"time"

	"jamsesh/internal/portal/comments"
	"jamsesh/internal/portal/events"
)

// fakeClock is a controllable time source used to exercise the clock-
// injection path on the comments.Service. Mirrors the shape of
// fakeClock in internal/portal/auth/magic_link_test.go.
type fakeClock struct {
	t time.Time
}

func (f *fakeClock) Now() time.Time { return f.t }

// TestService_Create_UsesInjectedClock asserts that a comment created
// through a Service whose Clock field is set picks up the fake clock's
// Now() for the CreatedAt stamp — proving the injected clock replaced
// the real time source.
func TestService_Create_UsesInjectedClock(t *testing.T) {
	ctx := context.Background()
	env := newTestEnv(t)

	fixed := time.Date(2026, 5, 17, 12, 0, 0, 0, time.UTC)
	svc := &comments.Service{Store: env.s, Log: events.New(env.s), Clock: &fakeClock{t: fixed}}

	comment, err := svc.Create(ctx, comments.CreateParams{
		OrgID:           env.orgID,
		SessionID:       env.sessID,
		AuthorAccountID: env.accID,
		AuthorKind:      "human",
		AnchorCommitSHA: "abc1234",
		Body:            "test comment",
		Kind:            "question",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if !comment.CreatedAt.Equal(fixed) {
		t.Errorf("CreatedAt: want %v, got %v", fixed, comment.CreatedAt)
	}
}

// TestService_NilClockFallsBackToReal asserts that a Service constructed
// without a Clock field falls back to the real wall clock via the now()
// helper — the nil-safe fallback contract.
func TestService_NilClockFallsBackToReal(t *testing.T) {
	ctx := context.Background()
	env := newTestEnv(t)

	svc := &comments.Service{Store: env.s, Log: events.New(env.s)} // Clock unset

	before := time.Now().UTC()
	comment, err := svc.Create(ctx, comments.CreateParams{
		OrgID:           env.orgID,
		SessionID:       env.sessID,
		AuthorAccountID: env.accID,
		AuthorKind:      "human",
		AnchorCommitSHA: "abc1234",
		Body:            "test comment",
		Kind:            "question",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	after := time.Now().UTC()

	if comment.CreatedAt.Before(before) || comment.CreatedAt.After(after) {
		t.Errorf("CreatedAt %v not in [%v, %v] — nil-safe fallback broken", comment.CreatedAt, before, after)
	}
}
