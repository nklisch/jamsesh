package retryqueue_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"jamsesh/cmd/jamsesh/retryqueue"
)

// withPluginData sets CLAUDE_PLUGIN_DATA for the duration of the test.
func withPluginData(t *testing.T, dir string) {
	t.Helper()
	t.Setenv("CLAUDE_PLUGIN_DATA", dir)
}

func newQueue(t *testing.T, dir string) *retryqueue.Queue {
	t.Helper()
	withPluginData(t, dir)
	return &retryqueue.Queue{SessionID: "test-session-abc"}
}

// TestQueue_emptyLoad verifies that Load on a missing file returns empty slice.
func TestQueue_emptyLoad(t *testing.T) {
	q := newQueue(t, t.TempDir())
	entries, err := q.Load()
	if err != nil {
		t.Fatalf("Load on missing file: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("Load() len = %d, want 0", len(entries))
	}
}

// TestQueue_roundTrip verifies enqueue-3 then load returns 3 entries.
func TestQueue_roundTrip(t *testing.T) {
	q := newQueue(t, t.TempDir())

	now := time.Now().Truncate(time.Second)
	for i := range 3 {
		e := retryqueue.Entry{
			CommitSHA:   "sha" + string(rune('a'+i)),
			Attempts:    i + 1,
			LastErrorAt: now,
			LastError:   "transient error",
		}
		if err := q.Enqueue(e); err != nil {
			t.Fatalf("Enqueue %d: %v", i, err)
		}
	}

	entries, err := q.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(entries) != 3 {
		t.Errorf("Load() len = %d, want 3", len(entries))
	}

	// Verify order (FIFO).
	for i, e := range entries {
		wantSHA := "sha" + string(rune('a'+i))
		if e.CommitSHA != wantSHA {
			t.Errorf("entries[%d].CommitSHA = %q, want %q", i, e.CommitSHA, wantSHA)
		}
	}
}

// TestQueue_drain verifies that Drain returns all entries and clears the queue.
func TestQueue_drain(t *testing.T) {
	q := newQueue(t, t.TempDir())

	for i := range 3 {
		if err := q.Enqueue(retryqueue.Entry{CommitSHA: "sha" + string(rune('0'+i))}); err != nil {
			t.Fatalf("Enqueue %d: %v", i, err)
		}
	}

	drained, err := q.Drain()
	if err != nil {
		t.Fatalf("Drain: %v", err)
	}
	if len(drained) != 3 {
		t.Errorf("Drain() len = %d, want 3", len(drained))
	}

	// After drain the queue must be empty.
	remaining, err := q.Load()
	if err != nil {
		t.Fatalf("Load after Drain: %v", err)
	}
	if len(remaining) != 0 {
		t.Errorf("Load after Drain: len = %d, want 0", len(remaining))
	}
}

// TestQueue_size verifies Size reports the correct count.
func TestQueue_size(t *testing.T) {
	q := newQueue(t, t.TempDir())

	n, err := q.Size()
	if err != nil {
		t.Fatalf("Size on empty queue: %v", err)
	}
	if n != 0 {
		t.Errorf("Size() = %d, want 0", n)
	}

	if err := q.Enqueue(retryqueue.Entry{CommitSHA: "abc"}); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	n, err = q.Size()
	if err != nil {
		t.Fatalf("Size after enqueue: %v", err)
	}
	if n != 1 {
		t.Errorf("Size() = %d, want 1", n)
	}
}

// TestQueue_atomicSave verifies that after Save the temp file is gone.
func TestQueue_atomicSave(t *testing.T) {
	dir := t.TempDir()
	q := newQueue(t, dir)

	if err := q.Enqueue(retryqueue.Entry{CommitSHA: "deadbeef"}); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	sessionDir := filepath.Join(dir, "sessions", "test-session-abc")
	entries, err := os.ReadDir(sessionDir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	for _, e := range entries {
		if e.Name() != "retry-queue.json" {
			t.Errorf("unexpected file after Save: %q", e.Name())
		}
	}
}

// TestQueue_fileMode verifies the queue file is written at mode 0600.
func TestQueue_fileMode(t *testing.T) {
	dir := t.TempDir()
	q := newQueue(t, dir)

	if err := q.Save([]retryqueue.Entry{{CommitSHA: "abc"}}); err != nil {
		t.Fatalf("Save: %v", err)
	}

	p, err := q.Path()
	if err != nil {
		t.Fatalf("Path: %v", err)
	}
	info, err := os.Stat(p)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Errorf("queue file mode = %04o, want 0600", got)
	}
}
