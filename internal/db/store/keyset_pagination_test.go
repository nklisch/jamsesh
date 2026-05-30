package store_test

// TestKeysetPaginationCommentsNoDuplicates tests that cursor pagination
// correctly handles rows sharing the same created_at by using the (created_at, id)
// keyset. Without the id tiebreaker, rows with identical created_at at a page
// boundary are silently dropped.
//
// Test: insert limit+2 comments with identical created_at, page through them,
// assert each id appears exactly once (no drops, no duplicates).

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"jamsesh/internal/db/store"
	"jamsesh/internal/db/store/storetest"
)

// maxIDSentinel sorts after every ULID (used as the first-page LastID).
const maxIDSentinel = "zzzzzzzzzzzzzzzzzzzzzzzzzz"

func TestKeysetPaginationCommentsNoDuplicates(t *testing.T) {
	for _, h := range storetest.Stores(t) {
		h := h
		t.Run(h.Name, func(t *testing.T) {
			ctx := context.Background()
			s := h.Open(t)

			// ---- seed ----
			now := time.Now().UTC().Truncate(time.Second)
			org, err := s.CreateOrg(ctx, store.CreateOrgParams{
				ID: "kp-org-" + h.Name, Name: "KP Org", Slug: "kp-org-" + h.Name, CreatedAt: now,
			})
			if err != nil {
				t.Fatalf("CreateOrg: %v", err)
			}
			acc, err := s.CreateAccount(ctx, store.CreateAccountParams{
				ID: "kp-acc-" + h.Name, Email: "kp-" + h.Name + "@x.com", DisplayName: "KP", CreatedAt: now,
			})
			if err != nil {
				t.Fatalf("CreateAccount: %v", err)
			}
			sess, err := s.CreateSession(ctx, store.CreateSessionParams{
				ID:            "kp-sess-" + h.Name,
				OrgID:         org.ID,
				Name:          "KP sess",
				Goal:          "test",
				WritableScope: `["src/"]`,
				DefaultMode:   "sync",
				Status:        "active",
				CreatedAt:     now,
			})
			if err != nil {
				t.Fatalf("CreateSession: %v", err)
			}
			if err := s.AddOrgMember(ctx, store.AddOrgMemberParams{
				OrgID: org.ID, AccountID: acc.ID, Role: "member", CreatedAt: now,
			}); err != nil {
				t.Fatalf("AddOrgMember: %v", err)
			}
			if err := s.AddSessionMember(ctx, store.AddSessionMemberParams{
				OrgID: org.ID, SessionID: sess.ID, AccountID: acc.ID, Role: "member", JoinedAt: now,
			}); err != nil {
				t.Fatalf("AddSessionMember: %v", err)
			}

			// Use the SAME created_at for all comments so they share the boundary.
			commentCreatedAt := now.Add(-time.Minute) // slightly in the past so Before sentinel works
			const total = 7                           // limit+2 when limit=5
			const pageLimit = 5

			wantIDs := make(map[string]bool, total)
			for i := 0; i < total; i++ {
				// Pad to ensure deterministic ULID-like ordering: higher i = lexicographically larger id.
				id := fmt.Sprintf("kp-comment-%s-%02d", h.Name, i)
				// ids must be exactly 26 chars for the sentinel comparison to work properly;
				// but for this test we just need uniqueness and lexicographic ordering to be testable.
				err := s.WithTx(ctx, func(tx store.TxStore) error {
					return tx.InsertComment(ctx, store.InsertCommentParams{
						ID:              id,
						OrgID:           org.ID,
						SessionID:       sess.ID,
						AuthorAccountID: acc.ID,
						AuthorKind:      "human",
						AnchorCommitSHA: strings.Repeat("a", 40),
						Kind:            "fyi",
						Body:            fmt.Sprintf("comment %d", i),
						CreatedAt:       commentCreatedAt,
					})
				})
				if err != nil {
					t.Fatalf("InsertComment %d: %v", i, err)
				}
				wantIDs[id] = true
			}

			// ---- page through ----
			seen := make(map[string]int, total)
			lastID := maxIDSentinel
			before := now.Add(time.Second) // first page sentinel: slightly in the future

			for page := 0; ; page++ {
				rows, err := s.ListCommentsForSession(ctx, store.ListCommentsForSessionParams{
					SessionID: sess.ID,
					Before:    before,
					LastID:    lastID,
					Limit:     pageLimit + 1, // fetch +1 to detect next page
				})
				if err != nil {
					t.Fatalf("page %d: ListCommentsForSession: %v", page, err)
				}

				hasNext := len(rows) > pageLimit
				if hasNext {
					rows = rows[:pageLimit]
				}

				for _, r := range rows {
					seen[r.ID]++
				}

				if !hasNext {
					break
				}

				// Advance cursor
				last := rows[len(rows)-1]
				before = last.CreatedAt
				lastID = last.ID
			}

			// ---- assert ----
			for id := range wantIDs {
				if seen[id] == 0 {
					t.Errorf("id %q was dropped (not seen on any page)", id)
				} else if seen[id] > 1 {
					t.Errorf("id %q appeared %d times (duplicate)", id, seen[id])
				}
			}
			for id, count := range seen {
				if !wantIDs[id] {
					t.Errorf("id %q appeared %d times but was not inserted", id, count)
				}
			}
			if len(seen) != total {
				t.Errorf("saw %d distinct ids, want %d", len(seen), total)
			}
		})
	}
}

func TestKeysetPaginationSessionsNoDuplicates(t *testing.T) {
	for _, h := range storetest.Stores(t) {
		h := h
		t.Run(h.Name, func(t *testing.T) {
			ctx := context.Background()
			s := h.Open(t)

			now := time.Now().UTC().Truncate(time.Second)
			org, err := s.CreateOrg(ctx, store.CreateOrgParams{
				ID: "kps-org-" + h.Name, Name: "KPS Org", Slug: "kps-org-" + h.Name, CreatedAt: now,
			})
			if err != nil {
				t.Fatalf("CreateOrg: %v", err)
			}

			// All sessions share the same created_at.
			sessCreatedAt := now.Add(-time.Minute)
			const total = 7
			const pageLimit = 5

			wantIDs := make(map[string]bool, total)
			for i := 0; i < total; i++ {
				id := fmt.Sprintf("kps-sess-%s-%02d", h.Name, i)
				_, err := s.CreateSession(ctx, store.CreateSessionParams{
					ID:            id,
					OrgID:         org.ID,
					Name:          fmt.Sprintf("sess %d", i),
					Goal:          "test",
					WritableScope: `["src/"]`,
					DefaultMode:   "sync",
					Status:        "active",
					CreatedAt:     sessCreatedAt,
				})
				if err != nil {
					t.Fatalf("CreateSession %d: %v", i, err)
				}
				wantIDs[id] = true
			}

			seen := make(map[string]int, total)
			lastID := maxIDSentinel
			before := now.Add(time.Second)

			for page := 0; ; page++ {
				rows, err := s.ListSessionsForOrgWithCursor(ctx, store.ListSessionsForOrgWithCursorParams{
					OrgID:  org.ID,
					Before: before,
					LastID: lastID,
					Limit:  pageLimit + 1,
				})
				if err != nil {
					t.Fatalf("page %d: ListSessionsForOrgWithCursor: %v", page, err)
				}

				hasNext := len(rows) > pageLimit
				if hasNext {
					rows = rows[:pageLimit]
				}

				for _, r := range rows {
					seen[r.ID]++
				}

				if !hasNext {
					break
				}

				last := rows[len(rows)-1]
				before = last.CreatedAt
				lastID = last.ID
			}

			for id := range wantIDs {
				if seen[id] == 0 {
					t.Errorf("id %q was dropped", id)
				} else if seen[id] > 1 {
					t.Errorf("id %q appeared %d times (duplicate)", id, seen[id])
				}
			}
			if len(seen) != total {
				t.Errorf("saw %d distinct ids, want %d", len(seen), total)
			}
		})
	}
}
