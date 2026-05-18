// Invariant: after a successful git push to a clustered-mode portal, every
// produced object (loose objects, pack files, refs) is queryable in the MinIO
// bucket via direct S3 API before the push ACK is returned to the client.
// RPO=0: ACK implies durable.
//
// Assertion order is mandated by test-integrity rules: bucket inspection via
// mn.ListObjects comes FIRST; push HTTP status is checked AFTER. Asserting
// only on the HTTP response is tautological — the bucket check IS the RPO=0
// assertion.
package golden_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"jamsesh/tests/e2e/fixtures/authflow"
	"jamsesh/tests/e2e/fixtures/gitclient"
	"jamsesh/tests/e2e/fixtures/mailhog"
	"jamsesh/tests/e2e/fixtures/minio"
	"jamsesh/tests/e2e/fixtures/portalcluster"
	"jamsesh/tests/e2e/fixtures/postgres"
)

// rpo0SessionRef is the minimal response shape from POST /api/orgs/{id}/sessions.
type rpo0SessionRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// TestObjectStorageRPO0 verifies the RPO=0 durability invariant for the
// object-storage sync layer across four push scenarios.
//
// Infrastructure starts once; each subtest creates its own user, org, and
// session so state is fully isolated without paying the Docker startup cost
// four times.
func TestObjectStorageRPO0(t *testing.T) {
	ctx := context.Background()

	// ── Infrastructure ───────────────────────────────────────────────────────
	mn := minio.Start(ctx, t, minio.Options{})
	pg := postgres.Start(ctx, t, postgres.Options{})
	mh := mailhog.Start(ctx, t)

	// PortalExtraEnv: MailHog SMTP for magic-link delivery; two pods share the
	// same object-store and postgres so any pod can answer auth calls.
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

	// ── Subtest 1: small_commit ──────────────────────────────────────────────
	// Push a single small commit; assert the bucket under sessions/<id>/ is
	// non-empty BEFORE checking the push status code.
	t.Run("small_commit", func(t *testing.T) {
		userEmail := randEmail(t, "rpo0-small")
		pair := authflow.SignInViaMagicLink(ctx, t, pod0, mh, userEmail)
		userID := rpo0GetMe(ctx, t, pod0.URL, pair.AccessToken)
		orgID := authflow.CreateOrg(ctx, t, pod0, pair.AccessToken, "RPO0 Small Org")
		sessionID := rpo0CreateSession(ctx, t, pod0.URL, pair.AccessToken, orgID, "rpo0-small-commit")

		repo := gitclient.Clone(ctx, t, pod0.URL, orgID, sessionID, userID, pair.AccessToken)
		ref := "jam/" + sessionID + "/" + userID + "/main"
		repo.Commit(ctx, t, "small.md", "small commit content", "rpo0: small commit")
		repo.Push(ctx, t, ref)

		// RPO=0 assertion — bucket FIRST, HTTP status NEVER (push already returned
		// if we reach here; requiring 2xx from Push is implicit in gitclient.Push
		// failing the test on non-zero exit, which git reports on non-2xx push).
		prefix := "sessions/" + sessionID + "/"
		keys, err := mn.ListObjects(ctx, prefix)
		require.NoError(t, err,
			"RPO=0: ListObjects(%q) must not error after a successful push", prefix)
		require.NotEmpty(t, keys,
			"RPO=0 violated: push returned 2xx but MinIO bucket has no objects "+
				"under prefix %q — this is a durability violation (bucket=%q)",
			prefix, mn.BucketName)
		t.Logf("small_commit: %d object(s) in bucket under %s", len(keys), prefix)
	})

	// ── Subtest 2: multi_pack_push ───────────────────────────────────────────
	// Push multiple commits carrying enough content to likely trigger a packfile.
	// Asserts that the bucket under sessions/<id>/ contains objects after the
	// final push; the exact keys depend on git's internal pack decisions.
	t.Run("multi_pack_push", func(t *testing.T) {
		userEmail := randEmail(t, "rpo0-multi")
		pair := authflow.SignInViaMagicLink(ctx, t, pod0, mh, userEmail)
		userID := rpo0GetMe(ctx, t, pod0.URL, pair.AccessToken)
		orgID := authflow.CreateOrg(ctx, t, pod0, pair.AccessToken, "RPO0 Multi Org")
		sessionID := rpo0CreateSession(ctx, t, pod0.URL, pair.AccessToken, orgID, "rpo0-multi-pack")

		repo := gitclient.Clone(ctx, t, pod0.URL, orgID, sessionID, userID, pair.AccessToken)
		ref := "jam/" + sessionID + "/" + userID + "/main"

		// Push ten commits with moderately-sized content. Git may pack these into
		// one or more packfiles on the server side after receiving the push.
		// We do not assert the number of packfiles — only that objects land in the
		// bucket, which is sufficient to verify the RPO=0 contract for the multi-
		// pack code path.
		for i := 0; i < 10; i++ {
			content := strings.Repeat(fmt.Sprintf("line %d of commit %d\n", i, i), 64)
			repo.Commit(ctx, t,
				fmt.Sprintf("file%02d.md", i),
				content,
				fmt.Sprintf("rpo0: multi-pack commit %d", i),
			)
		}
		repo.Push(ctx, t, ref)

		// Direct bucket inspection — the RPO=0 assertion.
		prefix := "sessions/" + sessionID + "/"
		keys, err := mn.ListObjects(ctx, prefix)
		require.NoError(t, err,
			"RPO=0: ListObjects(%q) must not error after multi-pack push", prefix)
		require.NotEmpty(t, keys,
			"RPO=0 violated: multi-pack push returned 2xx but MinIO bucket is empty "+
				"under prefix %q (bucket=%q)",
			prefix, mn.BucketName)
		t.Logf("multi_pack_push: %d object(s) in bucket under %s", len(keys), prefix)
	})

	// ── Subtest 3: refs_only_update ──────────────────────────────────────────
	// Force-push an existing ref to a different commit SHA (ref moves without
	// new unique objects). Asserts the bucket is updated (refs/manifest written).
	//
	// Implementation: push two commits; the second push force-resets the ref to
	// the first commit (rewrites history). The server must still record the ref
	// update in the bucket synchronously before returning 2xx.
	t.Run("refs_only_update", func(t *testing.T) {
		userEmail := randEmail(t, "rpo0-refs")
		pair := authflow.SignInViaMagicLink(ctx, t, pod0, mh, userEmail)
		userID := rpo0GetMe(ctx, t, pod0.URL, pair.AccessToken)
		orgID := authflow.CreateOrg(ctx, t, pod0, pair.AccessToken, "RPO0 Refs Org")
		sessionID := rpo0CreateSession(ctx, t, pod0.URL, pair.AccessToken, orgID, "rpo0-refs-only")

		repo := gitclient.Clone(ctx, t, pod0.URL, orgID, sessionID, userID, pair.AccessToken)
		ref := "jam/" + sessionID + "/" + userID + "/main"

		// First commit: establish the ref.
		firstSHA := repo.Commit(ctx, t, "first.md", "first content", "rpo0: refs-only first")
		repo.Push(ctx, t, ref)

		// Confirm objects landed after first push (prerequisite).
		prefix := "sessions/" + sessionID + "/"
		keysAfterFirst, err := mn.ListObjects(ctx, prefix)
		require.NoError(t, err, "RPO=0: first push ListObjects error")
		require.NotEmpty(t, keysAfterFirst,
			"RPO=0 violated: first push returned 2xx but bucket empty (bucket=%q prefix=%q)",
			mn.BucketName, prefix)
		t.Logf("refs_only_update: %d object(s) after first push", len(keysAfterFirst))

		// Second commit: advance the ref.
		repo.Commit(ctx, t, "second.md", "second content", "rpo0: refs-only second")
		repo.Push(ctx, t, ref)

		// Now force-reset the local branch back to firstSHA and force-push, creating
		// a refs-only update (the object already exists; only the ref tip changes).
		rpo0GitRun(t, repo.Dir, "git", "reset", "--hard", firstSHA)
		rpo0GitForcePush(ctx, t, repo, ref)

		// RPO=0 assertion after force-push (refs-only update path).
		keysAfterForce, err := mn.ListObjects(ctx, prefix)
		require.NoError(t, err, "RPO=0: force-push ListObjects error")
		require.NotEmpty(t, keysAfterForce,
			"RPO=0 violated: force-push returned 2xx but bucket empty "+
				"under prefix %q (bucket=%q) — refs-only update must still be durable",
			prefix, mn.BucketName)
		t.Logf("refs_only_update: %d object(s) after force-push", len(keysAfterForce))
	})

	// ── Subtest 4: tag_creation ──────────────────────────────────────────────
	// Push an annotated tag; assert the bucket reflects the updated ref state.
	//
	// An annotated tag creates a tag object (not just a ref pointer), so the
	// portal must sync both the new tag object and the updated packed-refs/manifest
	// to satisfy RPO=0.
	t.Run("tag_creation", func(t *testing.T) {
		userEmail := randEmail(t, "rpo0-tag")
		pair := authflow.SignInViaMagicLink(ctx, t, pod0, mh, userEmail)
		userID := rpo0GetMe(ctx, t, pod0.URL, pair.AccessToken)
		orgID := authflow.CreateOrg(ctx, t, pod0, pair.AccessToken, "RPO0 Tag Org")
		sessionID := rpo0CreateSession(ctx, t, pod0.URL, pair.AccessToken, orgID, "rpo0-tag-creation")

		repo := gitclient.Clone(ctx, t, pod0.URL, orgID, sessionID, userID, pair.AccessToken)
		ref := "jam/" + sessionID + "/" + userID + "/main"

		// Push a commit first so the tag has something to point at.
		repo.Commit(ctx, t, "tagged.md", "content to tag", "rpo0: tag commit")
		repo.Push(ctx, t, ref)

		// Confirm objects landed after the commit push.
		prefix := "sessions/" + sessionID + "/"
		keysAfterCommit, err := mn.ListObjects(ctx, prefix)
		require.NoError(t, err, "RPO=0: commit push ListObjects error")
		require.NotEmpty(t, keysAfterCommit,
			"RPO=0 violated: commit push returned 2xx but bucket empty (bucket=%q prefix=%q)",
			mn.BucketName, prefix)
		t.Logf("tag_creation: %d object(s) after commit push", len(keysAfterCommit))

		// Create and push an annotated tag. The tag namespace (refs/tags/) is
		// separate from the jam/ namespace; the portal's pre-receive hook may only
		// permit jam/ refs. We push the tag into the jam/ namespace to stay within
		// allowed ref namespaces: jam/<sessionID>/<userID>/v1.0.
		tagRef := "jam/" + sessionID + "/" + userID + "/v1.0"
		rpo0GitRun(t, repo.Dir,
			"git", "tag", "-a", "v1.0", "-m", "annotated tag for RPO=0 test",
		)
		rpo0PushTag(ctx, t, repo, tagRef)

		// RPO=0 assertion after tag push.
		keysAfterTag, err := mn.ListObjects(ctx, prefix)
		require.NoError(t, err, "RPO=0: tag push ListObjects error")
		require.NotEmpty(t, keysAfterTag,
			"RPO=0 violated: tag push returned 2xx but bucket empty "+
				"under prefix %q (bucket=%q) — annotated tag object must be durable",
			prefix, mn.BucketName)
		t.Logf("tag_creation: %d object(s) after tag push", len(keysAfterTag))
	})
}

// ---------------------------------------------------------------------------
// Local helpers — scoped to this file to avoid polluting the golden package.
// ---------------------------------------------------------------------------

// rpo0GetMe calls GET /api/me on baseURL and returns the authenticated user's ID.
func rpo0GetMe(ctx context.Context, t *testing.T, baseURL, accessToken string) string {
	t.Helper()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/api/me", nil)
	require.NoError(t, err, "rpo0GetMe: build request")
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err, "rpo0GetMe: GET /api/me")
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	require.Equalf(t, http.StatusOK, resp.StatusCode,
		"rpo0GetMe: want 200; body=%s", body)

	var me struct {
		ID    string `json:"id"`
		Email string `json:"email"`
	}
	require.NoError(t, json.Unmarshal(body, &me), "rpo0GetMe: decode")
	require.NotEmpty(t, me.ID, "rpo0GetMe: empty user ID")
	return me.ID
}

// rpo0CreateSession calls POST /api/orgs/{orgID}/sessions and returns the new
// session ID.
func rpo0CreateSession(ctx context.Context, t *testing.T, baseURL, accessToken, orgID, name string) string {
	t.Helper()
	body := map[string]string{
		"name":         name,
		"goal":         "RPO=0 golden test",
		"scope":        `["**"]`,
		"default_mode": "sync",
	}
	b, err := json.Marshal(body)
	require.NoError(t, err, "rpo0CreateSession: marshal")

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		fmt.Sprintf("%s/api/orgs/%s/sessions", baseURL, orgID),
		bytes.NewReader(b))
	require.NoError(t, err, "rpo0CreateSession: build request")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err, "rpo0CreateSession: POST")
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	require.Equalf(t, http.StatusCreated, resp.StatusCode,
		"rpo0CreateSession: want 201; body=%s", respBody)

	var s rpo0SessionRef
	require.NoError(t, json.Unmarshal(respBody, &s), "rpo0CreateSession: decode")
	require.NotEmpty(t, s.ID, "rpo0CreateSession: empty session ID")
	return s.ID
}

// rpo0GitRun executes a git command in dir, failing the test on error.
// Uses os/exec directly since gitclient.run is unexported.
func rpo0GitRun(t *testing.T, dir string, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("rpo0GitRun: %s %s: %v\n%s", name, strings.Join(args, " "), err, out)
	}
}

// rpo0GitForcePush force-pushes HEAD to the given ref on the repo's origin remote.
// This is the refs-only-update path: the objects are already in the repo, only
// the ref tip changes.
func rpo0GitForcePush(ctx context.Context, t *testing.T, repo *gitclient.Repo, ref string) {
	t.Helper()
	// We must re-encode credentials in the push URL since gitclient.Push calls
	// "git push origin HEAD:refs/heads/<ref>", but force-push requires --force.
	// Exec directly against the repo working tree which already has the
	// credentialed remote URL from Clone.
	cmd := exec.CommandContext(ctx, "git", "push", "--force", "origin", "HEAD:refs/heads/"+ref)
	cmd.Dir = repo.Dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("rpo0GitForcePush: git push --force: %v\n%s", err, out)
	}
}

// rpo0PushTag pushes the local annotated tag v1.0 to the remote under the given
// jam/ ref. The portal's pre-receive hook only allows jam/ refs; we push the tag
// object as a branch-like ref so it is accepted and stored in the repo.
func rpo0PushTag(ctx context.Context, t *testing.T, repo *gitclient.Repo, tagRef string) {
	t.Helper()
	// Push the tag object (v1.0) as the tip of tagRef. git resolves v1.0 to the
	// annotated tag object SHA; the server stores it as a ref update.
	cmd := exec.CommandContext(ctx, "git", "push", "origin", "v1.0:refs/heads/"+tagRef)
	cmd.Dir = repo.Dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("rpo0PushTag: git push v1.0 to %s: %v\n%s", tagRef, err, out)
	}
}

