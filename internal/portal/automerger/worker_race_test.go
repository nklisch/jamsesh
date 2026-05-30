package automerger_test

// worker_race_test.go — probabilistic stress test for the lost-event race fix.
//
// This test runs many sessions with a tiny IdleTimeout and verifies that every
// emitted commit.arrived event produces an outcome event (merge.succeeded or
// conflict.detected). Under the old two-sync.Map design this would regularly
// strand events; under the new single-mu-guarded sessions map it must not.
//
// The deterministic onIdleDecision-based test is in
// worker_race_internal_test.go (package automerger), which can access the
// unexported field.

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-git/go-git/v5/plumbing"

	"jamsesh/internal/db/store"
	"jamsesh/internal/portal/automerger"
	"jamsesh/internal/portal/events"
	"jamsesh/internal/portal/storage"
)

// raceMultiStorage satisfies storage.Service and routes each (orgID, sessID)
// to the correct repoDir.
type raceMultiStorage struct {
	repos map[string]string // key: orgID+":"+sessID → repoDir
}

func (ms *raceMultiStorage) RepoPath(orgID, sessID string) string {
	return ms.repos[orgID+":"+sessID]
}
func (ms *raceMultiStorage) CreateRepo(_ context.Context, _, _ string) error { return nil }
func (ms *raceMultiStorage) RemoveRepo(_ context.Context, _, _ string) error { return nil }
func (ms *raceMultiStorage) RepoExists(_, _ string) (bool, error)            { return true, nil }
func (ms *raceMultiStorage) ArchiveSession(_ context.Context, _, _ string, _ storage.ArchiveInfo) error {
	return nil
}
func (ms *raceMultiStorage) LookupArchived(_ context.Context, _, _ string) (*storage.ArchivedRecord, error) {
	return nil, nil
}
func (ms *raceMultiStorage) StubResponse(_ *storage.ArchivedRecord) storage.ArchivedStub {
	return storage.ArchivedStub{}
}

var _ storage.Service = (*raceMultiStorage)(nil)

// raceSession holds the per-session state for a stress test.
type raceSession struct {
	sess    store.Session
	repoDir string
	source  plumbing.Hash
}

// setupRaceSessions builds n sessions each with a clean-merge repo topology.
func setupRaceSessions(t *testing.T, s store.Store, n int) ([]*raceSession, *raceMultiStorage) {
	t.Helper()
	ctx := context.Background()
	ms := &raceMultiStorage{repos: make(map[string]string)}
	var sessions []*raceSession

	for i := range n {
		orgID := fmt.Sprintf("org-race-%d", i)
		sessID := fmt.Sprintf("sess-race-%d", i)

		if _, err := s.CreateOrg(ctx, store.CreateOrgParams{
			ID:        orgID,
			Name:      orgID,
			Slug:      orgID,
			CreatedAt: time.Now().UTC(),
		}); err != nil {
			t.Fatalf("CreateOrg %s: %v", orgID, err)
		}
		sess, err := s.CreateSession(ctx, store.CreateSessionParams{
			ID:            sessID,
			OrgID:         orgID,
			Name:          sessID,
			Goal:          "race-test",
			WritableScope: `["**"]`,
			DefaultMode:   "sync",
			Status:        "active",
			CreatedAt:     time.Now().UTC(),
		})
		if err != nil {
			t.Fatalf("CreateSession %s: %v", sessID, err)
		}
		if err := s.EnsureEventSeqRow(ctx, sessID); err != nil {
			t.Fatalf("EnsureEventSeqRow %s: %v", sessID, err)
		}

		repo, repoDir := initRepo(t)
		ancestor := commitFiles(t, repo, repoDir, nil, map[string][]byte{"f.txt": []byte("base\n")}, "base")
		draftTip := commitFiles(t, repo, repoDir, ancestor, map[string][]byte{
			"f.txt": []byte("base\n"), "a.txt": []byte("draft\n"),
		}, "draft")
		source := commitFiles(t, repo, repoDir, ancestor, map[string][]byte{
			"f.txt": []byte("base\n"), "b.txt": []byte("source\n"),
		}, "source")

		draftRef := plumbing.NewBranchReferenceName("jam/" + sessID + "/draft")
		if err := repo.Storer.SetReference(plumbing.NewHashReference(draftRef, draftTip.Hash)); err != nil {
			t.Fatalf("set draft ref %s: %v", sessID, err)
		}

		ms.repos[orgID+":"+sessID] = repoDir
		sessions = append(sessions, &raceSession{
			sess:    sess,
			repoDir: repoDir,
			source:  source.Hash,
		})
	}
	return sessions, ms
}

// TestWorkerRace_NoStrandedEvents runs many sessions with a very short
// IdleTimeout under the race detector and verifies that every commit.arrived
// event produces an outcome (merge.succeeded or conflict.detected). This
// catches regressions of the original two-sync.Map lost-event race.
func TestWorkerRace_NoStrandedEvents(t *testing.T) {
	const numSessions = 4
	const eventsPerSession = 10

	s := openWorkerStore(t)
	log := events.New(s)
	sessions, ms := setupRaceSessions(t, s, numSessions)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	applier := automerger.NewApplier(s, log)
	w := &automerger.Worker{
		Store:       s,
		Storage:     ms,
		Log:         log,
		Applier:     applier,
		PortalHost:  "test.jamsesh.local",
		IdleTimeout: 1 * time.Millisecond, // tiny timeout to maximise idle-race
		QueueSize:   256,
	}

	if err := w.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}

	outcomeCh, unsub := log.Subscribe("")
	defer unsub()

	var outcomeCount atomic.Int64
	go func() {
		for e := range outcomeCh {
			switch e.Type {
			case "merge.succeeded", "conflict.detected":
				outcomeCount.Add(1)
			}
		}
	}()

	total := int64(numSessions * eventsPerSession)
	for _, rs := range sessions {
		for range eventsPerSession {
			payload := buildCommitArrivedPayload(t,
				"refs/heads/jam/"+rs.sess.ID+"/alice/feat",
				rs.source.String(),
			)
			if _, err := log.Emit(ctx, rs.sess.OrgID, rs.sess.ID, "commit.arrived", payload); err != nil {
				t.Fatalf("Emit: %v", err)
			}
		}
	}

	// Wait up to 30s for all outcomes.
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		if outcomeCount.Load() >= total {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}

	if got := outcomeCount.Load(); got < total {
		t.Errorf("stranded events: got %d outcomes, want %d"+
			" (each commit.arrived must produce an outcome event)",
			got, total)
	}
}

