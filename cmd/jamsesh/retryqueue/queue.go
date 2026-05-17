// Package retryqueue implements a file-backed, per-session FIFO of commit
// SHAs that failed to push due to transient errors. It is consumed by
// user-prompt-submit (drain) and produced by post-tool-use (enqueue).
//
// Concurrency note: CC fires hooks serially within a session, so no file
// locking is needed in v1. This is documented rather than silent.
package retryqueue

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"jamsesh/cmd/jamsesh/state"
)

// Entry describes a single commit that is queued for a retry push.
type Entry struct {
	CommitSHA   string    `json:"commit_sha"`
	Attempts    int       `json:"attempts"`
	LastErrorAt time.Time `json:"last_error_at"`
	LastError   string    `json:"last_error"`
}

// Queue is a file-backed FIFO for a single session. SessionID must be set
// before any method is called.
type Queue struct {
	SessionID string
}

// Path returns the absolute path of the queue JSON file.
// The path is ${CLAUDE_PLUGIN_DATA}/sessions/<sessionID>/retry-queue.json.
func (q *Queue) Path() (string, error) {
	dir, err := state.PluginDataDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "sessions", q.SessionID, "retry-queue.json"), nil
}

// Load reads the queue from disk. If the file does not yet exist an empty
// slice is returned without error (the queue is logically empty).
func (q *Queue) Load() ([]Entry, error) {
	p, err := q.Path()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(p)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return []Entry{}, nil
		}
		return nil, fmt.Errorf("retryqueue: reading %q: %w", p, err)
	}
	var entries []Entry
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, fmt.Errorf("retryqueue: decoding %q: %w", p, err)
	}
	return entries, nil
}

// Save atomically writes entries to the queue file. The parent directory is
// created if it does not exist. The file is written at mode 0600.
func (q *Queue) Save(entries []Entry) error {
	p, err := q.Path()
	if err != nil {
		return err
	}

	// Ensure the directory exists.
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return fmt.Errorf("retryqueue: creating session dir: %w", err)
	}

	data, err := json.Marshal(entries)
	if err != nil {
		return fmt.Errorf("retryqueue: encoding entries: %w", err)
	}

	// Atomic write: temp file in the same directory, then rename.
	dir := filepath.Dir(p)
	tmp, err := os.CreateTemp(dir, ".retryqueue-write-*")
	if err != nil {
		return fmt.Errorf("retryqueue: creating temp file: %w", err)
	}
	tmpName := tmp.Name()

	success := false
	defer func() {
		if !success {
			_ = os.Remove(tmpName)
		}
	}()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("retryqueue: writing temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("retryqueue: closing temp file: %w", err)
	}
	if err := os.Chmod(tmpName, 0o600); err != nil {
		return fmt.Errorf("retryqueue: chmod temp file: %w", err)
	}
	if err := os.Rename(tmpName, p); err != nil {
		return fmt.Errorf("retryqueue: renaming temp file: %w", err)
	}
	success = true
	return nil
}

// Enqueue appends e to the tail of the queue (Load → append → Save).
func (q *Queue) Enqueue(e Entry) error {
	entries, err := q.Load()
	if err != nil {
		return err
	}
	entries = append(entries, e)
	return q.Save(entries)
}

// Drain returns all entries and atomically clears the queue (Load → save
// empty → return). The caller receives the full snapshot; subsequent Loads
// return an empty slice.
func (q *Queue) Drain() ([]Entry, error) {
	entries, err := q.Load()
	if err != nil {
		return nil, err
	}
	if err := q.Save([]Entry{}); err != nil {
		return nil, err
	}
	return entries, nil
}

// Size returns the number of entries currently in the queue.
func (q *Queue) Size() (int, error) {
	entries, err := q.Load()
	if err != nil {
		return 0, err
	}
	return len(entries), nil
}
