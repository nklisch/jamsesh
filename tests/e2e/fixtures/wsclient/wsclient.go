// Package wsclient provides a WebSocket client fixture for e2e specs that need
// to subscribe to a jamsesh portal session's WebSocket event stream.
//
// Authentication uses the Sec-WebSocket-Protocol subprotocol mechanism:
//
//	Sec-WebSocket-Protocol: jamsesh.bearer.<token>
//
// which is the only reliable auth path for browser WebSocket clients and is
// the same mechanism used by the production portal gateway.
package wsclient

import (
	"context"
	"encoding/json"
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
}

// Connect dials /ws/sessions/{sessionID} on the portal, authenticating with
// bearer via the Sec-WebSocket-Protocol subprotocol header. It registers
// t.Cleanup(c.Close) so callers do not need to close manually.
func Connect(ctx context.Context, t *testing.T, portalURL, sessionID, bearer string) *Client {
	t.Helper()

	wsURL := strings.Replace(portalURL, "http://", "ws://", 1) + "/ws/sessions/" + sessionID
	proto := "jamsesh.bearer." + bearer

	conn, _, err := websocket.Dial(ctx, wsURL, &websocket.DialOptions{
		Subprotocols: []string{proto},
	})
	if err != nil {
		t.Fatalf("wsclient.Connect: dial %s: %v", wsURL, err)
	}

	cctx, cancel := context.WithCancel(ctx)
	c := &Client{
		conn:   conn,
		events: make(chan Event, 64),
		cancel: cancel,
		done:   make(chan struct{}),
	}
	t.Cleanup(c.Close)
	go c.readLoop(cctx)
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
