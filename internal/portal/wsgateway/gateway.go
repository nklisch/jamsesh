// Package wsgateway implements the WebSocket gateway for the jamsesh portal.
// It serves GET /ws/sessions/{sessionID} — one connection per session
// subscription. Authentication happens at upgrade time via the
// Sec-WebSocket-Protocol header carrying a short-lived single-use ticket
// ("jamsesh-ticket.<ticket>"). The SPA obtains the ticket via
// POST /api/auth/ws-ticket immediately before opening the socket, so the
// long-lived bearer token is never placed in the Sec-WebSocket-Protocol header.
//
// After a successful upgrade the handler:
//  1. Registers the connection in the per-session subscription set.
//  2. Optionally replays historical events if the client sends
//     {"replay_from": <seq>} as its first text frame.
//  3. Fans out live events from events.Log.Subscribe to all connected clients.
//  4. Sends 30-second heartbeat pings; closes on pong timeout.
//  5. Closes slow consumers with status 1008 when their send buffer fills.
package wsgateway

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/go-chi/chi/v5"

	"jamsesh/internal/db/store"
	"jamsesh/internal/portal/events"
)

// Gateway is the WebSocket fan-out hub. Construct it with all exported fields
// populated and call Start(ctx) before registering Handler() on a router.
// Call Stop() (or cancel the Start context) during graceful shutdown.
type Gateway struct {
	// Store is used for membership checks at upgrade time.
	Store store.Store
	// Tickets is the short-lived ticket store. The handler consumes a ticket
	// from the Sec-WebSocket-Protocol header to authenticate the upgrade.
	Tickets *TicketStore
	// Log is the event log; the Gateway subscribes to all events on Start.
	Log *events.Log
	// AllowOrigins is passed to websocket.AcceptOptions.OriginPatterns for
	// CSRF defence. An empty slice causes every cross-origin upgrade to be
	// rejected — the intentional secure default. Operators MUST populate this
	// from config for browser clients to connect.
	AllowOrigins []string

	mu    sync.RWMutex
	subs  map[string]map[*conn]struct{} // sessionID -> set of live conns
	unsub func()                        // cancels the events.Log subscription
}

// conn is a single WebSocket connection registered in the subscription set.
type conn struct {
	ws        *websocket.Conn
	sessionID string
	orgID     string
	account   *store.Account
	// send is the per-connection delivery queue. Non-blocking send; overflow
	// triggers a slow-consumer close.
	send      chan events.Event
	closeOnce sync.Once
}

// envelope is the JSON shape written to the client for every event.
type envelope struct {
	Seq       int64           `json:"seq"`
	Version   int             `json:"version"`
	Type      string          `json:"type"`
	Timestamp string          `json:"timestamp"`
	SessionID string          `json:"session_id"`
	Payload   json.RawMessage `json:"payload"`
}

// Start subscribes to all events from the event log and begins the fan-out
// goroutine. It must be called before Handler() is registered. Start returns
// immediately; the fan-out runs until ctx is cancelled.
func (g *Gateway) Start(ctx context.Context) error {
	g.mu.Lock()
	g.subs = make(map[string]map[*conn]struct{})
	g.mu.Unlock()

	ch, unsub := g.Log.Subscribe("") // empty filter = all event types
	g.unsub = unsub
	go g.fanout(ctx, ch)
	return nil
}

// Stop cancels the event log subscription. Connections already in-flight
// will be cleaned up by their own request-context cancellation.
func (g *Gateway) Stop() {
	if g.unsub != nil {
		g.unsub()
	}
}

// fanout reads from the global events channel and non-blocking sends to every
// conn subscribed to the event's session. On send-buffer overflow it closes
// the connection with status 1008 (policy violation).
func (g *Gateway) fanout(ctx context.Context, ch <-chan events.Event) {
	for {
		select {
		case <-ctx.Done():
			return
		case e, ok := <-ch:
			if !ok {
				return
			}
			g.mu.RLock()
			conns := g.subs[e.SessionID]
			// Snapshot under read-lock; iterate without lock to avoid holding
			// it during potentially-slow channel sends.
			list := make([]*conn, 0, len(conns))
			for c := range conns {
				list = append(list, c)
			}
			g.mu.RUnlock()

			for _, c := range list {
				select {
				case c.send <- e:
				default:
					// Buffer full — slow consumer. Close once to avoid double-close.
					c.closeOnce.Do(func() {
						c.ws.Close(websocket.StatusPolicyViolation, "subscriber too slow")
					})
				}
			}
		}
	}
}

// Handler returns the http.HandlerFunc that handles WebSocket upgrade requests
// at /ws/sessions/{sessionID}. The route parameter name must be "sessionID".
func (g *Gateway) Handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sessionID := chi.URLParam(r, "sessionID")

		// --- Auth: consume single-use ticket from subprotocol header ---
		// The client presents "jamsesh-ticket.<ticket>" in Sec-WebSocket-Protocol.
		// The ticket was issued by POST /api/auth/ws-ticket moments before the
		// upgrade and is valid for 60 seconds, single-use. Using a ticket here
		// means the long-lived bearer token is never placed in this header.
		proto := r.Header.Get("Sec-WebSocket-Protocol")
		ticketVal, ok := strings.CutPrefix(proto, "jamsesh-ticket.")
		if !ok {
			http.Error(w, "missing subprotocol ticket", http.StatusUnauthorized)
			return
		}

		account := g.Tickets.Consume(ticketVal)
		if account == nil {
			http.Error(w, "invalid or expired ticket", http.StatusUnauthorized)
			return
		}

		// --- Membership check ---
		memberships, err := g.Store.ListSessionMembershipsForAccount(r.Context(), account.ID)
		if err != nil {
			http.Error(w, "server error", http.StatusInternalServerError)
			return
		}
		var orgID string
		member := false
		for _, m := range memberships {
			if m.SessionID == sessionID {
				member = true
				orgID = m.OrgID
				break
			}
		}
		if !member {
			http.Error(w, "not a member", http.StatusForbidden)
			return
		}

		// --- WebSocket upgrade ---
		ws, err := websocket.Accept(w, r, &websocket.AcceptOptions{
			Subprotocols:   []string{proto}, // MUST echo the exact value from the client
			OriginPatterns: g.AllowOrigins,
		})
		if err != nil {
			// Accept writes its own error response; just return.
			return
		}
		defer ws.CloseNow()

		c := &conn{
			ws:        ws,
			sessionID: sessionID,
			orgID:     orgID,
			account:   account,
			send:      make(chan events.Event, 64),
		}
		g.register(c)
		defer g.unregister(c)

		// --- Optional replay-from-cursor ---
		// The client MAY send {"replay_from": N} as its first text frame after
		// connecting. We must NOT pass a short-lived context to wsjson.Read
		// because coder/websocket closes the connection on context expiration.
		// Instead, we read in a goroutine and select with a timer so the conn is
		// never closed by our timeout logic.
		//
		// The read goroutine becomes the sole owner of the Read half for its
		// lifetime. The main goroutine only writes; this satisfies the library's
		// no-concurrent-reads constraint.
		type replayResult struct {
			seq int64
			ok  bool
		}
		replayCh := make(chan replayResult, 1)
		go func() {
			var hdr struct {
				ReplayFrom int64 `json:"replay_from"`
			}
			if err := wsjson.Read(r.Context(), ws, &hdr); err == nil && hdr.ReplayFrom > 0 {
				replayCh <- replayResult{seq: hdr.ReplayFrom, ok: true}
			} else {
				replayCh <- replayResult{}
			}
			// After the first frame (or error), keep draining the connection so
			// that pings are handled and disconnects are detected. This goroutine
			// runs until r.Context() is cancelled (client disconnects or server
			// shuts down).
			for {
				if _, _, err := ws.Read(r.Context()); err != nil {
					return
				}
			}
		}()

		select {
		case res := <-replayCh:
			if res.ok {
				replayed, listErr := g.Log.ListSince(r.Context(), sessionID, res.seq, 1000)
				if listErr == nil {
					for _, e := range replayed {
						if werr := writeEnvelope(r.Context(), ws, e); werr != nil {
							return
						}
					}
				}
			}
		case <-time.After(2 * time.Second):
			// No replay-from frame arrived within 2 seconds; proceed to live mode.
		case <-r.Context().Done():
			return
		}

		// --- Heartbeat + live send loop ---
		heartbeat := time.NewTicker(30 * time.Second)
		defer heartbeat.Stop()

		for {
			select {
			case <-r.Context().Done():
				return
			case e, ok := <-c.send:
				if !ok {
					return
				}
				if err := writeEnvelope(r.Context(), ws, e); err != nil {
					return
				}
			case <-heartbeat.C:
				pingCtx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
				err := ws.Ping(pingCtx)
				cancel()
				if err != nil {
					return
				}
			}
		}
	}
}

// register adds c to the per-session subscription set.
func (g *Gateway) register(c *conn) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.subs[c.sessionID] == nil {
		g.subs[c.sessionID] = make(map[*conn]struct{})
	}
	g.subs[c.sessionID][c] = struct{}{}
}

// unregister removes c from the per-session subscription set and cleans up
// empty session buckets.
func (g *Gateway) unregister(c *conn) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if set, ok := g.subs[c.sessionID]; ok {
		delete(set, c)
		if len(set) == 0 {
			delete(g.subs, c.sessionID)
		}
	}
}

// writeEnvelope serialises e as an EventEnvelope JSON text frame and writes
// it to ws.
func writeEnvelope(ctx context.Context, ws *websocket.Conn, e events.Event) error {
	env := envelope{
		Seq:       e.Seq,
		Version:   1,
		Type:      e.Type,
		Timestamp: e.CreatedAt.Format(time.RFC3339),
		SessionID: e.SessionID,
		Payload:   e.Payload,
	}
	return wsjson.Write(ctx, ws, env)
}
