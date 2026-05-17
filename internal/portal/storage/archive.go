package storage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"jamsesh/internal/db/store"
)

// ArchiveSession moves a live session to the archived state. The operation is
// ordered as follows to preserve consistency:
//  1. INSERT into archived_sessions (idempotent — unique violation → no-op).
//  2. Hard-delete the bare repo directory via RemoveRepo.
//  3. DELETE the live sessions row (cascades session_members via FK).
//
// If the session has already been archived (step 1 returns ErrUniqueViolation),
// the method returns nil immediately without touching the repo or the row.
func (s *service) ArchiveSession(ctx context.Context, orgID, sessionID string, info ArchiveInfo) error {
	memberJSON, err := json.Marshal(info.MemberAccountIDs)
	if err != nil {
		return fmt.Errorf("storage: marshal member_account_ids: %w", err)
	}

	insertErr := s.store.InsertArchivedSession(ctx, store.InsertArchivedSessionParams{
		SessionID:        sessionID,
		OrgID:            orgID,
		Name:             info.Name,
		GoalText:         info.GoalText,
		MemberAccountIDs: string(memberJSON),
		EndedAt:          info.EndedAt,
		ArchivedAt:       s.clock.Now(),
		EndReason:        info.EndReason,
		FinalBranchName:  info.FinalBranchName,
	})
	if insertErr != nil {
		// Unique violation: already archived — treat as no-op.
		if errors.Is(insertErr, store.ErrUniqueViolation) {
			return nil
		}
		return fmt.Errorf("storage: insert archived row: %w", insertErr)
	}

	// Hard-delete the bare repo.
	if err := s.RemoveRepo(ctx, orgID, sessionID); err != nil {
		return fmt.Errorf("storage: remove repo: %w", err)
	}

	// Delete the live sessions row; session_members cascade via FK.
	if err := s.store.DeleteSession(ctx, store.DeleteSessionParams{
		OrgID: orgID,
		ID:    sessionID,
	}); err != nil {
		return fmt.Errorf("storage: delete session row: %w", err)
	}

	return nil
}

// LookupArchived returns the archived record for a session. Returns
// store.ErrNotFound if the session has not been archived (or does not exist).
func (s *service) LookupArchived(ctx context.Context, orgID, sessionID string) (*ArchivedRecord, error) {
	row, err := s.store.GetArchivedSession(ctx, store.GetArchivedSessionParams{
		OrgID:     orgID,
		SessionID: sessionID,
	})
	if err != nil {
		return nil, err // propagates store.ErrNotFound to callers
	}
	return &ArchivedRecord{
		SessionID:        row.SessionID,
		OrgID:            row.OrgID,
		Name:             row.Name,
		GoalText:         row.GoalText,
		MemberAccountIDs: row.MemberAccountIDs,
		EndedAt:          row.EndedAt,
		ArchivedAt:       row.ArchivedAt,
		EndReason:        row.EndReason,
		FinalBranchName:  row.FinalBranchName,
	}, nil
}
