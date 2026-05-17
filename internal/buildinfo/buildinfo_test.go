package buildinfo_test

import (
	"strings"
	"testing"

	"jamsesh/internal/buildinfo"
)

// TestDefaultsNonEmpty ensures the compile-time defaults are non-empty
// strings. A blank Version or Commit would produce a confusing --version
// output and is most likely a build-flag misconfiguration.
func TestDefaultsNonEmpty(t *testing.T) {
	if buildinfo.Version == "" {
		t.Error("buildinfo.Version must not be empty; got empty string")
	}
	if buildinfo.Commit == "" {
		t.Error("buildinfo.Commit must not be empty; got empty string")
	}
}

// TestDefaultValues asserts the expected compile-time sentinel values.
// If -ldflags injection misfires during a release build, these sentinels
// will appear in the shipped binary — making the misconfiguration obvious.
func TestDefaultValues(t *testing.T) {
	if buildinfo.Version != "dev" {
		t.Errorf("expected default Version %q, got %q", "dev", buildinfo.Version)
	}
	if buildinfo.Commit != "unknown" {
		t.Errorf("expected default Commit %q, got %q", "unknown", buildinfo.Commit)
	}
}

// TestStringRoundTrip verifies that String() embeds both Version and Commit
// and that the format is "<version> (<commit>)".
func TestStringRoundTrip(t *testing.T) {
	got := buildinfo.String()

	if !strings.Contains(got, buildinfo.Version) {
		t.Errorf("String() %q does not contain Version %q", got, buildinfo.Version)
	}
	if !strings.Contains(got, buildinfo.Commit) {
		t.Errorf("String() %q does not contain Commit %q", got, buildinfo.Commit)
	}

	// Assert the canonical format.
	want := buildinfo.Version + " (" + buildinfo.Commit + ")"
	if got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
}

// TestStringWithInjectedValues simulates what a release build produces by
// temporarily overwriting the package vars (valid because they are exported).
func TestStringWithInjectedValues(t *testing.T) {
	orig := struct{ v, c string }{buildinfo.Version, buildinfo.Commit}
	t.Cleanup(func() {
		buildinfo.Version = orig.v
		buildinfo.Commit = orig.c
	})

	buildinfo.Version = "v1.2.3"
	buildinfo.Commit = "abc1234deadbeef"

	got := buildinfo.String()
	want := "v1.2.3 (abc1234deadbeef)"
	if got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
}
