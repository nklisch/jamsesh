package config

import (
	"fmt"
	"os"
	"strings"
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
