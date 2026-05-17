package scaffolding_test

import (
	"fmt"
	"net/http"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func requireDocker(t *testing.T) {
	t.Helper()
	if err := exec.Command("docker", "info").Run(); err != nil {
		t.Skip("docker not available")
	}
}

func requirePortalImage(t *testing.T) {
	t.Helper()
	if err := exec.Command("docker", "image", "inspect", "jamsesh/portal:e2e").Run(); err != nil {
		t.Skip("jamsesh/portal:e2e image not present — run 'make test-portal-image' first")
	}
}

// TestPortalImageHealthz boots the portal e2e image, polls /healthz, and
// asserts a 200 response within 10 seconds. The test skips cleanly when
// Docker is unavailable or the image has not been built yet.
func TestPortalImageHealthz(t *testing.T) {
	requireDocker(t)
	requirePortalImage(t)

	containerName := fmt.Sprintf("portal-e2e-test-%d", time.Now().UnixNano())

	startCmd := exec.Command(
		"docker", "run", "--rm", "-d",
		"--name", containerName,
		"-e", "JAMSESH_DB_DRIVER=sqlite",
		"-e", "JAMSESH_DB_DSN=:memory:",
		"-e", "JAMSESH_TLS_MODE=behind_proxy",
		"-e", "JAMSESH_EMAIL_FROM=noreply@example.com",
		"-p", "0:8443",
		"jamsesh/portal:e2e",
	)
	out, err := startCmd.Output()
	if err != nil {
		t.Fatalf("docker run failed: %v", err)
	}
	_ = strings.TrimSpace(string(out)) // container ID

	t.Cleanup(func() {
		_ = exec.Command("docker", "rm", "-f", containerName).Run()
	})

	// Resolve the host port assigned to container port 8443.
	portOut, err := exec.Command("docker", "port", containerName, "8443").Output()
	if err != nil {
		t.Fatalf("docker port lookup failed: %v", err)
	}
	// docker port output: "0.0.0.0:12345\n" or ":::12345\n"
	mapping := strings.TrimSpace(string(portOut))
	parts := strings.Split(mapping, ":")
	hostPort := parts[len(parts)-1]

	healthURL := fmt.Sprintf("http://localhost:%s/healthz", hostPort)

	deadline := time.Now().Add(10 * time.Second)
	var lastStatus int
	for time.Now().Before(deadline) {
		resp, err := http.Get(healthURL) //nolint:noctx
		if err == nil {
			lastStatus = resp.StatusCode
			resp.Body.Close()
			if lastStatus == http.StatusOK {
				return
			}
		}
		time.Sleep(500 * time.Millisecond)
	}

	t.Errorf("/healthz did not return 200 within 10s (last status: %d, url: %s)", lastStatus, healthURL)
}
