// Invariant: the pack manifest at sessions/<sessionID>/manifest.json in the
// MinIO bucket is a true description of the bucket state after every push.
// Every PackEntry in manifest.Packs has both its .pack and .idx objects
// present in the bucket. No dangling references, no missing entries.
//
// The manifest is read directly from MinIO via mn.GetObject; the portal API is
// never consulted (that would be tautological — we are testing the portal's
// own behavior, not asking it to describe itself).
package golden_test

import (
	"context"
	"encoding/json"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"

	"jamsesh/tests/e2e/fixtures/authflow"
	"jamsesh/tests/e2e/fixtures/gitclient"
	"jamsesh/tests/e2e/fixtures/mailhog"
	"jamsesh/tests/e2e/fixtures/minio"
	"jamsesh/tests/e2e/fixtures/portalcluster"
	"jamsesh/tests/e2e/fixtures/postgres"
)

// manifestJSON mirrors objectstore.Manifest from
// internal/portal/storage/objectstore/manifest.go.
// The JSON field tags are the stable contract; the struct is inlined here
// because the tests/e2e module cannot import portal internals across the
// module boundary. Keep field names and json tags in sync with the production
// struct (checked by build + vet rather than a shared import).
type manifestJSON struct {
	Version    int            `json:"version"`
	SessionID  string         `json:"session_id"`
	Packs      []packEntryJSON `json:"packs"`
	Refs       map[string]string `json:"refs"`
	PackedRefs string         `json:"packed_refs"`
}

// packEntryJSON mirrors objectstore.PackEntry.
type packEntryJSON struct {
	PackKey string `json:"pack_key"`
	IdxKey  string `json:"idx_key"`
	SHA     string `json:"sha"`
}

// manifestKey returns the object-storage key for the given session's manifest,
// matching the production formula: "sessions/<sessionID>/manifest.json".
func manifestKey(sessionID string) string {
	return "sessions/" + sessionID + "/manifest.json"
}

// TestObjectStoragePackManifest verifies the pack-manifest integrity invariant:
// the manifest is a true description of the MinIO bucket state after a push.
//
// Infrastructure starts once; each subtest creates its own user, org, and
// session so state is fully isolated without paying the Docker startup cost
// multiple times.
func TestObjectStoragePackManifest(t *testing.T) {
	ctx := context.Background()

	// ── Infrastructure ───────────────────────────────────────────────────────
	mn := minio.Start(ctx, t, minio.Options{})
	pg := postgres.Start(ctx, t, postgres.Options{})
	mh := mailhog.Start(ctx, t)

	cluster := portalcluster.Start(ctx, t, portalcluster.Options{
		Pods:        2,
		Postgres:    pg,
		ObjectStore: mn,
		Router:      false,
		PortalExtraEnv: map[string]string{
			"JAMSESH_EMAIL_PROVIDER":  "smtp",
			"JAMSESH_EMAIL_SMTP_HOST": mh.ContainerSMTPHost,
			"JAMSESH_EMAIL_SMTP_PORT": strconv.Itoa(mh.ContainerSMTPPort),
			"JAMSESH_EMAIL_SMTP_TLS":  "none",
		},
	})

	// All pushes go to pod 0 directly (Router: false).
	pod0 := cluster.Pods[0]

	// ── Subtest 1: manifest_matches_bucket_after_push ────────────────────────
	// After a push, read manifest.json directly from the bucket, unmarshal into
	// objectstore.Manifest, and assert every PackEntry's .pack and .idx objects
	// exist in the bucket. This is the non-tautological assertion: we compare
	// the manifest's claims against the actual objects in the bucket.
	t.Run("manifest_matches_bucket_after_push", func(t *testing.T) {
		userEmail := randEmail(t, "manifest-matches")
		pair := authflow.SignInViaMagicLink(ctx, t, pod0, mh, userEmail)
		userID := rpo0GetMe(ctx, t, pod0.URL, pair.AccessToken)
		orgID := authflow.CreateOrg(ctx, t, pod0, pair.AccessToken, "Manifest Matches Org")
		sessionID := rpo0CreateSession(ctx, t, pod0.URL, pair.AccessToken, orgID, "manifest-matches")

		repo := gitclient.Clone(ctx, t, pod0.URL, orgID, sessionID, userID, pair.AccessToken)
		ref := "jam/" + sessionID + "/" + userID + "/main"
		repo.Commit(ctx, t, "manifest-check.md", "content for manifest integrity test", "manifest: initial commit")
		repo.Push(ctx, t, ref)

		// Read the manifest directly from MinIO — NOT from the portal API.
		mKey := manifestKey(sessionID)
		manifestBytes, err := mn.GetObject(ctx, mKey)
		require.NoError(t, err,
			"manifest integrity: GetObject(%q) failed — manifest was not written to bucket after push",
			mKey)
		require.NotEmpty(t, manifestBytes,
			"manifest integrity: manifest.json at %q is empty after push", mKey)

		var m manifestJSON
		require.NoError(t, json.Unmarshal(manifestBytes, &m),
			"manifest integrity: failed to unmarshal manifest.json at %q — content: %s",
			mKey, string(manifestBytes))

		// Assert every PackEntry's .pack and .idx objects exist in the bucket.
		// A missing object is a dangling manifest reference — a production bug.
		for i, entry := range m.Packs {
			t.Logf("manifest_matches_bucket_after_push: checking pack entry %d: pack=%s idx=%s",
				i, entry.PackKey, entry.IdxKey)

			// Assert .pack object exists.
			packData, packErr := mn.GetObject(ctx, entry.PackKey)
			require.NoErrorf(t, packErr,
				"manifest integrity: PackEntry[%d].PackKey=%q not found in bucket %q — "+
					"dangling manifest reference (production bug: manifest references object that was not synced)",
				i, entry.PackKey, mn.BucketName)
			require.NotEmptyf(t, packData,
				"manifest integrity: PackEntry[%d].PackKey=%q is empty in bucket %q",
				i, entry.PackKey, mn.BucketName)

			// Assert .idx object exists.
			idxData, idxErr := mn.GetObject(ctx, entry.IdxKey)
			require.NoErrorf(t, idxErr,
				"manifest integrity: PackEntry[%d].IdxKey=%q not found in bucket %q — "+
					"dangling manifest reference (production bug: manifest references index that was not synced)",
				i, entry.IdxKey, mn.BucketName)
			require.NotEmptyf(t, idxData,
				"manifest integrity: PackEntry[%d].IdxKey=%q is empty in bucket %q",
				i, entry.IdxKey, mn.BucketName)
		}

		// List all pack objects in the bucket to verify no objects are missing from the
		// manifest (manifest must describe all packs, not just a subset).
		packPrefix := "sessions/" + sessionID + "/packs/"
		bucketPackKeys, err := mn.ListObjects(ctx, packPrefix)
		require.NoError(t, err,
			"manifest integrity: ListObjects(%q) failed", packPrefix)

		// Build a set of all keys referenced in the manifest.
		manifestKeySet := make(map[string]bool, len(m.Packs)*2)
		for _, entry := range m.Packs {
			manifestKeySet[entry.PackKey] = true
			manifestKeySet[entry.IdxKey] = true
		}

		// Every pack object in the bucket must appear in the manifest.
		for _, bucketKey := range bucketPackKeys {
			require.Truef(t, manifestKeySet[bucketKey],
				"manifest integrity: object %q exists in bucket %q but is not referenced "+
					"by any PackEntry in manifest — dangling bucket object (production bug: "+
					"object synced but manifest not updated atomically)",
				bucketKey, mn.BucketName)
		}

		t.Logf("manifest_matches_bucket_after_push: manifest has %d pack entries; "+
			"bucket has %d pack objects — all consistent",
			len(m.Packs), len(bucketPackKeys))
	})

	// ── Subtest 2: manifest_session_id_and_version ───────────────────────────
	// After a push, the manifest must carry the correct session_id (matching the
	// session under test) and version == 1. These fields are structural
	// invariants — a wrong session_id means the wrong manifest was written to
	// the wrong key; a wrong version means schema drift.
	t.Run("manifest_session_id_and_version", func(t *testing.T) {
		userEmail := randEmail(t, "manifest-meta")
		pair := authflow.SignInViaMagicLink(ctx, t, pod0, mh, userEmail)
		userID := rpo0GetMe(ctx, t, pod0.URL, pair.AccessToken)
		orgID := authflow.CreateOrg(ctx, t, pod0, pair.AccessToken, "Manifest Meta Org")
		sessionID := rpo0CreateSession(ctx, t, pod0.URL, pair.AccessToken, orgID, "manifest-meta")

		repo := gitclient.Clone(ctx, t, pod0.URL, orgID, sessionID, userID, pair.AccessToken)
		ref := "jam/" + sessionID + "/" + userID + "/main"
		repo.Commit(ctx, t, "meta-check.md", "content for manifest metadata test", "manifest: metadata check commit")
		repo.Push(ctx, t, ref)

		// Read the manifest directly from MinIO.
		mKey := manifestKey(sessionID)
		manifestBytes, err := mn.GetObject(ctx, mKey)
		require.NoError(t, err,
			"manifest metadata: GetObject(%q) failed — manifest not written after push", mKey)
		require.NotEmpty(t, manifestBytes,
			"manifest metadata: manifest.json at %q is empty after push", mKey)

		var m manifestJSON
		require.NoError(t, json.Unmarshal(manifestBytes, &m),
			"manifest metadata: failed to unmarshal manifest.json at %q — content: %s",
			mKey, string(manifestBytes))

		// Assert session_id is correct — a wrong value would mean the manifest was
		// written to the wrong key or populated with wrong metadata.
		require.Equalf(t, sessionID, m.SessionID,
			"manifest metadata: manifest.SessionID=%q want %q — "+
				"session ID embedded in manifest body does not match the session key path",
			m.SessionID, sessionID)

		// Assert version == 1 — the only valid schema version in the current
		// implementation. An unexpected version indicates schema drift or
		// a migration error.
		require.Equalf(t, 1, m.Version,
			"manifest metadata: manifest.Version=%d want 1 — "+
				"unexpected schema version; check objectstore.ManifestStore.Save version normalisation",
			m.Version)

		t.Logf("manifest_session_id_and_version: session_id=%q version=%d OK",
			m.SessionID, m.Version)
	})
}
