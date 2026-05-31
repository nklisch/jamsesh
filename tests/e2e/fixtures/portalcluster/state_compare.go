package portalcluster

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"testing"
)

// SessionState is the subset of session state that the handoff tests care
// about for cross-pod comparison. All fields are pod-local reads — no routing
// layer is involved.
type SessionState struct {
	// Status is the session lifecycle status (e.g. "active", "finalizing",
	// "ended").
	Status string `json:"status"`

	// EndedAt is non-empty when the session has ended (finalized, abandoned, or
	// shipped). Used to detect divergence in finalize state across pods.
	EndedAt string `json:"ended_at,omitempty"`

	// EndReason records why the session ended; empty while active.
	EndReason string `json:"end_reason,omitempty"`

	// Refs is the map of ref → tip SHA for all jam/* refs in the session's
	// bare repo, as reported by GET .../refs on the pod. This is the primary
	// durability invariant: both pods must agree on every ref tip after
	// hydration.
	Refs map[string]string
}

// sessionAPIResponse is the minimal shape of GET /api/orgs/{orgID}/sessions/{sessionID}.
type sessionAPIResponse struct {
	Status    string `json:"status"`
	EndedAt   string `json:"ended_at,omitempty"`
	EndReason string `json:"end_reason,omitempty"`
}

// refListAPIResponse is the minimal shape of GET .../refs.
type refListAPIResponse struct {
	Refs []refEntry `json:"refs"`
}

type refEntry struct {
	Ref string `json:"ref"`
	Sha string `json:"sha"`
}

// StateDiff records what diverged between two pods' view of a session. An
// empty StateDiff (no populated fields) means the pods agree.
type StateDiff struct {
	// StatusMismatch is set when the two pods report different session statuses.
	StatusMismatch string

	// EndedAtMismatch is set when ended_at differs between pods.
	EndedAtMismatch string

	// RefDiffs lists refs where the tip SHAs differ or where a ref is present
	// on one pod but not the other.
	RefDiffs []RefDiff
}

// RefDiff describes a single ref that diverged between two pods.
type RefDiff struct {
	Ref     string
	PodAsha string
	PodBsha string
}

// Empty reports whether all comparison fields are unset, meaning the pods
// are in agreement.
func (d StateDiff) Empty() bool {
	return d.StatusMismatch == "" &&
		d.EndedAtMismatch == "" &&
		len(d.RefDiffs) == 0
}

// String returns a human-readable description of the diff. Returns "<no diff>"
// when Empty.
func (d StateDiff) String() string {
	if d.Empty() {
		return "<no diff>"
	}
	var sb strings.Builder
	if d.StatusMismatch != "" {
		sb.WriteString("status: " + d.StatusMismatch + "\n")
	}
	if d.EndedAtMismatch != "" {
		sb.WriteString("ended_at: " + d.EndedAtMismatch + "\n")
	}
	for _, rd := range d.RefDiffs {
		sb.WriteString(fmt.Sprintf("ref %s: podA=%s podB=%s\n", rd.Ref, rd.PodAsha, rd.PodBsha))
	}
	return sb.String()
}

// CompareSessionState reads the session state from two pods (by index in
// c.Pods) and returns a structured diff. The accessToken must be valid for
// both pods (they share Postgres, so any token issued by the cluster works).
//
// The comparison covers:
//   - session status (from GET /api/orgs/{orgID}/sessions/{sessionID})
//   - ended_at / end_reason (finalize-state invariant)
//   - all jam/* ref tip SHAs (from GET .../refs) — the primary durability invariant
//
// Both pods are queried directly (not through the router) so each response
// is pod-local and unaffected by routing-layer caching.
func (c *Cluster) CompareSessionState(
	ctx context.Context,
	orgID, sessionID, accessToken string,
	podA, podB int,
) (StateDiff, error) {
	if podA < 0 || podA >= len(c.Pods) {
		return StateDiff{}, fmt.Errorf("CompareSessionState: podA %d out of range (cluster has %d pods)", podA, len(c.Pods))
	}
	if podB < 0 || podB >= len(c.Pods) {
		return StateDiff{}, fmt.Errorf("CompareSessionState: podB %d out of range (cluster has %d pods)", podB, len(c.Pods))
	}

	stateA, err := fetchSessionState(ctx, c.Pods[podA].URL, orgID, sessionID, accessToken)
	if err != nil {
		return StateDiff{}, fmt.Errorf("CompareSessionState: pod %d: %w", podA, err)
	}

	stateB, err := fetchSessionState(ctx, c.Pods[podB].URL, orgID, sessionID, accessToken)
	if err != nil {
		return StateDiff{}, fmt.Errorf("CompareSessionState: pod %d: %w", podB, err)
	}

	return diffStates(stateA, stateB), nil
}

// RequireSessionStateMatch wraps CompareSessionState and calls t.Fatal if the
// two pods disagree on any aspect of the session state.
//
// This is the primary handoff-invariant assertion: after a pod drain and
// hydration on the replacement pod, both pods (the drained one's last-known
// state and the new holder's state) must match. A draft-tip divergence is a
// Critical durability bug.
func (c *Cluster) RequireSessionStateMatch(
	ctx context.Context, t *testing.T,
	orgID, sessionID, accessToken string,
	podA, podB int,
) {
	t.Helper()
	diff, err := c.CompareSessionState(ctx, orgID, sessionID, accessToken, podA, podB)
	if err != nil {
		t.Fatalf("RequireSessionStateMatch: compare failed: %v", err)
	}
	if !diff.Empty() {
		t.Fatalf("RequireSessionStateMatch: pods %d and %d disagree on session %s:\n%s",
			podA, podB, sessionID, diff)
	}
}

// ---------------------------------------------------------------------------
// internal helpers
// ---------------------------------------------------------------------------

// fetchSessionState queries a pod's REST API for session state and refs.
func fetchSessionState(ctx context.Context, podURL, orgID, sessionID, accessToken string) (SessionState, error) {
	// 1. GET session metadata.
	sessResp, err := getJSON[sessionAPIResponse](ctx, podURL+"/api/orgs/"+orgID+"/sessions/"+sessionID, accessToken)
	if err != nil {
		return SessionState{}, fmt.Errorf("get session: %w", err)
	}

	// 2. GET ref list (the primary durability signal).
	refsResp, err := getJSON[refListAPIResponse](ctx, podURL+"/api/orgs/"+orgID+"/sessions/"+sessionID+"/refs", accessToken)
	if err != nil {
		return SessionState{}, fmt.Errorf("get refs: %w", err)
	}

	// The REST /refs API reports the FULL ref name ("refs/heads/jam/...", from
	// ListSessionRefs -> r.Name().String()). Normalize to the SHORT push form
	// ("jam/<sid>/<uid>/main") so the Refs map keys match the form the e2e suite
	// uses elsewhere (gitclient.Push / RevParse), keeping cross-pod diffs and any
	// short-ref lookups consistent.
	refs := make(map[string]string, len(refsResp.Refs))
	for _, r := range refsResp.Refs {
		refs[strings.TrimPrefix(r.Ref, "refs/heads/")] = r.Sha
	}

	return SessionState{
		Status:    sessResp.Status,
		EndedAt:   sessResp.EndedAt,
		EndReason: sessResp.EndReason,
		Refs:      refs,
	}, nil
}

// getJSON performs a GET request with Bearer auth and decodes the JSON body
// into T. Returns an error if the status is not 200 or decoding fails.
func getJSON[T any](ctx context.Context, url, accessToken string) (T, error) {
	var zero T
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return zero, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return zero, fmt.Errorf("GET %s: %w", url, err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return zero, fmt.Errorf("GET %s: unexpected status %d; body: %s", url, resp.StatusCode, body)
	}

	var result T
	if err := json.Unmarshal(body, &result); err != nil {
		return zero, fmt.Errorf("decode %s response: %w", url, err)
	}
	return result, nil
}

// diffStates computes the diff between two SessionState values.
func diffStates(a, b SessionState) StateDiff {
	var diff StateDiff

	if a.Status != b.Status {
		diff.StatusMismatch = fmt.Sprintf("podA=%q podB=%q", a.Status, b.Status)
	}
	if a.EndedAt != b.EndedAt {
		diff.EndedAtMismatch = fmt.Sprintf("podA=%q podB=%q", a.EndedAt, b.EndedAt)
	}

	// Build the union of all refs from both pods.
	allRefs := map[string]struct{}{}
	for ref := range a.Refs {
		allRefs[ref] = struct{}{}
	}
	for ref := range b.Refs {
		allRefs[ref] = struct{}{}
	}

	// Sort for deterministic output.
	sorted := make([]string, 0, len(allRefs))
	for ref := range allRefs {
		sorted = append(sorted, ref)
	}
	sort.Strings(sorted)

	for _, ref := range sorted {
		shaA := a.Refs[ref]
		shaB := b.Refs[ref]
		if shaA != shaB {
			diff.RefDiffs = append(diff.RefDiffs, RefDiff{
				Ref:     ref,
				PodAsha: shaA,
				PodBsha: shaB,
			})
		}
	}

	return diff
}
