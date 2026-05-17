package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeSecretFile writes content to a file in t.TempDir() and returns the path.
func writeSecretFile(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "secret")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("writeSecretFile: %v", err)
	}
	return path
}

// clearSecretEnv unsets the secret-bearing env vars and their _FILE variants
// for test isolation. Non-secret vars are left to clearEnv in config_test.go,
// but since that helper is in package config_test we replicate what we need here.
func clearSecretEnv(t *testing.T) {
	t.Helper()
	vars := []string{
		"JAMSESH_DB_DSN",
		"JAMSESH_OAUTH_GITHUB_CLIENT_SECRET",
		"JAMSESH_EMAIL_SMTP_PASS",
		"JAMSESH_EMAIL_SENDGRID_API_KEY",
		"JAMSESH_EMAIL_POSTMARK_SERVER_TOKEN",
		"JAMSESH_EMAIL_RESEND_API_KEY",
	}
	for _, v := range vars {
		t.Setenv(v, "")
		os.Unsetenv(v) //nolint:errcheck
		t.Setenv(v+"_FILE", "")
		os.Unsetenv(v + "_FILE") //nolint:errcheck
	}
	// Also clear the common non-secret vars needed for Load to succeed.
	nonSecrets := []string{
		"JAMSESH_BIND", "JAMSESH_DB_DRIVER",
		"JAMSESH_TLS_MODE", "JAMSESH_TLS_CERT", "JAMSESH_TLS_KEY",
		"JAMSESH_LOG_FORMAT", "JAMSESH_LOG_LEVEL", "JAMSESH_STORAGE",
		"JAMSESH_PORTAL_URL", "JAMSESH_GIT_MAX_PACK_BYTES",
		"JAMSESH_OAUTH_GITHUB_CLIENT_ID", "JAMSESH_OAUTH_GITHUB_BASE_URL",
		"JAMSESH_EMAIL_PROVIDER", "JAMSESH_EMAIL_FROM",
		"JAMSESH_EMAIL_SMTP_HOST", "JAMSESH_EMAIL_SMTP_PORT",
		"JAMSESH_EMAIL_SMTP_USER", "JAMSESH_EMAIL_SMTP_TLS",
		"JAMSESH_EMAIL_POSTMARK_MESSAGE_STREAM",
		"JAMSESH_BIND_FILE",
	}
	for _, v := range nonSecrets {
		t.Setenv(v, "")
		os.Unsetenv(v) //nolint:errcheck
	}
}

// TestReadEnvOrFile_NeitherSet verifies ("", nil) is returned when neither
// the plain var nor the _FILE var is set.
func TestReadEnvOrFile_NeitherSet(t *testing.T) {
	const name = "READENVTEST_NEITHER"
	t.Setenv(name, "")
	os.Unsetenv(name) //nolint:errcheck
	t.Setenv(name+"_FILE", "")
	os.Unsetenv(name + "_FILE") //nolint:errcheck

	got, err := readEnvOrFile(name)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "" {
		t.Errorf("got %q, want empty string", got)
	}
}

// TestReadEnvOrFile_PlainVarSet verifies the plain env var is returned when
// the _FILE variant is not set.
func TestReadEnvOrFile_PlainVarSet(t *testing.T) {
	const name = "READENVTEST_PLAIN"
	t.Setenv(name+"_FILE", "")
	os.Unsetenv(name + "_FILE") //nolint:errcheck
	t.Setenv(name, "my-secret-value")

	got, err := readEnvOrFile(name)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "my-secret-value" {
		t.Errorf("got %q, want %q", got, "my-secret-value")
	}
}

// TestReadEnvOrFile_FileWins verifies that when both the plain var and _FILE
// are set, the file contents take precedence.
func TestReadEnvOrFile_FileWins(t *testing.T) {
	const name = "READENVTEST_FILE_WINS"
	path := writeSecretFile(t, "from-file")
	t.Setenv(name+"_FILE", path)
	t.Setenv(name, "from-plain-var")

	got, err := readEnvOrFile(name)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "from-file" {
		t.Errorf("got %q, want %q (file should win)", got, "from-file")
	}
}

// TestReadEnvOrFile_FileReadable verifies that a readable _FILE is returned
// when only the _FILE var is set.
func TestReadEnvOrFile_FileReadable(t *testing.T) {
	const name = "READENVTEST_FILE_READABLE"
	path := writeSecretFile(t, "secret-from-file")
	t.Setenv(name+"_FILE", path)
	t.Setenv(name, "")
	os.Unsetenv(name) //nolint:errcheck

	got, err := readEnvOrFile(name)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "secret-from-file" {
		t.Errorf("got %q, want %q", got, "secret-from-file")
	}
}

// TestReadEnvOrFile_TrailingWhitespaceTrimmed verifies that trailing
// whitespace including newlines are stripped from file contents.
func TestReadEnvOrFile_TrailingWhitespaceTrimmed(t *testing.T) {
	const name = "READENVTEST_TRIM"
	tests := []struct {
		desc    string
		content string
		want    string
	}{
		{"trailing newline", "my-secret\n", "my-secret"},
		{"trailing CRLF", "my-secret\r\n", "my-secret"},
		{"trailing spaces and newline", "my-secret  \n", "my-secret"},
		{"trailing tabs and newline", "my-secret\t\n", "my-secret"},
		{"no trailing whitespace", "my-secret", "my-secret"},
		{"multiple trailing newlines", "my-secret\n\n\n", "my-secret"},
	}

	for _, tc := range tests {
		t.Run(tc.desc, func(t *testing.T) {
			path := writeSecretFile(t, tc.content)
			t.Setenv(name+"_FILE", path)
			t.Setenv(name, "")
			os.Unsetenv(name) //nolint:errcheck

			got, err := readEnvOrFile(name)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("content %q: got %q, want %q", tc.content, got, tc.want)
			}
		})
	}
}

// TestReadEnvOrFile_UnreadablePath verifies that a _FILE pointing at an
// unreadable path returns a descriptive error.
func TestReadEnvOrFile_UnreadablePath(t *testing.T) {
	const name = "READENVTEST_UNREADABLE"
	t.Setenv(name+"_FILE", "/nonexistent/path/to/secret")
	t.Setenv(name, "")
	os.Unsetenv(name) //nolint:errcheck

	got, err := readEnvOrFile(name)
	if err == nil {
		t.Fatalf("expected error for unreadable path, got value %q", got)
	}
	// Error message should identify the variable and path.
	if !strings.Contains(err.Error(), name+"_FILE") {
		t.Errorf("error %q does not mention %s_FILE", err.Error(), name)
	}
}

// TestLoad_DBDSNFile verifies that JAMSESH_DB_DSN_FILE is read through Load.
func TestLoad_DBDSNFile(t *testing.T) {
	clearSecretEnv(t)
	path := writeSecretFile(t, "postgres://file-host/filedb\n")
	t.Setenv("JAMSESH_DB_DSN_FILE", path)

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.DBDSN != "postgres://file-host/filedb" {
		t.Errorf("DBDSN: got %q, want %q", cfg.DBDSN, "postgres://file-host/filedb")
	}
}

// TestLoad_DBDSNFile_UnreadableErrors verifies that Load returns an error when
// JAMSESH_DB_DSN_FILE points at an unreadable path.
func TestLoad_DBDSNFile_UnreadableErrors(t *testing.T) {
	clearSecretEnv(t)
	t.Setenv("JAMSESH_DB_DSN_FILE", "/nonexistent/dsn-secret")

	_, err := Load("")
	if err == nil {
		t.Fatal("expected error for unreadable JAMSESH_DB_DSN_FILE, got nil")
	}
}

// TestLoad_OAuthClientSecretFile verifies that JAMSESH_OAUTH_GITHUB_CLIENT_SECRET_FILE
// is read through Load.
func TestLoad_OAuthClientSecretFile(t *testing.T) {
	clearSecretEnv(t)
	path := writeSecretFile(t, "ghsecret-from-file\n")
	t.Setenv("JAMSESH_OAUTH_GITHUB_CLIENT_SECRET_FILE", path)

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.OAuth.GitHub.ClientSecret != "ghsecret-from-file" {
		t.Errorf("ClientSecret: got %q, want %q", cfg.OAuth.GitHub.ClientSecret, "ghsecret-from-file")
	}
}

// TestLoad_SMTPPassFile verifies that JAMSESH_EMAIL_SMTP_PASS_FILE is read through Load.
func TestLoad_SMTPPassFile(t *testing.T) {
	clearSecretEnv(t)
	path := writeSecretFile(t, "smtp-password\n")
	t.Setenv("JAMSESH_EMAIL_SMTP_PASS_FILE", path)

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Email.SMTP.Pass != "smtp-password" {
		t.Errorf("SMTP.Pass: got %q, want %q", cfg.Email.SMTP.Pass, "smtp-password")
	}
}

// TestLoad_SendGridAPIKeyFile verifies that JAMSESH_EMAIL_SENDGRID_API_KEY_FILE
// is read through Load.
func TestLoad_SendGridAPIKeyFile(t *testing.T) {
	clearSecretEnv(t)
	path := writeSecretFile(t, "SG.apikey\n")
	t.Setenv("JAMSESH_EMAIL_SENDGRID_API_KEY_FILE", path)

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Email.SendGrid.APIKey != "SG.apikey" {
		t.Errorf("SendGrid.APIKey: got %q, want %q", cfg.Email.SendGrid.APIKey, "SG.apikey")
	}
}

// TestLoad_PostmarkServerTokenFile verifies that JAMSESH_EMAIL_POSTMARK_SERVER_TOKEN_FILE
// is read through Load.
func TestLoad_PostmarkServerTokenFile(t *testing.T) {
	clearSecretEnv(t)
	path := writeSecretFile(t, "pm-token\n")
	t.Setenv("JAMSESH_EMAIL_POSTMARK_SERVER_TOKEN_FILE", path)

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Email.Postmark.ServerToken != "pm-token" {
		t.Errorf("Postmark.ServerToken: got %q, want %q", cfg.Email.Postmark.ServerToken, "pm-token")
	}
}

// TestLoad_ResendAPIKeyFile verifies that JAMSESH_EMAIL_RESEND_API_KEY_FILE
// is read through Load.
func TestLoad_ResendAPIKeyFile(t *testing.T) {
	clearSecretEnv(t)
	path := writeSecretFile(t, "re_apikey\n")
	t.Setenv("JAMSESH_EMAIL_RESEND_API_KEY_FILE", path)

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Email.Resend.APIKey != "re_apikey" {
		t.Errorf("Resend.APIKey: got %q, want %q", cfg.Email.Resend.APIKey, "re_apikey")
	}
}

// TestLoad_FilePrecedenceOverPlainVar verifies that the _FILE value wins when
// both the plain env var and _FILE are set (tested end-to-end through Load).
func TestLoad_FilePrecedenceOverPlainVar(t *testing.T) {
	clearSecretEnv(t)
	path := writeSecretFile(t, "from-file-wins\n")
	t.Setenv("JAMSESH_DB_DSN_FILE", path)
	t.Setenv("JAMSESH_DB_DSN", "from-plain-var")

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.DBDSN != "from-file-wins" {
		t.Errorf("DBDSN: got %q, want file value %q", cfg.DBDSN, "from-file-wins")
	}
}

// TestLoad_NonSecretVarsUnaffected verifies that non-secret env vars like
// JAMSESH_BIND and JAMSESH_PORTAL_URL still use plain os.Getenv and do not
// gain _FILE support (i.e., JAMSESH_BIND_FILE should have no effect).
func TestLoad_NonSecretVarsUnaffected(t *testing.T) {
	clearSecretEnv(t)
	// Set a _FILE variant for a non-secret var — it must be ignored.
	t.Setenv("JAMSESH_BIND_FILE", "/nonexistent/bind-file")
	// The plain var should still work normally.
	t.Setenv("JAMSESH_BIND", "127.0.0.1:9999")

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load: %v", err) // must NOT error for BIND_FILE
	}
	if cfg.Bind != "127.0.0.1:9999" {
		t.Errorf("Bind: got %q, want %q", cfg.Bind, "127.0.0.1:9999")
	}
}
