// Package mailhog provides a Testcontainers-Go fixture for MailHog.
//
// MailHog is a test SMTP server with an HTTP API for inspecting captured mail.
// Each call to Start spins up a fresh container for the test and tears it down
// via t.Cleanup.
//
// Usage:
//
//	mh := mailhog.Start(ctx, t)
//	// mh.SMTPHost / mh.SMTPPort for sending
//	// mh.HTTPURL + "/api/v2/messages" for inspecting captured mail
package mailhog

import (
	"context"
	"fmt"
	"net/http"
	"os/exec"
	"testing"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

const (
	image    = "mailhog/mailhog:v1.0.1"
	smtpPort = "1025/tcp"
	httpPort = "8025/tcp"
)

// MailHog holds connection info for a running MailHog container.
type MailHog struct {
	// SMTPHost and SMTPPort use the host-side (mapped) port. Use these to send
	// mail from the test process (outside Docker).
	SMTPHost string
	SMTPPort int

	// ContainerSMTPHost is the Docker bridge IP of the MailHog container.
	// Use this when configuring another Docker container (e.g. the portal
	// fixture) to send mail via MailHog — from inside Docker the host-mapped
	// port is not reachable but the bridge IP on port 1025 is.
	ContainerSMTPHost string
	// ContainerSMTPPort is always 1025 (the internal port).
	ContainerSMTPPort int

	// HTTPURL is the base URL for the MailHog API using the host-side port,
	// e.g. "http://localhost:PORT". Append "/api/v2/messages" to list mail.
	HTTPURL string

	container testcontainers.Container
}

// Start spins up a fresh MailHog container and registers t.Cleanup to
// terminate it. Skips the test cleanly if Docker is unavailable.
func Start(ctx context.Context, t *testing.T) *MailHog {
	t.Helper()
	requireDocker(t)

	req := testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        image,
			ExposedPorts: []string{smtpPort, httpPort},
			WaitingFor: wait.ForAll(
				wait.ForListeningPort(smtpPort),
				wait.ForHTTP("/api/v2/messages").WithPort(httpPort),
			),
		},
		Started: true,
	}

	c, err := testcontainers.GenericContainer(ctx, req)
	if err != nil {
		t.Fatalf("mailhog: start container: %v", err)
	}

	t.Cleanup(func() {
		if err := testcontainers.TerminateContainer(c); err != nil {
			t.Logf("mailhog: cleanup: terminate: %v", err)
		}
	})

	host, err := c.Host(ctx)
	if err != nil {
		t.Fatalf("mailhog: get host: %v", err)
	}

	smtpMapped, err := c.MappedPort(ctx, smtpPort)
	if err != nil {
		t.Fatalf("mailhog: get smtp port: %v", err)
	}

	httpMapped, err := c.MappedPort(ctx, httpPort)
	if err != nil {
		t.Fatalf("mailhog: get http port: %v", err)
	}

	containerIP, err := c.ContainerIP(ctx)
	if err != nil {
		t.Fatalf("mailhog: get container IP: %v", err)
	}

	return &MailHog{
		SMTPHost:          host,
		SMTPPort:          int(smtpMapped.Num()),
		ContainerSMTPHost: containerIP,
		ContainerSMTPPort: 1025,
		HTTPURL:           fmt.Sprintf("http://%s:%d", host, httpMapped.Num()),
		container:         c,
	}
}

// requireDocker skips t if the Docker daemon is not reachable.
func requireDocker(t *testing.T) {
	t.Helper()
	if err := exec.Command("docker", "info").Run(); err != nil {
		t.Skip("docker not available")
	}
}

// Stop stops the MailHog container without removing it. It is safe to call
// multiple times. The t.Cleanup registered by Start will still call
// TerminateContainer (which is a no-op on an already-stopped container).
// Stop is used by failure-mode tests that need to simulate SMTP unavailability
// mid-test.
func (m *MailHog) Stop(ctx context.Context) error {
	return m.container.Stop(ctx, nil)
}

// CheckReachable performs a quick HTTP GET against the MailHog API to verify
// it is reachable. Returns a non-nil error if it is not.
func (m *MailHog) CheckReachable() error {
	resp, err := http.Get(m.HTTPURL + "/api/v2/messages") //nolint:noctx
	if err != nil {
		return fmt.Errorf("mailhog: http check: %w", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("mailhog: http check: unexpected status %d", resp.StatusCode)
	}
	return nil
}
