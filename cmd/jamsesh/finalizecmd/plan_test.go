package finalizecmd

import "testing"

func TestParsePlanID(t *testing.T) {
	cases := []struct {
		name    string
		input   string
		wantSID string
		wantLID string
		wantErr bool
	}{
		{"happy", "sess-abc:lock-xyz", "sess-abc", "lock-xyz", false},
		{"uuid:uuid", "550e8400-e29b-41d4-a716-446655440000:7c9e6679-7425-40de-944b-e07fc1f90ae7",
			"550e8400-e29b-41d4-a716-446655440000", "7c9e6679-7425-40de-944b-e07fc1f90ae7", false},
		{"missing colon", "sessabclock", "", "", true},
		{"empty session", ":lockxyz", "", "", true},
		{"empty lock", "sessabc:", "", "", true},
		{"empty", "", "", "", true},
		{"trimmed whitespace", "  sess:lock  ", "sess", "lock", false},
		{"multiple colons split on first", "sess:lock:extra", "sess", "lock:extra", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := parsePlanID(c.input)
			if c.wantErr {
				if err == nil {
					t.Fatalf("expected error, got: %+v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.SessionID != c.wantSID {
				t.Errorf("SessionID = %q, want %q", got.SessionID, c.wantSID)
			}
			if got.LockID != c.wantLID {
				t.Errorf("LockID = %q, want %q", got.LockID, c.wantLID)
			}
		})
	}
}
