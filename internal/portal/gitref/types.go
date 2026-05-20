// Package gitref provides shared ref-update types used across the portal's
// pre-receive and post-receive lifecycle packages.
package gitref

// RefUpdate describes one ref being updated in a push.
// OldSHA is empty when the ref is being created for the first time
// (i.e. "refs/heads/jam/<session>/<owner>/<branch>" is a new ref).
type RefUpdate struct {
	Ref    string // "refs/heads/jam/<session>/<owner>/<branch>"
	OldSHA string // empty if new ref
	NewSHA string
}
