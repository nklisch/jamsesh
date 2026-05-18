package wsgateway_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/go-chi/chi/v5"

	"jamsesh/internal/db"
	"jamsesh/internal/db/store"
	"jamsesh/internal/portal/events"
	"jamsesh/internal/portal/tokens"
	"jamsesh/internal/portal/wsgateway"
)

// ---------------------------------------------------------------------------
// Test infrastructure
// ---------------------------------------------------------------------------

var dbCounter atomic.Int64

// openStore opens a uniquely-named shared-cache in-memory SQLite database with
// a single connection so cross-goroutine writes are visible to readers.
func openStore(t *testing.T) store.Store {
	t.Helper()
	n := dbCounter.Add(1)
	dsn := fmt.Sprintf("file:wsgateway_test_%d?mode=memory&cache=shared", n)
	s, _, err := db.Open(context.Background(), "sqlite", dsn, db.PoolConfig{})
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	// Single connection required for shared-cache in-memory SQLite.
	type rawDBer interface {
		RawDB() interface{ SetMaxOpenConns(int) }
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

type testEnv struct {
	store   store.Store
	log     *events.Log
	tokens  tokens.Service
	tickets *wsgateway.TicketStore
	gw      *wsgateway.Gateway
	srv     *httptest.Server
	ctx     context.Context
	cancel  context.CancelFunc
}

// newTestEnv wires up an httptest server with the WS gateway mounted at
// /ws/sessions/{sessionID}.
func newTestEnv(t *testing.T) *testEnv {
	t.Helper()
	s := openStore(t)
	log := events.New(s)
	tokenSvc := tokens.New(s)
	ticketStore := wsgateway.NewTicketStore()
	ticketStore.Start()

	gw := &wsgateway.Gateway{
		Store:        s,
		Tickets:      ticketStore,
		Log:          log,
		AllowOrigins: []string{"*"}, // permissive for tests
	}

	ctx, cancel := context.WithCancel(context.Background())
	if err := gw.Start(ctx); err != nil {
		cancel()
		t.Fatalf("gateway.Start: %v", err)
	}

	r := chi.NewRouter()
	r.Get("/ws/sessions/{sessionID}", gw.Handler())

	srv := httptest.NewServer(r)
	t.Cleanup(func() {
		gw.Stop()
		ticketStore.Stop()
		cancel()
		srv.Close()
	})

	return &testEnv{
		store:   s,
		log:     log,
		tokens:  tokenSvc,
		tickets: ticketStore,
		gw:      gw,
		srv:     srv,
		ctx:     ctx,
		cancel:  cancel,
	}
}

// seed creates an org, account, session, and session membership; returns the
// account and session.
func seed(t *testing.T, s store.Store) (store.Account, store.Session) {
	t.Helper()
	ctx := context.Background()

	org, err := s.CreateOrg(ctx, store.CreateOrgParams{
		ID:        fmt.Sprintf("org-%d", dbCounter.Load()),
		Name:      "Test Org",
		Slug:      fmt.Sprintf("test-org-%d", dbCounter.Load()),
		CreatedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("CreateOrg: %v", err)
	}

	acc, err := s.CreateAccount(ctx, store.CreateAccountParams{
		ID:          fmt.Sprintf("acc-%d", dbCounter.Load()),
		Email:       fmt.Sprintf("user%d@example.com", dbCounter.Load()),
		DisplayName: "Test User",
		CreatedAt:   time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("CreateAccount: %v", err)
	}

	if err := s.AddOrgMember(ctx, store.AddOrgMemberParams{
		OrgID:     org.ID,
		AccountID: acc.ID,
		Role:      "member",
		CreatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("AddOrgMember: %v", err)
	}

	sess, err := s.CreateSession(ctx, store.CreateSessionParams{
		ID:            fmt.Sprintf("sess-%d", dbCounter.Load()),
		OrgID:         org.ID,
		Name:          "Test Session",
		Goal:          "testing",
		WritableScope: `["**"]`,
		DefaultMode:   "sync",
		Status:        "active",
		CreatedAt:     time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	if err := s.AddSessionMember(ctx, store.AddSessionMemberParams{
		OrgID:     org.ID,
		SessionID: sess.ID,
		AccountID: acc.ID,
		Role:      "member",
		JoinedAt:  time.Now().UTC(),
	}); err != nil {
		t.Fatalf("AddSessionMember: %v", err)
	}

	return acc, sess
}

// wsURL converts an httptest server URL to a ws:// URL for the session path.
func wsURL(srv *httptest.Server, sessionID string) string {
	u := "ws" + srv.URL[len("http"):] + "/ws/sessions/" + sessionID
	return u
}

// issueTicket issues a ticket for the given account in the ticket store.
func issueTicket(t *testing.T, ts *wsgateway.TicketStore, acc *store.Account) string {
	t.Helper()
	tok, _, err := ts.Issue(acc)
	if err != nil {
		t.Fatalf("Issue ticket: %v", err)
	}
	return tok
}

// dialWSWithTicket dials the WS endpoint with a ticket in Sec-WebSocket-Protocol.
func dialWSWithTicket(t *testing.T, url, ticket string) (*websocket.Conn, *http.Response) {
	t.Helper()
	proto := "jamsesh-ticket." + ticket
	c, resp, err := websocket.Dial(context.Background(), url, &websocket.DialOptions{
		Subprotocols: []string{proto},
	})
	if err != nil {
		t.Fatalf("websocket.Dial: %v (status=%v)", err, resp)
	}
	return c, resp
}

// readEnvelope reads one JSON text frame from conn and decodes it into the
// envelope map.
func readEnvelope(t *testing.T, ctx context.Context, c *websocket.Conn) map[string]any {
	t.Helper()
	var env map[string]any
	if err := wsjson.Read(ctx, c, &env); err != nil {
		t.Fatalf("read envelope: %v", err)
	}
	return env
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

// TestHandler_SuccessfulUpgrade_LiveEvent verifies the happy-path: a valid
// ticket + membership allows upgrade; emitting an event on the session delivers
// it to the connected client.
func TestHandler_SuccessfulUpgrade_LiveEvent(t *testing.T) {
	tenv := newTestEnv(t)
	acc, sess := seed(t, tenv.store)
	ticket := issueTicket(t, tenv.tickets, &acc)

	c, _ := dialWSWithTicket(t, wsURL(tenv.srv, sess.ID), ticket)
	defer c.CloseNow()

	// Emit an event on the session.
	payload := mustMarshal(t, map[string]string{"sha": "abc123"})
	_, err := tenv.log.Emit(context.Background(), sess.OrgID, sess.ID, "commit.arrived", payload)
	if err != nil {
		t.Fatalf("Emit: %v", err)
	}

	readCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	msg := readEnvelope(t, readCtx, c)

	if got := msg["type"]; got != "commit.arrived" {
		t.Errorf("type: want commit.arrived, got %v", got)
	}
	if got := msg["session_id"]; got != sess.ID {
		t.Errorf("session_id: want %s, got %v", sess.ID, got)
	}
	if got, ok := msg["version"]; !ok || got != float64(1) {
		t.Errorf("version: want 1, got %v", got)
	}
}

// TestHandler_InvalidTicket_Returns401 verifies that an unknown ticket causes
// the upgrade to be rejected with HTTP 401.
func TestHandler_InvalidTicket_Returns401(t *testing.T) {
	tenv := newTestEnv(t)
	_, sess := seed(t, tenv.store)

	url := wsURL(tenv.srv, sess.ID)
	proto := "jamsesh-ticket.not-a-real-ticket"
	_, resp, err := websocket.Dial(context.Background(), url, &websocket.DialOptions{
		Subprotocols: []string{proto},
	})
	if err == nil {
		t.Fatal("expected dial to fail for invalid ticket, but it succeeded")
	}
	if resp == nil {
		t.Fatal("expected an HTTP response, got nil")
	}
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status: want 401, got %d", resp.StatusCode)
	}
}

// TestHandler_MissingSubprotocol_Returns401 verifies that a request with no
// jamsesh-ticket. prefix in the subprotocol is rejected with HTTP 401.
func TestHandler_MissingSubprotocol_Returns401(t *testing.T) {
	tenv := newTestEnv(t)
	_, sess := seed(t, tenv.store)

	url := wsURL(tenv.srv, sess.ID)
	_, resp, err := websocket.Dial(context.Background(), url, &websocket.DialOptions{
		Subprotocols: []string{"not-the-right-protocol"},
	})
	if err == nil {
		t.Fatal("expected dial to fail with wrong protocol, but it succeeded")
	}
	if resp == nil {
		t.Fatal("expected an HTTP response, got nil")
	}
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status: want 401, got %d", resp.StatusCode)
	}
}

// TestHandler_RawBearer_Returns401 verifies that the old bearer-in-protocol
// format is rejected. There is no backwards-compat path.
func TestHandler_RawBearer_Returns401(t *testing.T) {
	tenv := newTestEnv(t)
	acc, sess := seed(t, tenv.store)
	tokenSvc := tokens.New(tenv.store)
	pair, err := tokenSvc.Issue(context.Background(), acc.ID)
	if err != nil {
		t.Fatalf("Issue token: %v", err)
	}

	url := wsURL(tenv.srv, sess.ID)
	// The old format — must be rejected.
	proto := "jamsesh.bearer." + pair.AccessToken
	_, resp, err := websocket.Dial(context.Background(), url, &websocket.DialOptions{
		Subprotocols: []string{proto},
	})
	if err == nil {
		t.Fatal("expected dial to fail for raw bearer (no backwards-compat path), but it succeeded")
	}
	if resp == nil {
		t.Fatal("expected an HTTP response, got nil")
	}
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status: want 401, got %d", resp.StatusCode)
	}
}

// TestHandler_ReusedTicket_Returns401 verifies that a ticket can only be
// consumed once — a second upgrade attempt with the same ticket is rejected.
func TestHandler_ReusedTicket_Returns401(t *testing.T) {
	tenv := newTestEnv(t)
	acc, sess := seed(t, tenv.store)
	ticket := issueTicket(t, tenv.tickets, &acc)

	// First dial — should succeed.
	c, _ := dialWSWithTicket(t, wsURL(tenv.srv, sess.ID), ticket)
	defer c.CloseNow()

	// Second dial with the same ticket — must fail.
	url := wsURL(tenv.srv, sess.ID)
	proto := "jamsesh-ticket." + ticket
	_, resp, err := websocket.Dial(context.Background(), url, &websocket.DialOptions{
		Subprotocols: []string{proto},
	})
	if err == nil {
		t.Fatal("expected second dial with same ticket to fail, but it succeeded")
	}
	if resp == nil {
		t.Fatal("expected an HTTP response, got nil")
	}
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status: want 401 for reused ticket, got %d", resp.StatusCode)
	}
}

// TestHandler_NonMember_Returns403 verifies that a valid ticket for an account
// that is NOT a session member is rejected with HTTP 403.
func TestHandler_NonMember_Returns403(t *testing.T) {
	tenv := newTestEnv(t)
	_, sess := seed(t, tenv.store)

	// Create a separate account that is NOT a member of sess.
	outsider, err := tenv.store.CreateAccount(context.Background(), store.CreateAccountParams{
		ID:          "outsider-1",
		Email:       "outsider@example.com",
		DisplayName: "Outsider",
		CreatedAt:   time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("CreateAccount: %v", err)
	}
	ticket := issueTicket(t, tenv.tickets, &outsider)

	url := wsURL(tenv.srv, sess.ID)
	proto := "jamsesh-ticket." + ticket
	_, resp, err := websocket.Dial(context.Background(), url, &websocket.DialOptions{
		Subprotocols: []string{proto},
	})
	if err == nil {
		t.Fatal("expected dial to fail for non-member, but it succeeded")
	}
	if resp == nil {
		t.Fatal("expected an HTTP response, got nil")
	}
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("status: want 403, got %d", resp.StatusCode)
	}
}

// TestHandler_ReplayFromCursor verifies that a client sending
// {"replay_from": <seq>} as its first frame receives historical events with
// seq > replay_from before transitioning to live events.
func TestHandler_ReplayFromCursor(t *testing.T) {
	tenv := newTestEnv(t)
	acc, sess := seed(t, tenv.store)
	ticket := issueTicket(t, tenv.tickets, &acc)

	ctx := context.Background()
	// Emit 3 events so we have seq 1, 2, 3 in the DB.
	for i := 0; i < 3; i++ {
		payload := mustMarshal(t, map[string]int{"i": i})
		if _, err := tenv.log.Emit(ctx, sess.OrgID, sess.ID, "test.event", payload); err != nil {
			t.Fatalf("Emit %d: %v", i, err)
		}
	}

	c, _ := dialWSWithTicket(t, wsURL(tenv.srv, sess.ID), ticket)
	defer c.CloseNow()

	// Send the replay-from frame: replay_from=1 means we want seq > 1 (seq 2, 3).
	replayReq := map[string]int64{"replay_from": 1}
	if err := wsjson.Write(ctx, c, replayReq); err != nil {
		t.Fatalf("write replay_from frame: %v", err)
	}

	readCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	// Expect seq 2 and 3 from replay.
	for _, wantSeq := range []float64{2, 3} {
		env := readEnvelope(t, readCtx, c)
		if got := env["seq"]; got != wantSeq {
			t.Errorf("replay seq: want %v, got %v", wantSeq, got)
		}
	}

	// Emit a 4th event — should arrive as a live event.
	payload := mustMarshal(t, map[string]string{"live": "yes"})
	if _, err := tenv.log.Emit(ctx, sess.OrgID, sess.ID, "test.live", payload); err != nil {
		t.Fatalf("Emit live: %v", err)
	}
	liveEnv := readEnvelope(t, readCtx, c)
	if got := liveEnv["type"]; got != "test.live" {
		t.Errorf("live event type: want test.live, got %v", got)
	}
}

// TestHandler_SlowConsumer_ClosedWith1008 verifies that a client whose send
// buffer fills up is closed by the server with status 1008 (policy violation).
func TestHandler_SlowConsumer_ClosedWith1008(t *testing.T) {
	tenv := newTestEnv(t)
	acc, sess := seed(t, tenv.store)
	ticket := issueTicket(t, tenv.tickets, &acc)

	c, _ := dialWSWithTicket(t, wsURL(tenv.srv, sess.ID), ticket)
	defer c.CloseNow()

	ctx := context.Background()

	// Emit events without reading from the client. We need to fill c.send
	// (buffer=64) and have a 65th event arrive at the gateway.fanout so it
	// triggers the slow-consumer close.
	//
	// Strategy: emit 64 events in batches, yielding to the goroutine scheduler
	// between each batch so the gateway.fanout can drain the log-subscriber
	// channel into c.send. Once c.send is full (64 events), emit one more event
	// to trigger the overflow-close.
	for i := 0; i < 64; i++ {
		payload := mustMarshal(t, map[string]int{"n": i})
		if _, err := tenv.log.Emit(ctx, sess.OrgID, sess.ID, "flood.event", payload); err != nil {
			t.Fatalf("Emit %d: %v", i, err)
		}
	}
	// Allow the gateway.fanout goroutine to process the 64 events from the
	// log-subscriber channel into c.send.
	time.Sleep(50 * time.Millisecond)

	// Now c.send should be full. Emit one more event to trigger the close.
	payload := mustMarshal(t, map[string]int{"n": 64})
	if _, err := tenv.log.Emit(ctx, sess.OrgID, sess.ID, "flood.event", payload); err != nil {
		t.Fatalf("Emit overflow: %v", err)
	}

	// Give the server a moment to process the close.
	time.Sleep(50 * time.Millisecond)

	// Read until we see the close; verify it's status 1008.
	checkCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	var closedWith1008 bool
	for {
		_, _, err := c.Read(checkCtx)
		if err != nil {
			var ce websocket.CloseError
			if errors.As(err, &ce) && ce.Code == websocket.StatusPolicyViolation {
				closedWith1008 = true
			}
			break
		}
	}

	if !closedWith1008 {
		t.Error("expected server to close with status 1008 (policy violation) for slow consumer")
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func mustMarshal(t *testing.T, v any) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("mustMarshal: %v", err)
	}
	return json.RawMessage(b)
}
