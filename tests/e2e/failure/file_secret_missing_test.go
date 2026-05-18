// Invariant: the portal exits non-zero when JAMSESH_DB_DSN_FILE is set but
// the target file is missing or unreadable. A silent fallback to an empty DSN
// in this case would allow a production deploy with broken secret-file wiring
// to start with no credentials — the bug these tests exist to catch.
//
// Two subtests:
//   - file_missing: _FILE points at /no/such/file (never existed)
//   - file_unreadable: _FILE points at a mounted file with mode 0o000
//
// Both assert: container exits non-zero within 30 s AND container logs contain
// a substring that identifies the _FILE failure (exit-code-only is too weak).
package failure_test

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
)

// baseFileSecretEnv returns the minimal env map for a portal that would
// succeed except for the missing/unreadable _FILE secret. Callers overwrite
// JAMSESH_DB_DSN_FILE with the path under test.
func baseFileSecretEnv() map[string]string {
	return map[string]string{
		"JAMSESH_BIND":       ":8443",
		"JAMSESH_TLS_MODE":   "behind_proxy",
		"JAMSESH_DB_DRIVER":  "sqlite",
		"JAMSESH_EMAIL_FROM": "noreply@example.com",
		// JAMSESH_STORAGE is omitted — the portal won't reach init if _FILE fails.
	}
}

// assertFileSecretFailure polls until the container exits, then asserts:
//   - exit code is non-zero
//   - container logs mention _FILE or the secret-read error
func assertFileSecretFailure(ctx context.Context, t *testing.T, c testcontainers.Container) {
	t.Helper()

	require.Eventually(t, func() bool {
		state, err := c.State(ctx)
		if err != nil {
			return false
		}
		return state.Status == "exited"
	}, 30*time.Second, 500*time.Millisecond,
		"portal must exit when JAMSESH_DB_DSN_FILE is set but the target is missing or unreadable")

	state, err := c.State(ctx)
	require.NoError(t, err, "inspect container state after exit")
	require.NotEqual(t, 0, state.ExitCode,
		"portal must exit non-zero on _FILE error (exit code %d)", state.ExitCode)

	// Read container logs and assert they surface the _FILE failure.
	// secrets.go produces: "config: read JAMSESH_DB_DSN_FILE (<path>): <os error>"
	// which always contains "_FILE" — the canonical marker callers should look for.
	rc, err := c.Logs(ctx)
	require.NoError(t, err, "collect container logs")
	defer rc.Close()
	logBytes, _ := io.ReadAll(rc)
	logs := strings.ToLower(string(logBytes))
	require.True(t,
		strings.Contains(logs, "_file") ||
			strings.Contains(logs, "read secret") ||
			strings.Contains(logs, "secret file"),
		"expected container logs to mention _FILE failure; got:\n%s", string(logBytes))
}

func TestFileSecretMissing(t *testing.T) {
	requireDockerLocal(t)
	requirePortalImageLocal(t)

	t.Run("file_missing", func(t *testing.T) {
		// JAMSESH_DB_DSN_FILE points at a path that will never exist inside the
		// container. readEnvOrFile calls os.ReadFile and returns an error, which
		// main.go surfaces as a fatal log + non-zero exit.
		ctx := context.Background()

		env := baseFileSecretEnv()
		env["JAMSESH_DB_DSN_FILE"] = "/no/such/file"

		// Use the raw testcontainers API — portal.Start waits for /healthz 200
		// which will never come when the portal crashes at config load.
		c, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
			ContainerRequest: testcontainers.ContainerRequest{
				Image:        portalImage,
				ExposedPorts: []string{"8443/tcp"},
				Env:          env,
				// No WaitingFor: the portal must crash before /healthz is reachable.
			},
			Started: true,
		})
		require.NoError(t, err,
			"container should be created at the Docker level (portal crash comes later)")
		t.Cleanup(func() { _ = testcontainers.TerminateContainer(c) })

		assertFileSecretFailure(ctx, t, c)
	})

	t.Run("file_unreadable", func(t *testing.T) {
		// A file exists at the mounted path but is mode 0o000 — the container
		// process (running as nobody) cannot read it. readEnvOrFile returns
		// "permission denied", triggering the same fatal exit path.
		ctx := context.Background()

		secretPath := filepath.Join(t.TempDir(), "db_dsn")
		// Host file must be readable (0o600) so testcontainers can os.Open it
		// during container build. Unreadability inside the container is
		// enforced via the ContainerFile FileMode (0o000) below.
		require.NoError(t,
			os.WriteFile(secretPath, []byte("ignored"), 0o600),
			"create secret file")

		env := baseFileSecretEnv()
		env["JAMSESH_DB_DSN_FILE"] = "/run/secrets/db_dsn"

		c, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
			ContainerRequest: testcontainers.ContainerRequest{
				Image:        portalImage,
				ExposedPorts: []string{"8443/tcp"},
				Env:          env,
				Files: []testcontainers.ContainerFile{
					{
						HostFilePath:      secretPath,
						ContainerFilePath: "/run/secrets/db_dsn",
						FileMode:          0o000, // unreadable by the container process
					},
				},
				// No WaitingFor: the portal must crash before /healthz is reachable.
			},
			Started: true,
		})
		require.NoError(t, err,
			"container should be created at the Docker level (portal crash comes later)")
		t.Cleanup(func() { _ = testcontainers.TerminateContainer(c) })

		assertFileSecretFailure(ctx, t, c)
	})
}
