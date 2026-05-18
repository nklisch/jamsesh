package automerger

import (
	"testing"
)

// ---------------------------------------------------------------------------
// parseSourceRefOwner (tested indirectly via computeAddressedTo, but let's
// also test the exported behaviour through the addressing function)
// ---------------------------------------------------------------------------

func TestComputeAddressedTo_SourceRefOwner(t *testing.T) {
	// Build a repo with a single commit on the draft branch (no conflicted files).
	repo, dir := initRepo(t)
	base := commitFiles(t, repo, dir, nil, map[string][]byte{"file.txt": []byte("base\n")}, "base")

	sourceRef := "refs/heads/jam/sess-1/alice/my-feature"
	addressed, err := computeAddressedTo(repo, base.Hash, nil, sourceRef)
	if err != nil {
		t.Fatalf("computeAddressedTo: %v", err)
	}
	if len(addressed) != 1 || addressed[0] != "@alice/my-feature" {
		t.Errorf("addressed_to: got %v, want [@alice/my-feature]", addressed)
	}
}

func TestComputeAddressedTo_ConflictedFileAuthor(t *testing.T) {
	// Build a draft history:
	//   base → commit-by-alice (modifies file.txt)
	repo, dir := initRepo(t)
	base := commitFiles(t, repo, dir, nil, map[string][]byte{"file.txt": []byte("base\n")}, "base")
	aliceCommit := commitFilesWithMessage(t, repo, dir, base, map[string][]byte{
		"file.txt": []byte("alice edits this\n"),
	}, "alice: edit file.txt\n\nSigned-off-by: Alice <alice@example.com>\n")

	_ = aliceCommit

	// The draftTip is aliceCommit; conflict is in "file.txt".
	conflicts := []Conflict{{File: "file.txt"}}
	sourceRef := "refs/heads/jam/sess-1/bob/fix"

	addressed, err := computeAddressedTo(repo, aliceCommit.Hash, conflicts, sourceRef)
	if err != nil {
		t.Fatalf("computeAddressedTo: %v", err)
	}

	// Should include @bob/fix (source-ref owner) + test@jamsesh.test (alice's
	// commit email from the test helper).
	foundBob := false
	foundAlice := false
	for _, a := range addressed {
		if a == "@bob/fix" {
			foundBob = true
		}
		if a == "test@jamsesh.test" {
			foundAlice = true
		}
	}
	if !foundBob {
		t.Errorf("addressed_to %v does not contain @bob/fix", addressed)
	}
	if !foundAlice {
		t.Errorf("addressed_to %v does not contain test@jamsesh.test", addressed)
	}
}

func TestComputeAddressedTo_UnknownRef(t *testing.T) {
	// A sourceRef that doesn't match jam/<sess>/<user>/<branch> should produce
	// an empty owner token (no panic, no empty string added to set).
	repo, dir := initRepo(t)
	base := commitFiles(t, repo, dir, nil, map[string][]byte{"file.txt": []byte("base\n")}, "base")

	addressed, err := computeAddressedTo(repo, base.Hash, nil, "refs/heads/not-a-jam-ref")
	if err != nil {
		t.Fatalf("computeAddressedTo: %v", err)
	}
	for _, a := range addressed {
		if a == "" || a == "@" {
			t.Errorf("unexpected empty/bare-at token in addressed_to: %v", addressed)
		}
	}
}
