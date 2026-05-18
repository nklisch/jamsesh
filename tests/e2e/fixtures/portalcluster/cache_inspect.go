package portalcluster

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

const (
	// defaultStoragePath is the value set by portal.go's buildEnv for
	// JAMSESH_STORAGE. Tests that need deterministic cache inspection should
	// pass JAMSESH_STORAGE="/tmp/jamsesh-repos" via PortalExtraEnv (the
	// fixture already sets this as the default, but callers must not assume
	// it if they override ExtraEnv).
	//
	// Repo path on container FS:
	//   <storagePath>/orgs/<orgID>/sessions/<sessionID>.git
	defaultStoragePath = "/tmp/jamsesh-repos"
)

// VerifyCacheEvicted asserts that the pod's local bare-repo cache for
// sessionID has been removed from the container's filesystem after eviction.
//
// Implementation: runs `ls <repoPath>` inside the container using
// testcontainers' Exec API. If the directory is absent the command exits
// non-zero; that exit code is treated as "evicted". A zero exit code means
// the directory still exists, which is a test failure.
//
// storagePath is the JAMSESH_STORAGE value for the pod's container. Pass ""
// to use the default ("/tmp/jamsesh-repos"). Tests that override
// JAMSESH_STORAGE via PortalExtraEnv must supply the overriding value here so
// the path computation is correct.
//
// Callers that need eviction inspection should ensure the container's
// JAMSESH_STORAGE is deterministic by passing it explicitly in
// PortalExtraEnv — see cluster.go Options.PortalExtraEnv.
func (c *Cluster) VerifyCacheEvicted(
	ctx context.Context, t *testing.T,
	orgID, sessionID string,
	podIndex int,
	storagePath string,
) {
	t.Helper()

	if podIndex < 0 || podIndex >= len(c.Pods) {
		t.Fatalf("VerifyCacheEvicted: podIndex %d out of range (cluster has %d pods)", podIndex, len(c.Pods))
	}
	if storagePath == "" {
		storagePath = defaultStoragePath
	}

	repoPath := containerRepoPath(storagePath, orgID, sessionID)
	exitCode, output, err := c.Pods[podIndex].Exec(ctx, []string{"ls", repoPath})
	if err != nil {
		t.Fatalf("VerifyCacheEvicted: pod %d: exec ls %s: %v", podIndex, repoPath, err)
	}

	if exitCode == 0 {
		t.Fatalf("VerifyCacheEvicted: pod %d: cache NOT evicted — %s still exists (ls output: %s)",
			podIndex, repoPath, strings.TrimSpace(stripDockerMux(output)))
	}
	// Non-zero exit = directory absent = cache evicted. Pass.
	t.Logf("VerifyCacheEvicted: pod %d: cache evicted — %s is absent (exit code %d)", podIndex, repoPath, exitCode)
}

// VerifyCachePresent asserts that the pod's local bare-repo cache for
// sessionID EXISTS on the container's filesystem. This is the companion check
// to VerifyCacheEvicted — run it before triggering eviction to guard against
// false-positives (i.e. to confirm the cache was ever populated in the first
// place).
//
// storagePath follows the same convention as VerifyCacheEvicted.
func (c *Cluster) VerifyCachePresent(
	ctx context.Context, t *testing.T,
	orgID, sessionID string,
	podIndex int,
	storagePath string,
) {
	t.Helper()

	if podIndex < 0 || podIndex >= len(c.Pods) {
		t.Fatalf("VerifyCachePresent: podIndex %d out of range (cluster has %d pods)", podIndex, len(c.Pods))
	}
	if storagePath == "" {
		storagePath = defaultStoragePath
	}

	repoPath := containerRepoPath(storagePath, orgID, sessionID)
	exitCode, output, err := c.Pods[podIndex].Exec(ctx, []string{"ls", repoPath})
	if err != nil {
		t.Fatalf("VerifyCachePresent: pod %d: exec ls %s: %v", podIndex, repoPath, err)
	}

	if exitCode != 0 {
		t.Fatalf("VerifyCachePresent: pod %d: cache NOT present — %s absent (exit code %d; output: %s)",
			podIndex, repoPath, exitCode, strings.TrimSpace(stripDockerMux(output)))
	}
	t.Logf("VerifyCachePresent: pod %d: cache present — %s exists", podIndex, repoPath)
}

// CacheExists is the non-fatal variant of VerifyCachePresent. It returns true
// if the repo directory exists on the pod, false otherwise. Use this in
// polling loops (e.g. waiting for hydration to populate the cache) or in
// tests that branch on cache state without failing immediately.
//
// Returns (false, error) on Docker API failures. Returns (false, nil) when
// the directory is absent. Returns (true, nil) when present.
func (c *Cluster) CacheExists(
	ctx context.Context,
	orgID, sessionID string,
	podIndex int,
	storagePath string,
) (bool, error) {
	if podIndex < 0 || podIndex >= len(c.Pods) {
		return false, fmt.Errorf("CacheExists: podIndex %d out of range (cluster has %d pods)", podIndex, len(c.Pods))
	}
	if storagePath == "" {
		storagePath = defaultStoragePath
	}

	repoPath := containerRepoPath(storagePath, orgID, sessionID)
	exitCode, _, err := c.Pods[podIndex].Exec(ctx, []string{"ls", repoPath})
	if err != nil {
		return false, fmt.Errorf("CacheExists: pod %d: exec ls %s: %w", podIndex, repoPath, err)
	}
	return exitCode == 0, nil
}

// ---------------------------------------------------------------------------
// internal helpers
// ---------------------------------------------------------------------------

// containerRepoPath mirrors the storage.RepoPath logic from
// internal/portal/storage/paths.go for the container's filesystem:
//
//	<storagePath>/orgs/<orgID>/sessions/<sessionID>.git
func containerRepoPath(storagePath, orgID, sessionID string) string {
	return storagePath + "/orgs/" + orgID + "/sessions/" + sessionID + ".git"
}

// stripDockerMux removes the 8-byte Docker multiplexer header that
// testcontainers prepends to exec output when reading from the combined
// stdout+stderr stream. The header is non-printable and confuses log output.
// This is a best-effort strip — if the output is shorter than 8 bytes or
// doesn't start with a mux header, the input is returned unchanged.
//
// Docker mux frame format: [stream_type(1)] [0(3)] [size(4)] [payload...]
// stream_type: 1=stdout, 2=stderr.
func stripDockerMux(s string) string {
	// Strip leading frames as long as the string starts with a valid mux
	// header (stream type 1 or 2, three zero bytes).
	for len(s) >= 8 && (s[0] == 1 || s[0] == 2) && s[1] == 0 && s[2] == 0 && s[3] == 0 {
		frameSize := int(s[4])<<24 | int(s[5])<<16 | int(s[6])<<8 | int(s[7])
		if frameSize < 0 || 8+frameSize > len(s) {
			break
		}
		s = s[8 : 8+frameSize]
	}
	return s
}
