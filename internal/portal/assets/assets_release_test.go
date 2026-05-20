//go:build release

package assets

import (
	"io/fs"
	"testing"
)

// TestAssets_EmbeddedSPAIsBuilt fails the release pipeline if `make frontend-build`
// did not run before this test fires. Catches the v0.1.1 regression where the
// release workflow ran `go build` without `make frontend-build` and shipped a
// portal image containing only `dist/.gitkeep` (empty SPA).
//
// This test is gated by the `release` build tag so a fresh checkout (where dist/
// contains only .gitkeep) still passes `go test ./...`. The release workflow
// runs `go test -tags release ./internal/portal/assets/...` after frontend-build
// to enforce the guard.
func TestAssets_EmbeddedSPAIsBuilt(t *testing.T) {
	sub, err := fs.Sub(dist, "dist")
	if err != nil {
		t.Fatalf("fs.Sub(dist, \"dist\"): %v", err)
	}

	// Assertion 1: index.html exists and is non-empty.
	indexBytes, err := fs.ReadFile(sub, "index.html")
	if err != nil {
		t.Fatalf("expected dist/index.html to be embedded but it was not found — "+
			"did `make frontend-build` run before `go build`? (err: %v)", err)
	}
	if len(indexBytes) == 0 {
		t.Fatalf("dist/index.html is embedded but empty — frontend build likely failed")
	}

	// Assertion 2: at least one JS bundle exists in the embedded FS. Walks the
	// embed and looks for any *.js file (matches whatever path the Svelte/Vite
	// build emits — currently `assets/index-*.js`, but the test stays robust
	// if that path changes).
	var jsFound bool
	var jsPath string
	walkErr := fs.WalkDir(sub, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && len(path) > 3 && path[len(path)-3:] == ".js" {
			jsFound = true
			jsPath = path
			return fs.SkipAll
		}
		return nil
	})
	if walkErr != nil {
		t.Fatalf("walking embedded dist/: %v", walkErr)
	}
	if !jsFound {
		t.Fatalf("no JS bundle found in embedded dist/ — frontend build did not " +
			"produce JS output, did `make frontend-build` complete successfully?")
	}
	t.Logf("embedded SPA verified: dist/index.html (%d bytes), found JS bundle at %s", len(indexBytes), jsPath)
}
