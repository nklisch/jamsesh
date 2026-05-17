// Package config loads and validates the portal server configuration.
// Config is sourced from an optional YAML file with env-var overrides.
//
// YAML keys:  bind, db_driver, db_dsn, portal_url, tls.mode, tls.cert_path,
//
//	tls.key_path, log.format, log.level, storage,
//	email.provider, email.from, email.smtp.*, email.sendgrid.*,
//	email.postmark.*, email.resend.*,
//	oauth.github.client_id, oauth.github.client_secret,
//	oauth.github.base_url,
//	git.max_pack_bytes,
//	db.max_open_conns, db.max_idle_conns, db.conn_max_lifetime,
//	shutdown_grace_s,
//	deploy_mode, lease_heartbeat_interval_s,
//	lease_retention_days, lease_retention_interval_hours
//
// Env vars:   JAMSESH_BIND, JAMSESH_DB_DRIVER, JAMSESH_DB_DSN,
//
//	JAMSESH_PORTAL_URL,
//	JAMSESH_TLS_MODE, JAMSESH_TLS_CERT, JAMSESH_TLS_KEY,
//	JAMSESH_LOG_FORMAT, JAMSESH_LOG_LEVEL, JAMSESH_STORAGE,
//	JAMSESH_EMAIL_PROVIDER, JAMSESH_EMAIL_FROM,
//	JAMSESH_EMAIL_SMTP_HOST, JAMSESH_EMAIL_SMTP_PORT,
//	JAMSESH_EMAIL_SMTP_USER, JAMSESH_EMAIL_SMTP_PASS,
//	JAMSESH_EMAIL_SMTP_TLS,
//	JAMSESH_EMAIL_SENDGRID_API_KEY,
//	JAMSESH_EMAIL_POSTMARK_SERVER_TOKEN,
//	JAMSESH_EMAIL_POSTMARK_MESSAGE_STREAM,
//	JAMSESH_EMAIL_RESEND_API_KEY,
//	JAMSESH_OAUTH_GITHUB_CLIENT_ID,
//	JAMSESH_OAUTH_GITHUB_CLIENT_SECRET,
//	JAMSESH_OAUTH_GITHUB_BASE_URL,
//	JAMSESH_GIT_MAX_PACK_BYTES,
//	JAMSESH_DB_MAX_OPEN_CONNS,
//	JAMSESH_DB_MAX_IDLE_CONNS,
//	JAMSESH_DB_CONN_MAX_LIFETIME,
//	JAMSESH_SHUTDOWN_GRACE_S,
//	JAMSESH_DEPLOY_MODE,
//	JAMSESH_LEASE_HEARTBEAT_INTERVAL_S,
//	JAMSESH_LEASE_RETENTION_DAYS,
//	JAMSESH_LEASE_RETENTION_INTERVAL_HOURS
//
// Secret env vars with _FILE variants (file contents take precedence):
//
//	JAMSESH_DB_DSN_FILE,
//	JAMSESH_OAUTH_GITHUB_CLIENT_SECRET_FILE,
//	JAMSESH_EMAIL_SMTP_PASS_FILE,
//	JAMSESH_EMAIL_SENDGRID_API_KEY_FILE,
//	JAMSESH_EMAIL_POSTMARK_SERVER_TOKEN_FILE,
//	JAMSESH_EMAIL_RESEND_API_KEY_FILE
//
// log.level is an integer matching slog.Level values:
//
//	-4=DEBUG, 0=INFO, 4=WARN, 8=ERROR
//
// Use JAMSESH_LOG_LEVEL=-4 for debug, 0 for info (default), etc.
package config

import (
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"time"

	"gopkg.in/yaml.v3"
)

// Config holds the full portal server configuration.
type Config struct {
	Bind      string      `yaml:"bind"`       // listen address, e.g. ":8443"
	DBDriver  string      `yaml:"db_driver"`  // "sqlite" | "postgres"
	DBDSN     string      `yaml:"db_dsn"`     // DSN appropriate for DBDriver
	PortalURL string      `yaml:"portal_url"` // public base URL, e.g. "https://example.com"
	TLS       TLSConfig   `yaml:"tls"`
	Log       LogConfig   `yaml:"log"`
	Storage   string      `yaml:"storage"` // path for bare git repos
	Email     EmailConfig `yaml:"email"`
	OAuth     OAuthConfig `yaml:"oauth"`
	Git       GitConfig   `yaml:"git"`
	DB        DBConfig    `yaml:"db"`
	// ShutdownGraceSeconds is the total wall-clock budget for graceful shutdown,
	// shared across HTTP draining, auto-merger stop, and WS gateway stop.
	// Default: 30 (matches k8s terminationGracePeriodSeconds). Must be positive.
	// Env: JAMSESH_SHUTDOWN_GRACE_S
	ShutdownGraceSeconds int `yaml:"shutdown_grace_s"`

	// DeployMode selects single-instance vs. clustered operation.
	// "single" (default) uses NoopManager — no PG lease queries are issued.
	// "clustered" uses PostgresManager with advisory locks and fencing tokens;
	// requires db_driver=postgres.
	// Env: JAMSESH_DEPLOY_MODE
	DeployMode string `yaml:"deploy_mode"`

	// LeaseHeartbeatIntervalS is how often (in seconds) the heartbeat goroutine
	// pings the dedicated PG connection. Default: 10.
	// Env: JAMSESH_LEASE_HEARTBEAT_INTERVAL_S
	LeaseHeartbeatIntervalS int `yaml:"lease_heartbeat_interval_s"`

	// LeaseRetentionDays controls how long released lease rows are kept before
	// the retention goroutine deletes them. Default: 30.
	// Env: JAMSESH_LEASE_RETENTION_DAYS
	LeaseRetentionDays int `yaml:"lease_retention_days"`

	// LeaseRetentionIntervalHours is how often the retention goroutine runs.
	// Default: 1.
	// Env: JAMSESH_LEASE_RETENTION_INTERVAL_HOURS
	LeaseRetentionIntervalHours int `yaml:"lease_retention_interval_hours"`
}

// DBConfig holds database connection pool settings.
// These apply to Postgres; for SQLite the values are accepted but have no
// concurrency benefit since SQLite is effectively single-writer.
type DBConfig struct {
	// MaxOpenConns is the maximum number of open connections in the pool.
	// Default: 25. For Postgres this maps to pgxpool.Config.MaxConns.
	MaxOpenConns int `yaml:"max_open_conns"`
	// MaxIdleConns is the minimum number of idle connections the pool
	// maintains. Default: 5. For Postgres this maps to pgxpool.Config.MinConns.
	MaxIdleConns int `yaml:"max_idle_conns"`
	// ConnMaxLifetime is the maximum lifetime of a pooled connection before
	// it is closed and replaced. Default: 30m.
	// Accepts Go duration strings: "30m", "1h", etc.
	ConnMaxLifetime time.Duration `yaml:"conn_max_lifetime"`
}

// GitConfig holds git-push policy settings.
type GitConfig struct {
	// MaxPackBytes is the maximum size (in bytes) of a pushed pack.
	// Pushes exceeding this limit are rejected with push.size_limit.
	// Default: 52428800 (50 MiB). Set to 0 to disable the check.
	MaxPackBytes int64 `yaml:"max_pack_bytes"`
}

// OAuthConfig holds OAuth provider credentials. Only providers with non-empty
// ClientID and ClientSecret are considered configured. The start endpoint
// returns 503 for providers without credentials.
type OAuthConfig struct {
	GitHub GitHubOAuthConfig `yaml:"github"`
}

// GitHubOAuthConfig holds GitHub OAuth application credentials.
type GitHubOAuthConfig struct {
	ClientID     string `yaml:"client_id"`
	ClientSecret string `yaml:"client_secret"`
	// BaseURL overrides the GitHub OAuth and API base URL for testing.
	// Leave empty in production.
	BaseURL string `yaml:"base_url"`
}

// EmailConfig selects the email delivery provider and holds all provider
// credentials. Only the chosen provider's sub-struct needs valid fields;
// the others are ignored. Validation runs in senders.New() at startup.
type EmailConfig struct {
	// Provider selects the delivery backend: smtp | sendgrid | postmark | resend
	Provider string `yaml:"provider"`
	// From is the envelope sender address, e.g. "jamsesh <noreply@example.com>"
	From string `yaml:"from"`

	SMTP     SMTPConfig     `yaml:"smtp"`
	SendGrid SendGridConfig `yaml:"sendgrid"`
	Postmark PostmarkConfig `yaml:"postmark"`
	Resend   ResendConfig   `yaml:"resend"`
}

// SMTPConfig holds SMTP connection settings.
type SMTPConfig struct {
	Host    string `yaml:"host"`
	Port    int    `yaml:"port"`
	User    string `yaml:"user"`
	Pass    string `yaml:"pass"`
	TLSMode string `yaml:"tls"` // "mandatory" | "opportunistic" | "none"
}

// SendGridConfig holds SendGrid API credentials.
type SendGridConfig struct {
	APIKey string `yaml:"api_key"`
}

// PostmarkConfig holds Postmark API credentials.
type PostmarkConfig struct {
	ServerToken   string `yaml:"server_token"`
	MessageStream string `yaml:"message_stream"` // defaults to "outbound"
}

// ResendConfig holds Resend API credentials.
type ResendConfig struct {
	APIKey string `yaml:"api_key"`
}

// TLSConfig controls how the portal presents to clients.
type TLSConfig struct {
	Mode     string `yaml:"mode"`      // "native" | "behind_proxy"
	CertPath string `yaml:"cert_path"` // required when mode == "native"
	KeyPath  string `yaml:"key_path"`  // required when mode == "native"
}

// LogConfig controls structured logging output.
// Level is an integer: -4=DEBUG, 0=INFO, 4=WARN, 8=ERROR (slog.Level).
// YAML accepts both integer and string representations:
//
//	log.level: -4       # DEBUG
//	log.level: "debug"  # also DEBUG (case-insensitive)
type LogConfig struct {
	Format string     `yaml:"format"` // "json" | "text"
	Level  slog.Level `yaml:"level"`  // integer or level name; see package doc
}

// UnmarshalYAML handles both integer and string level values in YAML config.
// String names follow slog's own convention (e.g. "DEBUG", "INFO", "WARN",
// "ERROR") and are accepted case-insensitively. Integer values are used
// directly as slog.Level, allowing fine-grained levels like -4, 0, 4, 8.
func (lc *LogConfig) UnmarshalYAML(value *yaml.Node) error {
	// Use an intermediate struct to capture the raw level node.
	type plain struct {
		Format string    `yaml:"format"`
		Level  yaml.Node `yaml:"level"`
	}
	var p plain
	if err := value.Decode(&p); err != nil {
		return err
	}
	lc.Format = p.Format
	if p.Level.Value == "" {
		// level key absent; leave Level at zero value (INFO = 0).
		return nil
	}
	// Try integer first.
	if n, err := strconv.Atoi(p.Level.Value); err == nil {
		lc.Level = slog.Level(n)
		return nil
	}
	// Fall back to slog's own text parsing (handles DEBUG/INFO/WARN/ERROR).
	if err := lc.Level.UnmarshalText([]byte(p.Level.Value)); err != nil {
		return fmt.Errorf("config: log.level %q: %w", p.Level.Value, err)
	}
	return nil
}

// Load reads configuration from an optional YAML file at path, then
// overlays environment variables. Returns validated defaults when path
// is empty and no env vars are set. Returns an error if the file cannot
// be read/parsed or if the resulting config fails validation.
func Load(path string) (Config, error) {
	cfg := defaults()
	if path != "" {
		b, err := os.ReadFile(path)
		if err != nil {
			return cfg, fmt.Errorf("config: read %s: %w", path, err)
		}
		if err := yaml.Unmarshal(b, &cfg); err != nil {
			return cfg, fmt.Errorf("config: parse %s: %w", path, err)
		}
	}
	if err := applyEnv(&cfg); err != nil {
		return cfg, err
	}
	if err := cfg.validate(); err != nil {
		return cfg, err
	}
	return cfg, nil
}

// defaults returns the baseline configuration that matches docs/SELF_HOST.md
// Configuration table.
func defaults() Config {
	return Config{
		Bind:      ":8443",
		DBDriver:  "sqlite",
		DBDSN:     "./jamsesh.db",
		PortalURL: "http://localhost:8443",
		TLS:       TLSConfig{Mode: "behind_proxy"},
		Log:       LogConfig{Format: "json", Level: slog.LevelInfo},
		Storage:   "./storage",
		Email: EmailConfig{
			Provider: "smtp",
			SMTP: SMTPConfig{
				Host:    "localhost",
				Port:    587,
				TLSMode: "mandatory",
			},
		},
		Git: GitConfig{
			MaxPackBytes: 52428800, // 50 MiB
		},
		DB: DBConfig{
			MaxOpenConns:    25,
			MaxIdleConns:    5,
			ConnMaxLifetime: 30 * time.Minute,
		},
		ShutdownGraceSeconds:        30,
		DeployMode:                  "single",
		LeaseHeartbeatIntervalS:     10,
		LeaseRetentionDays:          30,
		LeaseRetentionIntervalHours: 1,
	}
}

// validate checks invariants that cannot be expressed as YAML types.
func (c Config) validate() error {
	switch c.TLS.Mode {
	case "native":
		if c.TLS.CertPath == "" || c.TLS.KeyPath == "" {
			return fmt.Errorf("config: tls.mode=native requires tls.cert_path and tls.key_path")
		}
	case "behind_proxy":
		// valid; no TLS material needed
	default:
		return fmt.Errorf("config: tls.mode must be \"native\" or \"behind_proxy\", got %q", c.TLS.Mode)
	}
	switch c.DBDriver {
	case "sqlite", "postgres":
		// valid
	default:
		return fmt.Errorf("config: db_driver must be \"sqlite\" or \"postgres\", got %q", c.DBDriver)
	}
	if c.ShutdownGraceSeconds <= 0 {
		return fmt.Errorf("config: shutdown_grace_s must be a positive integer, got %d", c.ShutdownGraceSeconds)
	}
	switch c.DeployMode {
	case "single", "clustered":
		// valid
	default:
		return fmt.Errorf("config: deploy_mode must be \"single\" or \"clustered\", got %q", c.DeployMode)
	}
	if c.DeployMode == "clustered" && c.DBDriver == "sqlite" {
		return fmt.Errorf("config: deploy_mode=clustered requires db_driver=postgres; SQLite does not support distributed leases")
	}
	if c.LeaseHeartbeatIntervalS <= 0 {
		return fmt.Errorf("config: lease_heartbeat_interval_s must be a positive integer, got %d", c.LeaseHeartbeatIntervalS)
	}
	if c.LeaseRetentionDays <= 0 {
		return fmt.Errorf("config: lease_retention_days must be a positive integer, got %d", c.LeaseRetentionDays)
	}
	if c.LeaseRetentionIntervalHours <= 0 {
		return fmt.Errorf("config: lease_retention_interval_hours must be a positive integer, got %d", c.LeaseRetentionIntervalHours)
	}
	return nil
}

// applyEnv overlays environment variables onto cfg. Only non-empty env
// values take effect; missing vars leave the existing value unchanged.
// Returns an error if a _FILE secret variable is set but unreadable.
func applyEnv(c *Config) error {
	if v := os.Getenv("JAMSESH_BIND"); v != "" {
		c.Bind = v
	}
	if v := os.Getenv("JAMSESH_DB_DRIVER"); v != "" {
		c.DBDriver = v
	}
	v, err := readEnvOrFile("JAMSESH_DB_DSN")
	if err != nil {
		return err
	}
	if v != "" {
		c.DBDSN = v
	}
	if v := os.Getenv("JAMSESH_TLS_MODE"); v != "" {
		c.TLS.Mode = v
	}
	if v := os.Getenv("JAMSESH_TLS_CERT"); v != "" {
		c.TLS.CertPath = v
	}
	if v := os.Getenv("JAMSESH_TLS_KEY"); v != "" {
		c.TLS.KeyPath = v
	}
	if v := os.Getenv("JAMSESH_LOG_FORMAT"); v != "" {
		c.Log.Format = v
	}
	if v := os.Getenv("JAMSESH_LOG_LEVEL"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			c.Log.Level = slog.Level(n)
		}
	}
	if v := os.Getenv("JAMSESH_STORAGE"); v != "" {
		c.Storage = v
	}
	if v := os.Getenv("JAMSESH_PORTAL_URL"); v != "" {
		c.PortalURL = v
	}
	if err := applyEmailEnv(&c.Email); err != nil {
		return err
	}
	if err := applyOAuthEnv(&c.OAuth); err != nil {
		return err
	}
	applyGitEnv(&c.Git)
	applyDBEnv(&c.DB)
	if v := os.Getenv("JAMSESH_SHUTDOWN_GRACE_S"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			c.ShutdownGraceSeconds = n
		}
	}
	applyLeaseEnv(c)
	return nil
}

// applyLeaseEnv overlays lease-related environment variables.
func applyLeaseEnv(c *Config) {
	if v := os.Getenv("JAMSESH_DEPLOY_MODE"); v != "" {
		c.DeployMode = v
	}
	if v := os.Getenv("JAMSESH_LEASE_HEARTBEAT_INTERVAL_S"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			c.LeaseHeartbeatIntervalS = n
		}
	}
	if v := os.Getenv("JAMSESH_LEASE_RETENTION_DAYS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			c.LeaseRetentionDays = n
		}
	}
	if v := os.Getenv("JAMSESH_LEASE_RETENTION_INTERVAL_HOURS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			c.LeaseRetentionIntervalHours = n
		}
	}
}

// applyGitEnv overlays git-policy environment variables.
func applyGitEnv(g *GitConfig) {
	if v := os.Getenv("JAMSESH_GIT_MAX_PACK_BYTES"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			g.MaxPackBytes = n
		}
	}
}

// applyDBEnv overlays database connection pool environment variables.
func applyDBEnv(d *DBConfig) {
	if v := os.Getenv("JAMSESH_DB_MAX_OPEN_CONNS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			d.MaxOpenConns = n
		}
	}
	if v := os.Getenv("JAMSESH_DB_MAX_IDLE_CONNS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			d.MaxIdleConns = n
		}
	}
	if v := os.Getenv("JAMSESH_DB_CONN_MAX_LIFETIME"); v != "" {
		if dur, err := time.ParseDuration(v); err == nil {
			d.ConnMaxLifetime = dur
		}
	}
}

// applyOAuthEnv overlays OAuth-related environment variables.
// Returns an error if a _FILE secret variable is set but unreadable.
func applyOAuthEnv(o *OAuthConfig) error {
	if v := os.Getenv("JAMSESH_OAUTH_GITHUB_CLIENT_ID"); v != "" {
		o.GitHub.ClientID = v
	}
	v, err := readEnvOrFile("JAMSESH_OAUTH_GITHUB_CLIENT_SECRET")
	if err != nil {
		return err
	}
	if v != "" {
		o.GitHub.ClientSecret = v
	}
	if v := os.Getenv("JAMSESH_OAUTH_GITHUB_BASE_URL"); v != "" {
		o.GitHub.BaseURL = v
	}
	return nil
}

// applyEmailEnv overlays email-related environment variables.
// Returns an error if a _FILE secret variable is set but unreadable.
func applyEmailEnv(e *EmailConfig) error {
	if v := os.Getenv("JAMSESH_EMAIL_PROVIDER"); v != "" {
		e.Provider = v
	}
	if v := os.Getenv("JAMSESH_EMAIL_FROM"); v != "" {
		e.From = v
	}
	if v := os.Getenv("JAMSESH_EMAIL_SMTP_HOST"); v != "" {
		e.SMTP.Host = v
	}
	if v := os.Getenv("JAMSESH_EMAIL_SMTP_PORT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			e.SMTP.Port = n
		}
	}
	if v := os.Getenv("JAMSESH_EMAIL_SMTP_USER"); v != "" {
		e.SMTP.User = v
	}
	smtpPass, err := readEnvOrFile("JAMSESH_EMAIL_SMTP_PASS")
	if err != nil {
		return err
	}
	if smtpPass != "" {
		e.SMTP.Pass = smtpPass
	}
	if v := os.Getenv("JAMSESH_EMAIL_SMTP_TLS"); v != "" {
		e.SMTP.TLSMode = v
	}
	sendgridKey, err := readEnvOrFile("JAMSESH_EMAIL_SENDGRID_API_KEY")
	if err != nil {
		return err
	}
	if sendgridKey != "" {
		e.SendGrid.APIKey = sendgridKey
	}
	postmarkToken, err := readEnvOrFile("JAMSESH_EMAIL_POSTMARK_SERVER_TOKEN")
	if err != nil {
		return err
	}
	if postmarkToken != "" {
		e.Postmark.ServerToken = postmarkToken
	}
	if v := os.Getenv("JAMSESH_EMAIL_POSTMARK_MESSAGE_STREAM"); v != "" {
		e.Postmark.MessageStream = v
	}
	resendKey, err := readEnvOrFile("JAMSESH_EMAIL_RESEND_API_KEY")
	if err != nil {
		return err
	}
	if resendKey != "" {
		e.Resend.APIKey = resendKey
	}
	return nil
}
