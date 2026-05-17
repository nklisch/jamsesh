// Package binary provides a cached, shared build of the jamsesh server binary
// for e2e tests. Use Build(t) to get the absolute path to the binary; the first
// call per test binary invocation compiles it; subsequent calls return the same
// path without rebuilding.
//
// The binary is compiled from ./cmd/jamsesh at the repo root. Tests that need
// to inspect the CLI (e.g. version flags) can run it directly; tests that spin
// up a containerised portal do not need this fixture — they use the pre-built
// Docker image instead.
package binary

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

var (
	once    sync.Once
	binPath string
	onceErr error
)

// Build builds the jamsesh binary once per test binary invocation and returns
// its absolute path. Subsequent calls return the cached path without rebuilding.
//
// The output is placed in an OS temporary directory; no explicit cleanup is
// registered because the OS reclaims the directory on reboot and the path is
// stable for the lifetime of the test process.
func Build(t *testing.T) string {
	t.Helper()
	once.Do(func() {
		repoRoot, err := findRepoRoot()
		if err != nil {
			onceErr = err
			return
		}
		outDir, err := os.MkdirTemp("", "jamsesh-e2e-bin-*")
		if err != nil {
			onceErr = fmt.Errorf("binary: mkdir temp: %w", err)
			return
		}
		out := filepath.Join(outDir, "jamsesh")
		cmd := exec.Command("go", "build", "-o", out, "./cmd/jamsesh")
		cmd.Dir = repoRoot
		if combined, err := cmd.CombinedOutput(); err != nil {
			onceErr = fmt.Errorf("binary: go build ./cmd/jamsesh: %w\n%s", err, combined)
			return
		}
		binPath = out
	})
	if onceErr != nil {
		t.Fatalf("binary.Build: %v", onceErr)
	}
	return binPath
}

// findRepoRoot walks upward from the current working directory until it finds a
// go.mod whose module line is "module jamsesh". Returns an error if not found
// within ten directory levels.
func findRepoRoot() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("binary: getwd: %w", err)
	}
	d := cwd
	for range 10 {
		modPath := filepath.Join(d, "go.mod")
		data, err := os.ReadFile(modPath)
		if err == nil && strings.HasPrefix(string(data), "module jamsesh\n") {
			return d, nil
		}
		parent := filepath.Dir(d)
		if parent == d {
			break
		}
		d = parent
	}
	return "", fmt.Errorf("binary: could not find jamsesh repo root (started at %s)", cwd)
}
