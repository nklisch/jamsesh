package storage

import "path/filepath"

// RepoPath returns the absolute filesystem path for the bare repo of the
// given org+session pair:
//
//	<root>/orgs/<orgID>/sessions/<sessionID>.git
//
// This function is pure — it does not access the filesystem.
func (s *service) RepoPath(orgID, sessionID string) string {
	return filepath.Join(s.root, "orgs", orgID, "sessions", sessionID+".git")
}
