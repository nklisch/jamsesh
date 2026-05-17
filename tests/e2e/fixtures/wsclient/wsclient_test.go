// Tests for the wsclient fixture. These use an in-process httptest.Server
// with a stub WebSocket handler that mirrors the portal gateway's auth +
// replay_from wire contract closely enough to validate the helpers without
// requiring Docker or the full portal Testcontainer fixture.
//
// The portal-side behaviour is already covered end-to-end by
// internal/portal/wsgateway/gateway_test.go::TestHandler_ReplayFromCursor.
// What we verify here is that the wsclient helpers produce the right wire
// payload — specifically that ConnectFromSeq writes exactly one
// {"replay_from": N} text frame before any reads, and that Connect (and
// ConnectFromSeq with replaySeq <= 0) does not write any frame.

package wsclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

// stubGateway is an in-process WebSocket handler that mirrors the parts of
// the portal gateway relevant to wsclient testing:
//
//   - Requires Sec-WebSocket-Protocol "jamsesh.bearer.<token>".
//   - Tries to read a first text frame and decode it as {"replay_from": N}.
//     If decoded and N > 0, the gateway emits events with seqs N+1..maxSeq.
//   - Then emits any "live" events queued via the liveCh channel.
//
// The handler records the replayFrom value observed (if any) into observed,
// closed when the read of the first frame has completed (whether or not a
// frame was actually present).
type stubGateway struct {
	maxSeq      int64
	liveCh      chan Event
	wantBearer  string

	observed       chan int64 // buffered(1); receives the replay_from seq (0 if none)
	connectionsCh  chan struct{}
}

func newStubGateway(maxSeq int64, wantBearer string) *stubGateway {
	return &stubGateway{
		maxSeq:        maxSeq,
		liveCh:        make(chan Event, 8),
		wantBearer:    wantBearer,
		observed:      make(chan int64, 1),
		connectionsCh: make(chan struct{}, 4),
	}
}

func (s *stubGateway) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	want := "jamsesh.bearer." + s.wantBearer
	ws, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		Subprotocols: []string{want},
	})
	if err != nil {
		return
	}
	if ws.Subprotocol() != want {
		ws.Close(websocket.StatusPolicyViolation, "bad subprotocol")
		return
	}
	defer ws.Close(websocket.StatusNormalClosure, "")

	select {
	case s.connectionsCh <- struct{}{}:
	default:
	}

	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	// Race a single first-frame read against a short deadline so we don't
	// hang forever when the client never sends a replay_from. This mirrors
	// the portal's "wait briefly for replay_from, then go live" behaviour.
	type firstFrame struct {
		seq int64
		ok  bool
	}
	frameCh := make(chan firstFrame, 1)
	go func() {
		var hdr struct {
			ReplayFrom int64 `json:"replay_from"`
		}
		if err := wsjson.Read(ctx, ws, &hdr); err == nil && hdr.ReplayFrom > 0 {
			frameCh <- firstFrame{seq: hdr.ReplayFrom, ok: true}
			return
		}
		frameCh <- firstFrame{}
	}()

	var replayFrom int64
	select {
	case ff := <-frameCh:
		if ff.ok {
			replayFrom = ff.seq
		}
	case <-time.After(300 * time.Millisecond):
		// No frame; treat as live-from-now.
	}
	// Record the observation (non-blocking; observed is buffered).
	select {
	case s.observed <- replayFrom:
	default:
	}

	// Use a fresh context for writes — the read-side may have timed out.
	writeCtx, writeCancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer writeCancel()

	// Replay events with seq > replayFrom up to maxSeq.
	if replayFrom > 0 {
		for seq := replayFrom + 1; seq <= s.maxSeq; seq++ {
			ev := Event{
				Seq:     seq,
				Version: 1,
				Type:    "replay.event",
				Payload: json.RawMessage(`{}`),
			}
			if err := wsjson.Write(writeCtx, ws, ev); err != nil {
				return
			}
		}
	}

	// Drain any live events the test queued.
	for {
		select {
		case ev, ok := <-s.liveCh:
			if !ok {
				return
			}
			if err := wsjson.Write(writeCtx, ws, ev); err != nil {
				return
			}
		case <-r.Context().Done():
			return
		case <-time.After(500 * time.Millisecond):
			return
		}
	}
}

// httpURL converts a fully-formed http://host:port URL from httptest.Server
// into the form wsclient expects (portalURL is http://, wsclient rewrites it
// to ws://).
func httpURL(srv *httptest.Server) string {
	return strings.TrimSuffix(srv.URL, "/")
}

// TestConnect_NoReplayFrame verifies that Connect does not write a
// replay_from frame; the stub's observed value should be 0 (no frame).
func TestConnect_NoReplayFrame(t *testing.T) {
	stub := newStubGateway(0, "token-1")
	srv := httptest.NewServer(stub)
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_ = Connect(ctx, t, httpURL(srv), "sess-1", "token-1")

	select {
	case got := <-stub.observed:
		if got != 0 {
			t.Errorf("stub observed replay_from=%d; want 0 (no frame from Connect)", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for stub gateway to observe first-frame outcome")
	}
}

// TestConnectFromSeq_WritesReplayFrame verifies that ConnectFromSeq writes a
// single {"replay_from": N} text frame and that subsequent replay events with
// seq > N are delivered to the client.
func TestConnectFromSeq_WritesReplayFrame(t *testing.T) {
	const maxSeq int64 = 3
	const replaySeq int64 = 1
	stub := newStubGateway(maxSeq, "token-2")
	srv := httptest.NewServer(stub)
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	c := ConnectFromSeq(ctx, t, httpURL(srv), "sess-2", "token-2", replaySeq)

	select {
	case got := <-stub.observed:
		if got != replaySeq {
			t.Errorf("stub observed replay_from=%d; want %d", got, replaySeq)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for stub gateway to observe replay_from")
	}

	// Expect events with seq 2, 3.
	want := []int64{2, 3}
	for _, wantSeq := range want {
		select {
		case ev := <-c.Events():
			if ev.Seq != wantSeq {
				t.Errorf("replay event: want seq=%d, got seq=%d", wantSeq, ev.Seq)
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("timed out waiting for replay event seq=%d", wantSeq)
		}
	}
}

// TestConnectFromSeq_ZeroSeqDoesNotWriteFrame verifies that
// ConnectFromSeq(..., 0) is equivalent to Connect — no replay_from frame is
// emitted on the wire.
func TestConnectFromSeq_ZeroSeqDoesNotWriteFrame(t *testing.T) {
	stub := newStubGateway(0, "token-3")
	srv := httptest.NewServer(stub)
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_ = ConnectFromSeq(ctx, t, httpURL(srv), "sess-3", "token-3", 0)

	select {
	case got := <-stub.observed:
		if got != 0 {
			t.Errorf("stub observed replay_from=%d; want 0 for replaySeq=0", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for stub gateway to observe first-frame outcome")
	}
}

// TestConnectFromSeq_NegativeSeqDoesNotWriteFrame verifies the same
// no-frame behaviour for a negative replaySeq.
func TestConnectFromSeq_NegativeSeqDoesNotWriteFrame(t *testing.T) {
	stub := newStubGateway(0, "token-4")
	srv := httptest.NewServer(stub)
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_ = ConnectFromSeq(ctx, t, httpURL(srv), "sess-4", "token-4", -5)

	select {
	case got := <-stub.observed:
		if got != 0 {
			t.Errorf("stub observed replay_from=%d; want 0 for replaySeq<0", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for stub gateway to observe first-frame outcome")
	}
}
