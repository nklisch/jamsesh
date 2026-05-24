package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// readEnvOrFile returns the value for env var name, preferring the contents
// of the file named by name+"_FILE" when that variable is set. Trailing
// whitespace (including newlines) is trimmed from file contents.
//
// Precedence:
//   - name+"_FILE" is set → read file; return its trimmed contents (fail-fast
//     if the file is unreadable)
//   - name is set → return its value
//   - neither set → return ("", nil)
func readEnvOrFile(name string) (string, error) {
	if path := os.Getenv(name + "_FILE"); path != "" {
		b, err := os.ReadFile(path)
		if err != nil {
			return "", fmt.Errorf("config: read %s_FILE (%s): %w", name, path, err)
		}
		return strings.TrimRight(string(b), " \t\r\n"), nil
	}
	return os.Getenv(name), nil
}

// mustBePositive returns an error when v is not greater than zero.
// The error message matches "config: <name> must be positive (got <v>)".
func mustBePositive(name string, v int) error {
	if v <= 0 {
		return fmt.Errorf("config: %s must be positive (got %d)", name, v)
	}
	return nil
}

// mustBeNonNegative returns an error when v is negative.
// Used for fields that treat zero as a meaningful sentinel (e.g. "unlimited").
func mustBeNonNegative(name string, v int64) error {
	if v < 0 {
		return fmt.Errorf("config: %s must be zero or positive, got %d", name, v)
	}
	return nil
}

// readEnvString copies the env var key into *dst when the var is non-empty.
// No error path — absence of the var is a no-op.
func readEnvString(key string, dst *string) {
	if v := os.Getenv(key); v != "" {
		*dst = v
	}
}

// readEnvInt parses key as a base-10 integer and stores it in *dst.
// Silently ignores absent or unparseable values to preserve existing behavior.
func readEnvInt(key string, dst *int) {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			*dst = n
		}
	}
}

// readEnvInt64 parses key as a base-10 int64 and stores it in *dst.
// Silently ignores absent or unparseable values.
func readEnvInt64(key string, dst *int64) {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			*dst = n
		}
	}
}

// readEnvDuration parses key as a Go duration string and stores it in *dst.
// Silently ignores absent or unparseable values.
func readEnvDuration(key string, dst *time.Duration) {
	if v := os.Getenv(key); v != "" {
		if dur, err := time.ParseDuration(v); err == nil {
			*dst = dur
		}
	}
}
