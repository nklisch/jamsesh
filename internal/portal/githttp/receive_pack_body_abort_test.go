package githttp_test

// receive_pack_body_abort_test.go tests the client-abort classification in the
// receive_pack body-read path.
//
// When io.Copy drains the request body and the client disconnects mid-upload,
// the read returns an error AND r.Context().Err() != nil.  The handler must
// return 499 (client closed request) rather than 413 (request entity too large).
//
// A genuine size-limit error (MaxBytesReader cap exceeded) on a live context
// must still return 413 unchanged.

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"jamsesh/internal/db"
	"jamsesh/internal/db/store"
	"jamsesh/internal/portal/githttp"
	"jamsesh/internal/portal/prereceive"
	"jamsesh/internal/portal/storage"
	"jamsesh/internal/portal/tokens"

	"github.com/go-chi/chi/v5"
)

// contextCancellingReader wraps an io.Reader and cancels a context when Read
// is called after the underlying reader returns EOF (or always after one read,
// depending on mode). Here it cancels the context on the read, simulating a
// client that disconnects mid-upload. The read returns an error so the handler
// sees a body-read failure with r.Context().Err() != nil.
type contextCancellingReader struct {
	cancel  context.CancelFunc
	readErr error // error to return after cancelling
	called  bool
}

func (r *contextCancellingReader) Read(p []byte) (int, error) {
	if !r.called {
		r.called = true
		r.cancel()         // cancel the context to simulate client disconnect
		return 0, r.readErr // return the body-read error
	}
	return 0, io.EOF
}

// buildReceivePackAbortEnv creates a Handler + router for body-abort tests.
// Returns a cancel-able context, the router, and credentials.
func buildReceivePackAbortEnv(t *testing.T) (ctx context.Context, cancel context.CancelFunc, router *chi.Mux, orgID, sessionID, token string) {
	t.Helper()
	s, _, err := db.Open(context.Background(), "sqlite", ":memory:", db.PoolConfig{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })

	acc, err := s.CreateAccount(context.Background(), store.CreateAccountParams{
		ID: nextID("rba-acc"), Email: "rba@example.com", DisplayName: "rba",
		CreatedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("CreateAccount: %v", err)
	}
	tokenSvc := tokens.New(s)
	pair, err := tokenSvc.Issue(context.Background(), acc.ID)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}

	orgID = nextID("rba-org")
	if _, err := s.CreateOrg(context.Background(), store.CreateOrgParams{
		ID: orgID, Name: "RBA Org", Slug: orgID, CreatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("CreateOrg: %v", err)
	}
	sessionID = nextID("rba-sess")
	if _, err := s.CreateSession(context.Background(), store.CreateSessionParams{
		ID: sessionID, OrgID: orgID, Name: "RBA Session", Goal: "rba",
		WritableScope: `["**"]`, DefaultMode: "sync", Status: "active",
		CreatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	if err := s.AddSessionMember(context.Background(), store.AddSessionMemberParams{
		SessionID: sessionID, OrgID: orgID, AccountID: acc.ID,
		Role: "member", JoinedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("AddSessionMember: %v", err)
	}

	storageSvc := storage.New(t.TempDir(), s)
	h := &githttp.Handler{
		Store:     s,
		Tokens:    tokenSvc,
		Storage:   storageSvc,
		Validator: &prereceive.Validator{MaxPackBytes: 50 * 1024 * 1024},
	}
	router = chi.NewRouter()
	h.Mount(router)

	ctx, cancel = context.WithCancel(context.Background())
	return ctx, cancel, router, orgID, sessionID, pair.AccessToken
}

// TestReceivePack_BodyReadError_CancelledContext_Returns499 verifies that when
// the request body read fails AND the request context is cancelled
// (client disconnected mid-upload), receivePack returns 499 rather than 413.
//
// The body reader cancels the context on its first Read call, simulating a
// client that disconnects during the body stream. The context is live during
// auth so auth passes normally; the body-read failure then triggers the
// client-abort branch in the handler.
func TestReceivePack_BodyReadError_CancelledContext_Returns499(t *testing.T) {
	ctx, cancel, router, orgID, sessionID, token := buildReceivePackAbortEnv(t)
	defer cancel()

	body := &contextCancellingReader{
		cancel:  cancel, // cancel the ctx when body read is attempted
		readErr: context.Canceled,
	}

	reqURL := "/" + orgID + "/" + sessionID + ".git/git-receive-pack"
	req := httptest.NewRequest(http.MethodPost, reqURL, body)
	req.Header.Set("Authorization", basicAuthHeader("x-access-token", token))
	req.Header.Set("Content-Type", "application/x-git-receive-pack-request")
	req = req.WithContext(ctx)

	rw := httptest.NewRecorder()
	router.ServeHTTP(rw, req)

	// Must NOT be 413 (size limit) when the client is the one who disconnected.
	if rw.Code == http.StatusRequestEntityTooLarge {
		t.Errorf("want non-413 for client-abort body read, got 413")
	}
	// The correct code is 499 (client closed request).
	if rw.Code != 499 {
		t.Errorf("want 499 (client closed request) for cancelled-ctx body read, got %d", rw.Code)
	}
}

// TestReceivePack_BodyReadError_LiveContext_Returns413 verifies that a genuine
// body-size limit error on a live context still returns 413 (not client abort).
// This guards against over-broad client-abort suppression.
func TestReceivePack_BodyReadError_LiveContext_Returns413(t *testing.T) {
	ctx, cancel, router, orgID, sessionID, token := buildReceivePackAbortEnv(t)
	defer cancel()

	// The handler wraps the body with MaxBytesReader(w, r.Body, maxBytes+16KiB).
	// MaxPackBytes is 50 MiB; the handler adds 16 KiB overhead. Send more than
	// that limit so MaxBytesReader caps and returns an error. Use a zero reader
	// so no RAM allocation is needed.
	const overLimit = 50*1024*1024 + 16*1024 + 1
	bigBody := io.LimitReader(zeroReader{}, overLimit)

	reqURL := "/" + orgID + "/" + sessionID + ".git/git-receive-pack"
	req := httptest.NewRequest(http.MethodPost, reqURL, bigBody)
	req.Header.Set("Authorization", basicAuthHeader("x-access-token", token))
	req.Header.Set("Content-Type", "application/x-git-receive-pack-request")
	req = req.WithContext(ctx) // live (not cancelled) context

	rw := httptest.NewRecorder()
	router.ServeHTTP(rw, req)

	if rw.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("want 413 for genuine size-limit on live ctx, got %d", rw.Code)
	}
}

// zeroReader is an io.Reader that returns infinite zero bytes.
type zeroReader struct{}

func (zeroReader) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = 0
	}
	return len(p), nil
}
