// Package buildinfo carries link-time-injected release identifiers.
//
// Values are populated via -ldflags at build time:
//
//	go build -ldflags "-X jamsesh/internal/buildinfo.Version=v1.2.3 \
//	                   -X jamsesh/internal/buildinfo.Commit=abc1234"
//
// In development (go run ./cmd/...) both vars read their compile-time
// defaults so code paths that call String() always return something
// meaningful without special-casing.
package buildinfo

// Version is the release tag (e.g. "v1.2.3") injected at build time.
// Default: "dev".
var Version = "dev"

// Commit is the full git SHA injected at build time.
// Default: "unknown".
var Commit = "unknown"

// String returns a human-readable version+commit string suitable for
// --version flags and /healthz responses.
// Format: "<version> (<commit>)"
func String() string {
	return Version + " (" + Commit + ")"
}
