package finalize_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/oklog/ulid/v2"

	"jamsesh/internal/api/openapi"
	"jamsesh/internal/db/store"
)

// setSessionStatus is a test helper that flips the session to the given
// status using the existing store methods.
func setSessionStatus(t *testing.T, env *finalizeEnv, status string) {
	t.Helper()
	if err := env.store.UpdateSessionStatus(context.Background(), store.UpdateSessionStatusParams{
		OrgID:  env.orgID,
		ID:     env.sessID,
		Status: status,
	}); err != nil {
		t.Fatalf("set session status %s: %v", status, err)
	}
}

// setSessionEnded sets status=ended with the given end_reason and ended_at.
func setSessionEnded(t *testing.T, env *finalizeEnv, endReason string) {
	t.Helper()
	now := time.Now().UTC()
	if err := env.store.UpdateSessionStatus(context.Background(), store.UpdateSessionStatusParams{
		OrgID: env.orgID, ID: env.sessID, Status: "ended",
	}); err != nil {
		t.Fatalf("set ended status: %v", err)
	}
	if err := env.store.SetSessionEndReason(context.Background(), store.SetSessionEndReasonParams{
		OrgID: env.orgID, ID: env.sessID, EndReason: &endReason, EndedAt: &now,
	}); err != nil {
		t.Fatalf("set end reason %s: %v", endReason, err)
	}
}

// seedActiveLock inserts an active finalize lock for env.caller and points
// sessions.finalize_locked_by at them. Returns the lock id.
func seedActiveLock(t *testing.T, env *finalizeEnv) string {
	t.Helper()
	lockID := ulid.Make().String()
	now := time.Now().UTC()
	if err := env.store.InsertFinalizeLock(context.Background(), store.InsertFinalizeLockParams{
		ID:                  lockID,
		OrgID:               env.orgID,
		SessionID:           env.sessID,
		AcquiredByAccountID: env.caller.ID,
		AcquiredAt:          now,
		LastActivityAt:      now,
		SelectedCommitSHAs:  "[]",
		Mode:                "squash",
	}); err != nil {
		t.Fatalf("insert lock: %v", err)
	}
	accID := env.caller.ID
	if err := env.store.SetFinalizeLock(context.Background(), store.SetFinalizeLockParams{
		OrgID: env.orgID, ID: env.sessID, AccountID: &accID,
	}); err != nil {
		t.Fatalf("set sessions pointer: %v", err)
	}
	return lockID
}

func TestMarkSessionShipped_HappyTransition(t *testing.T) {
	env := newFinalizeEnv(t)
	setSessionStatus(t, env, "finalizing")

	events, unsub := env.log.Subscribe("session.ended")
	defer unsub()

	resp, err := env.handler.MarkSessionShipped(env.callerCtx, openapi.MarkSessionShippedRequestObject{
		OrgID:     env.orgID,
		SessionID: env.sessID,
	})
	if err != nil {
		t.Fatalf("mark shipped: %v", err)
	}
	r, ok := resp.(openapi.MarkSessionShipped200JSONResponse)
	if !ok {
		t.Fatalf("expected 200, got %T", resp)
	}
	if r.Status != openapi.SessionStatus("ended") {
		t.Errorf("status = %q, want ended", r.Status)
	}
	if r.EndReason != "shipped" {
		t.Errorf("end_reason = %q, want shipped", r.EndReason)
	}
	if r.EndedAt.IsZero() {
		t.Error("ended_at is zero")
	}

	sess, err := env.store.GetSession(context.Background(), env.orgID, env.sessID)
	if err != nil {
		t.Fatalf("re-get session: %v", err)
	}
	if sess.Status != "ended" {
		t.Errorf("persisted status = %q, want ended", sess.Status)
	}
	if sess.EndReason == nil || *sess.EndReason != "shipped" {
		t.Errorf("persisted end_reason = %v, want shipped", sess.EndReason)
	}
	if sess.EndedAt == nil {
		t.Error("persisted ended_at is nil")
	}

	// Event was emitted with reason=shipped.
	select {
	case e := <-events:
		if e.Type != "session.ended" {
			t.Errorf("event type = %q, want session.ended", e.Type)
		}
		var payload struct {
			Reason          string `json:"reason"`
			FinalBranchName string `json:"final_branch_name,omitempty"`
		}
		if err := json.Unmarshal(e.Payload, &payload); err != nil {
			t.Fatalf("unmarshal payload: %v", err)
		}
		if payload.Reason != "shipped" {
			t.Errorf("payload.reason = %q, want shipped", payload.Reason)
		}
		if payload.FinalBranchName != "" {
			t.Errorf("payload.final_branch_name = %q, want empty", payload.FinalBranchName)
		}
	case <-time.After(time.Second):
		t.Error("no session.ended event emitted")
	}
}

func TestMarkSessionShipped_WithFinalBranchName_OnPayload(t *testing.T) {
	env := newFinalizeEnv(t)
	setSessionStatus(t, env, "finalizing")

	events, unsub := env.log.Subscribe("session.ended")
	defer unsub()

	resp, err := env.handler.MarkSessionShipped(env.callerCtx, openapi.MarkSessionShippedRequestObject{
		OrgID:     env.orgID,
		SessionID: env.sessID,
		Body: &openapi.MarkSessionShippedJSONRequestBody{
			FinalBranchName: "release/2026-05-17",
		},
	})
	if err != nil {
		t.Fatalf("mark shipped: %v", err)
	}
	if _, ok := resp.(openapi.MarkSessionShipped200JSONResponse); !ok {
		t.Fatalf("expected 200, got %T", resp)
	}

	select {
	case e := <-events:
		var payload struct {
			Reason          string `json:"reason"`
			FinalBranchName string `json:"final_branch_name,omitempty"`
		}
		if err := json.Unmarshal(e.Payload, &payload); err != nil {
			t.Fatalf("unmarshal payload: %v", err)
		}
		if payload.FinalBranchName != "release/2026-05-17" {
			t.Errorf("payload.final_branch_name = %q, want release/2026-05-17", payload.FinalBranchName)
		}
	case <-time.After(time.Second):
		t.Error("no session.ended event emitted")
	}
}

func TestMarkSessionShipped_Idempotent_AlreadyShipped(t *testing.T) {
	env := newFinalizeEnv(t)
	setSessionStatus(t, env, "finalizing")

	// First call.
	resp1, err := env.handler.MarkSessionShipped(env.callerCtx, openapi.MarkSessionShippedRequestObject{
		OrgID: env.orgID, SessionID: env.sessID,
	})
	if err != nil {
		t.Fatalf("first mark: %v", err)
	}
	r1 := resp1.(openapi.MarkSessionShipped200JSONResponse)

	// Second call — should be idempotent (no error, same row).
	resp2, err := env.handler.MarkSessionShipped(env.callerCtx, openapi.MarkSessionShippedRequestObject{
		OrgID: env.orgID, SessionID: env.sessID,
	})
	if err != nil {
		t.Fatalf("second mark: %v", err)
	}
	r2, ok := resp2.(openapi.MarkSessionShipped200JSONResponse)
	if !ok {
		t.Fatalf("second call: expected 200, got %T", resp2)
	}
	if r1.EndedAt != r2.EndedAt {
		t.Errorf("idempotent re-call mutated ended_at: %s -> %s", r1.EndedAt, r2.EndedAt)
	}
	if r2.EndReason != "shipped" {
		t.Errorf("end_reason = %q, want shipped", r2.EndReason)
	}
}

func TestMarkSessionShipped_409_NotFinalizing_WhenActive(t *testing.T) {
	env := newFinalizeEnv(t)
	// Session is created as "active" by newFinalizeEnv — no need to flip.

	resp, err := env.handler.MarkSessionShipped(env.callerCtx, openapi.MarkSessionShippedRequestObject{
		OrgID: env.orgID, SessionID: env.sessID,
	})
	if err != nil {
		t.Fatalf("mark shipped: %v", err)
	}
	r, ok := resp.(openapi.MarkSessionShipped409JSONResponse)
	if !ok {
		t.Fatalf("expected 409, got %T", resp)
	}
	if r.Error != "session.not_finalizing" {
		t.Errorf("error = %q, want session.not_finalizing", r.Error)
	}
}

func TestMarkSessionShipped_409_AlreadyEnded_WithOtherReason(t *testing.T) {
	env := newFinalizeEnv(t)
	setSessionEnded(t, env, "abandoned")

	resp, err := env.handler.MarkSessionShipped(env.callerCtx, openapi.MarkSessionShippedRequestObject{
		OrgID: env.orgID, SessionID: env.sessID,
	})
	if err != nil {
		t.Fatalf("mark shipped: %v", err)
	}
	r, ok := resp.(openapi.MarkSessionShipped409JSONResponse)
	if !ok {
		t.Fatalf("expected 409, got %T", resp)
	}
	if r.Error != "session.already_ended" {
		t.Errorf("error = %q, want session.already_ended", r.Error)
	}
	if r.Details == nil {
		t.Fatal("expected details with end_reason")
	}
	if got := r.Details["end_reason"]; got != "abandoned" {
		t.Errorf("details.end_reason = %v, want abandoned", got)
	}
}

func TestMarkSessionShipped_ReleasesHeldFinalizeLock(t *testing.T) {
	env := newFinalizeEnv(t)
	setSessionStatus(t, env, "finalizing")
	lockID := seedActiveLock(t, env)

	resp, err := env.handler.MarkSessionShipped(env.callerCtx, openapi.MarkSessionShippedRequestObject{
		OrgID: env.orgID, SessionID: env.sessID,
	})
	if err != nil {
		t.Fatalf("mark shipped: %v", err)
	}
	if _, ok := resp.(openapi.MarkSessionShipped200JSONResponse); !ok {
		t.Fatalf("expected 200, got %T", resp)
	}

	// Lock row has released_at set.
	lock, err := env.store.GetFinalizeLockByID(context.Background(), lockID)
	if err != nil {
		t.Fatalf("re-get lock: %v", err)
	}
	if lock.ReleasedAt == nil {
		t.Error("lock.released_at is nil; want non-nil")
	}

	// Lookup of active lock returns ErrNotFound (no active row left).
	if _, err := env.store.GetActiveFinalizeLockForSession(context.Background(), env.sessID); err == nil {
		t.Error("expected no active lock after mark-shipped, got one")
	}

	// Sessions pointer is cleared.
	sess, err := env.store.GetSession(context.Background(), env.orgID, env.sessID)
	if err != nil {
		t.Fatalf("re-get session: %v", err)
	}
	if sess.FinalizeLockedByAccountID != nil {
		t.Errorf("finalize_locked_by_account_id = %v, want nil", sess.FinalizeLockedByAccountID)
	}
}

func TestMarkSessionShipped_NonMember_403(t *testing.T) {
	env := newFinalizeEnv(t)
	setSessionStatus(t, env, "finalizing")

	// Build an account that is in the org but not a session member.
	now := time.Now().UTC()
	outsiderID := ulid.Make().String()
	outsider, err := env.store.CreateAccount(context.Background(), store.CreateAccountParams{
		ID: outsiderID, Email: "outsider@ex.com", DisplayName: "Outsider", CreatedAt: now,
	})
	if err != nil {
		t.Fatalf("create outsider: %v", err)
	}
	if err := env.store.AddOrgMember(context.Background(), store.AddOrgMemberParams{
		OrgID: env.orgID, AccountID: outsiderID, Role: "member", CreatedAt: now,
	}); err != nil {
		t.Fatalf("add outsider to org: %v", err)
	}
	ctx := contextWithAccount(context.Background(), &outsider)

	resp, err := env.handler.MarkSessionShipped(ctx, openapi.MarkSessionShippedRequestObject{
		OrgID: env.orgID, SessionID: env.sessID,
	})
	if err != nil {
		t.Fatalf("mark shipped: %v", err)
	}
	if _, ok := resp.(openapi.MarkSessionShipped403JSONResponse); !ok {
		t.Fatalf("expected 403, got %T", resp)
	}
}

func TestMarkSessionShipped_Unauthenticated_401(t *testing.T) {
	env := newFinalizeEnv(t)
	setSessionStatus(t, env, "finalizing")

	resp, err := env.handler.MarkSessionShipped(context.Background(), openapi.MarkSessionShippedRequestObject{
		OrgID: env.orgID, SessionID: env.sessID,
	})
	if err != nil {
		t.Fatalf("mark shipped: %v", err)
	}
	if _, ok := resp.(openapi.MarkSessionShipped401JSONResponse); !ok {
		t.Fatalf("expected 401, got %T", resp)
	}
}

func TestMarkSessionShipped_SessionNotFound_404(t *testing.T) {
	env := newFinalizeEnv(t)

	resp, err := env.handler.MarkSessionShipped(env.callerCtx, openapi.MarkSessionShippedRequestObject{
		OrgID: env.orgID, SessionID: ulid.Make().String(),
	})
	if err != nil {
		t.Fatalf("mark shipped: %v", err)
	}
	if _, ok := resp.(openapi.MarkSessionShipped404JSONResponse); !ok {
		t.Fatalf("expected 404, got %T", resp)
	}
}

func TestMarkSessionShipped_Idempotent_NoEventOnReship(t *testing.T) {
	env := newFinalizeEnv(t)
	setSessionStatus(t, env, "finalizing")

	// First call emits.
	if _, err := env.handler.MarkSessionShipped(env.callerCtx, openapi.MarkSessionShippedRequestObject{
		OrgID: env.orgID, SessionID: env.sessID,
	}); err != nil {
		t.Fatalf("first mark: %v", err)
	}

	// Subscribe AFTER the first call so we only see emissions from the
	// second one (or lack thereof).
	events, unsub := env.log.Subscribe("session.ended")
	defer unsub()

	if _, err := env.handler.MarkSessionShipped(env.callerCtx, openapi.MarkSessionShippedRequestObject{
		OrgID: env.orgID, SessionID: env.sessID,
	}); err != nil {
		t.Fatalf("second mark: %v", err)
	}

	select {
	case e := <-events:
		t.Errorf("second mark-shipped emitted an event: %+v", e)
	case <-time.After(100 * time.Millisecond):
		// Good — no event.
	}
}
