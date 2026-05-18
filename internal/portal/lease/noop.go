package lease

import (
	"context"
	"sync"
)

// NoopManager is the single-instance compatibility shim. Acquire never
// blocks, never returns [ErrAlreadyHeld], and the returned [Handle]'s
// FencingToken is always 0. Lost() never closes until Release() is called.
//
// Selected automatically when JAMSESH_DEPLOY_MODE is "single" (the default).
// Keeps call-site code identical across single and clustered modes.
type NoopManager struct{}

// Acquire always succeeds and returns a [Handle] whose FencingToken is 0.
// The context is respected for cancellation before the Handle is returned,
// but no I/O is performed.
func (NoopManager) Acquire(ctx context.Context, sessionID string) (Handle, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return &noopHandle{
		sessionID: sessionID,
		lost:      make(chan struct{}),
	}, nil
}

// noopHandle is the Handle returned by [NoopManager.Acquire].
type noopHandle struct {
	sessionID string
	once      sync.Once
	lost      chan struct{}
}

func (h *noopHandle) SessionID() string   { return h.sessionID }
func (h *noopHandle) FencingToken() int64 { return 0 }
func (h *noopHandle) Lost() <-chan struct{} { return h.lost }

// Release closes Lost() exactly once. Safe to call multiple times.
func (h *noopHandle) Release() error {
	h.once.Do(func() { close(h.lost) })
	return nil
}
