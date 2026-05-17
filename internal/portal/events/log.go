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
	"log/slog"
	"sync"
	"time"

	"github.com/oklog/ulid/v2"

	"jamsesh/internal/db/store"
	"jamsesh/internal/portal/metrics"
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

// subscriberRec is an internal subscriber record holding the channel and an
// optional type filter.
type subscriberRec struct {
	ch     chan Event
	filter string // empty = receive all events
}

// Log is the write-side and read-side entry point for the event log.
// Construct it once per server lifetime via New and share it across
// components.
type Log struct {
	s      store.Store
	metrics *metrics.Registry // optional; nil disables metric recording
	muSubs sync.RWMutex
	subs   []*subscriberRec
}

// New constructs a Log backed by the given store. The store must implement
// EventLogStore and PresenceStore (satisfied by both dialect adapters).
func New(s store.Store) *Log {
	return &Log{s: s}
}

// WithMetrics attaches a metrics Registry to the Log so that every emitted
// event increments EventLogEmitTotal. Returns the same *Log for chaining.
func (l *Log) WithMetrics(reg *metrics.Registry) *Log {
	l.metrics = reg
	return l
}

// Subscribe returns a receive-only channel that will receive events emitted
// to this Log, and an unsubscribe function that must be called to clean up.
//
// typeFilter restricts delivery to events whose Type equals typeFilter. Pass
// an empty string to receive all events. The channel is buffered at 64; if
// the consumer falls behind, events are dropped silently (the worker's
// startup scan catches transient drops).
func (l *Log) Subscribe(typeFilter string) (<-chan Event, func()) {
	ch := make(chan Event, 64)
	sub := &subscriberRec{ch: ch, filter: typeFilter}
	l.muSubs.Lock()
	l.subs = append(l.subs, sub)
	l.muSubs.Unlock()

	unsubscribe := func() {
		l.muSubs.Lock()
		defer l.muSubs.Unlock()
		for i, s := range l.subs {
			if s == sub {
				l.subs = append(l.subs[:i], l.subs[i+1:]...)
				close(ch)
				return
			}
		}
	}
	return ch, unsubscribe
}

// FanOut delivers a pre-built event to all subscribers. It is intended for
// callers that handle DB persistence themselves (e.g. the comments service
// which inserts the event row inside its own transaction). Sends are
// non-blocking; subscribers that are full receive a Warn log and the event is
// dropped.
func (l *Log) FanOut(e Event) {
	l.fanOut(e)
}

// fanOut delivers e to all subscribers whose filter matches. Sends are
// non-blocking; subscribers that are full receive a Warn log and the event is
// dropped.
func (l *Log) fanOut(e Event) {
	l.muSubs.RLock()
	defer l.muSubs.RUnlock()
	for _, s := range l.subs {
		if s.filter != "" && s.filter != e.Type {
			continue
		}
		select {
		case s.ch <- e:
		default:
			slog.Warn("events: subscriber channel full, dropping event",
				"event_type", e.Type,
				"session_id", e.SessionID,
				"seq", e.Seq,
			)
		}
	}
}

// Emit allocates the next seq for the session, inserts one event row, and
// returns the seq. The entire operation runs in a single DB transaction.
//
// orgID is stored on the row for org-scoped queries; sessionID is the
// per-session namespace for seq allocation.
func (l *Log) Emit(ctx context.Context, orgID, sessionID, eventType string, payload json.RawMessage) (int64, error) {
	var seq int64
	var emittedAt time.Time
	var id string
	err := l.s.WithTx(ctx, func(tx store.TxStore) error {
		if err := tx.EnsureEventSeqRow(ctx, sessionID); err != nil {
			return fmt.Errorf("ensure event_seq row: %w", err)
		}
		allocated, err := tx.AllocateNextSeq(ctx, sessionID)
		if err != nil {
			return fmt.Errorf("allocate seq: %w", err)
		}
		seq = allocated
		id = ulid.Make().String()
		emittedAt = time.Now().UTC()
		return tx.InsertEvent(ctx, store.InsertEventParams{
			ID:        id,
			OrgID:     orgID,
			SessionID: sessionID,
			Seq:       seq,
			Type:      eventType,
			Payload:   string(payload),
			CreatedAt: emittedAt,
		})
	})
	if err != nil {
		return 0, err
	}
	l.fanOut(Event{
		ID:        id,
		OrgID:     orgID,
		SessionID: sessionID,
		Seq:       seq,
		Type:      eventType,
		Payload:   payload,
		CreatedAt: emittedAt,
	})
	if l.metrics != nil {
		l.metrics.EventLogEmitTotal.Inc()
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

	// ids holds the generated IDs in order so we can fan out after commit.
	ids := make([]string, len(drafts))

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
			ids[i] = ulid.Make().String()
			if err := tx.InsertEvent(ctx, store.InsertEventParams{
				ID:        ids[i],
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
	for i, draft := range drafts {
		l.fanOut(Event{
			ID:        ids[i],
			OrgID:     orgID,
			SessionID: sessionID,
			Seq:       firstSeq + int64(i),
			Type:      draft.Type,
			Payload:   draft.Payload,
			CreatedAt: now,
		})
	}
	if l.metrics != nil {
		l.metrics.EventLogEmitTotal.Add(float64(len(drafts)))
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
	var seq int64
	var id string
	err = l.s.WithTx(ctx, func(tx store.TxStore) error {
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
		allocated, err := tx.AllocateNextSeq(ctx, sessionID)
		if err != nil {
			return fmt.Errorf("allocate seq: %w", err)
		}
		seq = allocated
		id = ulid.Make().String()
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
	if err != nil {
		return err
	}
	l.fanOut(Event{
		ID:        id,
		OrgID:     orgID,
		SessionID: sessionID,
		Seq:       seq,
		Type:      "presence.updated",
		Payload:   payloadBytes,
		CreatedAt: now,
	})
	return nil
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
