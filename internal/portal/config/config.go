// Package config loads and validates the portal server configuration.
// Config is sourced from an optional YAML file with env-var overrides.
//
// YAML keys:  bind, db_driver, db_dsn, tls.mode, tls.cert_path,
//
//	tls.key_path, log.format, log.level, storage
//
// Env vars:   JAMSESH_BIND, JAMSESH_DB_DRIVER, JAMSESH_DB_DSN,
//
//	JAMSESH_TLS_MODE, JAMSESH_TLS_CERT, JAMSESH_TLS_KEY,
//	JAMSESH_LOG_FORMAT, JAMSESH_LOG_LEVEL, JAMSESH_STORAGE
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

	"gopkg.in/yaml.v3"
)

// Config holds the full portal server configuration.
type Config struct {
	Bind     string    `yaml:"bind"`      // listen address, e.g. ":8443"
	DBDriver string    `yaml:"db_driver"` // "sqlite" | "postgres"
	DBDSN    string    `yaml:"db_dsn"`    // DSN appropriate for DBDriver
	TLS      TLSConfig `yaml:"tls"`
	Log      LogConfig `yaml:"log"`
	Storage  string    `yaml:"storage"` // path for bare git repos
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
	applyEnv(&cfg)
	if err := cfg.validate(); err != nil {
		return cfg, err
	}
	return cfg, nil
}

// defaults returns the baseline configuration that matches docs/SELF_HOST.md
// Configuration table.
func defaults() Config {
	return Config{
		Bind:     ":8443",
		DBDriver: "sqlite",
		DBDSN:    "./jamsesh.db",
		TLS:      TLSConfig{Mode: "behind_proxy"},
		Log:      LogConfig{Format: "json", Level: slog.LevelInfo},
		Storage:  "./storage",
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
	return nil
}

// applyEnv overlays environment variables onto cfg. Only non-empty env
// values take effect; missing vars leave the existing value unchanged.
func applyEnv(c *Config) {
	if v := os.Getenv("JAMSESH_BIND"); v != "" {
		c.Bind = v
	}
	if v := os.Getenv("JAMSESH_DB_DRIVER"); v != "" {
		c.DBDriver = v
	}
	if v := os.Getenv("JAMSESH_DB_DSN"); v != "" {
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
}
