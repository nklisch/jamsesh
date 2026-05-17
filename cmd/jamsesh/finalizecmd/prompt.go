package finalizecmd

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
)

// stdin is the input source for confirm(). Overridable in tests.
var stdin io.Reader = os.Stdin

// confirmFn is the function-typed prompt the orchestration layer
// accepts so the pre-flight code is testable without stdin plumbing.
type confirmFn func(prompt string, defaultYes bool) (bool, error)

// confirm prints `prompt [Y/n]` (or `[y/N]`), reads a line from the
// stdin var, and returns the boolean answer. Empty input falls back to
// defaultYes. Invalid input re-prompts up to three times before
// returning an error.
func confirm(out io.Writer, prompt string, defaultYes bool) (bool, error) {
	suffix := "[Y/n]"
	if !defaultYes {
		suffix = "[y/N]"
	}
	reader := bufio.NewReader(stdin)
	for attempt := 0; attempt < 3; attempt++ {
		fmt.Fprintf(out, "%s %s ", prompt, suffix)
		line, err := reader.ReadString('\n')
		if err != nil {
			// EOF on the first read with empty buffer → treat as the
			// default; tests that pass strings.NewReader without a
			// trailing newline rely on this.
			line = strings.TrimRight(line, "\r\n")
			if line == "" {
				return defaultYes, nil
			}
			// EOF mid-line: fall through to the parsing branch with
			// whatever we have.
		}
		ans := strings.ToLower(strings.TrimSpace(line))
		switch ans {
		case "":
			return defaultYes, nil
		case "y", "yes":
			return true, nil
		case "n", "no":
			return false, nil
		}
		fmt.Fprintf(out, "Please answer y or n.\n")
	}
	return false, fmt.Errorf("no valid answer after 3 attempts")
}
