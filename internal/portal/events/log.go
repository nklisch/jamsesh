// Package events provides the event log for the portal. Producers emit typed
// events by marshalling their structs into json.RawMessage and calling Emit or
// EmitBatch. The Log handles monotonic per-session sequence allocation inside
// DB transactions so that seqs are gapless and collision-free under concurrent
// push traffic.
package events

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/oklog/ulid/v2"

	"jamsesh/internal/db/store"
)

// Event is a domain-level event row returned by ListSince.
type Event struct {
	ID        string
	OrgID     string
	SessionID string
	Seq       int64
	Type      string
	Payload   json.RawMessage
	CreatedAt time.Time
}

// DraftEvent is a single event to be emitted as part of an EmitBatch call.
type DraftEvent struct {
	Type    string
	Payload json.RawMessage
}

// Log is the write-side and read-side entry point for the event log.
// Construct it once per server lifetime via New and share it across
// components.
type Log struct {
	s store.Store
}

// New constructs a Log backed by the given store. The store must implement
// EventLogStore and PresenceStore (satisfied by both dialect adapters).
func New(s store.Store) *Log {
	return &Log{s: s}
}

// Emit allocates the next seq for the session, inserts one event row, and
// returns the seq. The entire operation runs in a single DB transaction.
//
// orgID is stored on the row for org-scoped queries; sessionID is the
// per-session namespace for seq allocation.
func (l *Log) Emit(ctx context.Context, orgID, sessionID, eventType string, payload json.RawMessage) (int64, error) {
	var seq int64
	err := l.s.WithTx(ctx, func(tx store.TxStore) error {
		if err := tx.EnsureEventSeqRow(ctx, sessionID); err != nil {
			return fmt.Errorf("ensure event_seq row: %w", err)
		}
		allocated, err := tx.AllocateNextSeq(ctx, sessionID)
		if err != nil {
			return fmt.Errorf("allocate seq: %w", err)
		}
		seq = allocated
		id := ulid.Make().String()
		return tx.InsertEvent(ctx, store.InsertEventParams{
			ID:        id,
			OrgID:     orgID,
			SessionID: sessionID,
			Seq:       seq,
			Type:      eventType,
			Payload:   string(payload),
			CreatedAt: time.Now().UTC(),
		})
	})
	if err != nil {
		return 0, err
	}
	return seq, nil
}

// EmitBatch emits n events in a single transaction with contiguous seq values.
// It returns the first allocated seq. All events share the same sessionID and
// orgID; each draft carries its own Type and Payload.
//
// If drafts is empty the call is a no-op and returns 0, nil.
func (l *Log) EmitBatch(ctx context.Context, orgID, sessionID string, drafts []DraftEvent) (int64, error) {
	if len(drafts) == 0 {
		return 0, nil
	}
	n := int64(len(drafts))
	var firstSeq int64
	now := time.Now().UTC()

	err := l.s.WithTx(ctx, func(tx store.TxStore) error {
		if err := tx.EnsureEventSeqRow(ctx, sessionID); err != nil {
			return fmt.Errorf("ensure event_seq row: %w", err)
		}
		// AllocateNextSeqN increments by n and returns the LAST allocated seq.
		// The range is [last-n+1, last].
		last, err := tx.AllocateNextSeqN(ctx, sessionID, n)
		if err != nil {
			return fmt.Errorf("allocate %d seqs: %w", n, err)
		}
		firstSeq = last - n + 1
		for i, draft := range drafts {
			seq := firstSeq + int64(i)
			id := ulid.Make().String()
			if err := tx.InsertEvent(ctx, store.InsertEventParams{
				ID:        id,
				OrgID:     orgID,
				SessionID: sessionID,
				Seq:       seq,
				Type:      draft.Type,
				Payload:   string(draft.Payload),
				CreatedAt: now,
			}); err != nil {
				return fmt.Errorf("insert event seq=%d: %w", seq, err)
			}
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return firstSeq, nil
}

// UpdatePresence upserts the presence row for the given (sessionID, accountID,
// ref) triple and emits a "presence.updated" event — both in a single
// transaction. If either operation fails, both are rolled back.
func (l *Log) UpdatePresence(ctx context.Context, orgID, sessionID, accountID, ref, currentSHA string) error {
	type presencePayload struct {
		AccountID  string `json:"account_id"`
		Ref        string `json:"ref"`
		CurrentSHA string `json:"current_sha"`
	}
	payloadBytes, err := json.Marshal(presencePayload{
		AccountID:  accountID,
		Ref:        ref,
		CurrentSHA: currentSHA,
	})
	if err != nil {
		return fmt.Errorf("marshal presence payload: %w", err)
	}

	now := time.Now().UTC()
	return l.s.WithTx(ctx, func(tx store.TxStore) error {
		if err := tx.UpsertPresence(ctx, store.UpsertPresenceParams{
			OrgID:        orgID,
			SessionID:    sessionID,
			AccountID:    accountID,
			Ref:          ref,
			CurrentSHA:   currentSHA,
			LastActiveAt: now,
		}); err != nil {
			return fmt.Errorf("upsert presence: %w", err)
		}
		if err := tx.EnsureEventSeqRow(ctx, sessionID); err != nil {
			return fmt.Errorf("ensure event_seq row: %w", err)
		}
		seq, err := tx.AllocateNextSeq(ctx, sessionID)
		if err != nil {
			return fmt.Errorf("allocate seq: %w", err)
		}
		id := ulid.Make().String()
		return tx.InsertEvent(ctx, store.InsertEventParams{
			ID:        id,
			OrgID:     orgID,
			SessionID: sessionID,
			Seq:       seq,
			Type:      "presence.updated",
			Payload:   string(payloadBytes),
			CreatedAt: now,
		})
	})
}

// ListSince returns events with seq > sinceSeq for the given session, in
// ascending seq order, up to limit rows. It is used by the digest cursor to
// stream new events to clients.
func (l *Log) ListSince(ctx context.Context, sessionID string, sinceSeq int64, limit int) ([]Event, error) {
	rows, err := l.s.ListEventsSince(ctx, store.ListEventsSinceParams{
		SessionID: sessionID,
		SinceSeq:  sinceSeq,
		Limit:     int64(limit),
	})
	if err != nil {
		return nil, err
	}
	events := make([]Event, len(rows))
	for i, r := range rows {
		events[i] = Event{
			ID:        r.ID,
			OrgID:     r.OrgID,
			SessionID: r.SessionID,
			Seq:       r.Seq,
			Type:      r.Type,
			Payload:   json.RawMessage(r.Payload),
			CreatedAt: r.CreatedAt,
		}
	}
	return events, nil
}
