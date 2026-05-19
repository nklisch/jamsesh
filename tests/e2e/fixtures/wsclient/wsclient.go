// Package wsclient provides a WebSocket client fixture for e2e specs that need
// to subscribe to a jamsesh portal session's WebSocket event stream.
//
// Authentication uses the two-step ticket flow:
//  1. POST /api/auth/ws-ticket with Authorization: Bearer <token> to obtain a
//     short-lived single-use ticket.
//  2. Dial the WebSocket endpoint with the ticket in the Sec-WebSocket-Protocol
//     subprotocol header: jamsesh-ticket.<ticket>.
//
// This matches the production gateway's expected authentication protocol.
package wsclient

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

// Event mirrors the portal's WebSocket event envelope as written by
// wsgateway.writeEnvelope. Field names match the JSON tags on
// internal/portal/wsgateway.envelope.
type Event struct {
	Seq       int64           `json:"seq"`
	Version   int             `json:"version"`
	Type      string          `json:"type"`
	Payload   json.RawMessage `json:"payload"`
	Timestamp string          `json:"timestamp"`
	SessionID string          `json:"session_id"`
}

// Client is a single-session WebSocket subscriber. Create one per test (or per
// agent per test) via Connect. It reads events into an internal channel until
// Close is called or the test ends.
type Client struct {
	conn   *websocket.Conn
	events chan Event
	cancel context.CancelFunc
	done   chan struct{}
	// readCtx is the context bound to the read loop. It is derived from the
	// caller's ctx via WithCancel; cancel() is the matching CancelFunc.
	readCtx context.Context
}

// Connect dials /ws/sessions/{sessionID} on the portal, authenticating with
// bearer via the Sec-WebSocket-Protocol subprotocol header. It registers
// t.Cleanup(c.Close) so callers do not need to close manually.
func Connect(ctx context.Context, t *testing.T, portalURL, sessionID, bearer string) *Client {
	t.Helper()
	c := dial(ctx, t, portalURL, sessionID, bearer)
	go c.readLoop(c.readCtx)
	return c
}

// ConnectFromSeq behaves like Connect but additionally writes a
// {"replay_from": replaySeq} text frame as the first message after the
// WebSocket handshake, so the gateway replays missed events with
// seq > replaySeq before transitioning to live mode.
//
// If replaySeq <= 0, ConnectFromSeq is equivalent to Connect (no frame is
// sent). The replay frame is written BEFORE the internal read loop starts so
// the read half of the WebSocket is free; the portal's reply goroutine reads
// the frame and switches to replay mode (see
// internal/portal/wsgateway/gateway.go around the replay_from handler).
func ConnectFromSeq(ctx context.Context, t *testing.T, portalURL, sessionID, bearer string, replaySeq int64) *Client {
	t.Helper()
	c := dial(ctx, t, portalURL, sessionID, bearer)
	if replaySeq > 0 {
		type replayHdr struct {
			ReplayFrom int64 `json:"replay_from"`
		}
		if err := wsjson.Write(ctx, c.conn, replayHdr{ReplayFrom: replaySeq}); err != nil {
			// Start the read loop before failing so the t.Cleanup-registered
			// Close — which blocks on the read loop's done channel — can
			// complete during teardown.
			go c.readLoop(c.readCtx)
			t.Fatalf("wsclient.ConnectFromSeq: write replay_from: %v", err)
		}
	}
	go c.readLoop(c.readCtx)
	return c
}

// fetchWsTicket calls POST /api/auth/ws-ticket with the given bearer token and
// returns the single-use ticket string. The ticket is valid for 60 seconds and
// must be consumed immediately by the WebSocket dial that follows.
func fetchWsTicket(ctx context.Context, t *testing.T, portalURL, bearer string) string {
	t.Helper()

	ticketURL := portalURL + "/api/auth/ws-ticket"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, ticketURL, http.NoBody)
	if err != nil {
		t.Fatalf("wsclient: build ws-ticket request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+bearer)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("wsclient: POST %s: %v", ticketURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("wsclient: POST %s: unexpected status %d: %s", ticketURL, resp.StatusCode, body)
	}

	var ticketResp struct {
		Ticket           string `json:"ticket"`
		ExpiresInSeconds int    `json:"expires_in_seconds"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&ticketResp); err != nil {
		t.Fatalf("wsclient: decode ws-ticket response: %v", err)
	}
	if ticketResp.Ticket == "" {
		t.Fatalf("wsclient: ws-ticket response contained empty ticket")
	}
	return ticketResp.Ticket
}

// dial fetches a single-use WS ticket, builds the WebSocket URL, opens the
// connection with the ticket subprotocol, and constructs a Client. It is the
// single source of truth for auth + URL construction shared by Connect and
// ConnectFromSeq.
//
// The returned Client has its read loop NOT yet started — the caller is
// responsible for starting it (typically with `go c.readLoop(c.readCtx)`),
// after any pre-readloop writes (like a replay_from frame) have been issued.
func dial(ctx context.Context, t *testing.T, portalURL, sessionID, bearer string) *Client {
	t.Helper()

	ticket := fetchWsTicket(ctx, t, portalURL, bearer)
	wsURL := strings.Replace(portalURL, "http://", "ws://", 1) + "/ws/sessions/" + sessionID
	proto := fmt.Sprintf("jamsesh-ticket.%s", ticket)

	conn, _, err := websocket.Dial(ctx, wsURL, &websocket.DialOptions{
		Subprotocols: []string{proto},
	})
	if err != nil {
		t.Fatalf("wsclient: dial %s: %v", wsURL, err)
	}

	cctx, cancel := context.WithCancel(ctx)
	c := &Client{
		conn:    conn,
		events:  make(chan Event, 64),
		cancel:  cancel,
		done:    make(chan struct{}),
		readCtx: cctx,
	}
	t.Cleanup(c.Close)
	return c
}

// Events returns a read-only channel of all events received so far.
// The channel is buffered (capacity 64); callers that drain slowly may lose
// events if the buffer fills (the server will close slow consumers anyway).
func (c *Client) Events() <-chan Event {
	return c.events
}

// WaitFor blocks until an event whose Type equals eventType arrives, or until
// timeout expires. It drains and discards non-matching events so that a single
// call to WaitFor does not require all prior events to be consumed first.
//
// The test is failed (t.Fatalf) on timeout.
func (c *Client) WaitFor(t *testing.T, eventType string, timeout time.Duration) Event {
	t.Helper()
	deadline := time.After(timeout)
	for {
		select {
		case ev, ok := <-c.events:
			if !ok {
				t.Fatalf("wsclient.WaitFor(%q): event channel closed before event arrived", eventType)
				return Event{}
			}
			if ev.Type == eventType {
				return ev
			}
			// Discard non-matching events and keep waiting.
		case <-deadline:
			t.Fatalf("wsclient.WaitFor(%q): timed out after %s", eventType, timeout)
			return Event{} // unreachable
		}
	}
}

// Close cancels the read loop and closes the WebSocket connection. Safe to
// call multiple times. Registered automatically via t.Cleanup by Connect.
func (c *Client) Close() {
	c.cancel()
	// CloseNow is idempotent and does not block.
	c.conn.CloseNow()
	<-c.done
}

// readLoop runs in its own goroutine. It reads JSON events from the WebSocket
// and forwards them to the events channel. It exits when ctx is cancelled or
// the connection is closed.
func (c *Client) readLoop(ctx context.Context) {
	defer close(c.done)
	for {
		var ev Event
		if err := wsjson.Read(ctx, c.conn, &ev); err != nil {
			return
		}
		select {
		case c.events <- ev:
		case <-ctx.Done():
			return
		}
	}
}
