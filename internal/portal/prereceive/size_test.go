package prereceive

import "testing"

func TestCheckPackSize(t *testing.T) {
	cases := []struct {
		name      string
		packBytes int64
		maxBytes  int64
		wantOK    bool
		wantCode  string
	}{
		{
			name:      "under limit",
			packBytes: 1024,
			maxBytes:  52428800,
			wantOK:    true,
		},
		{
			name:      "exactly at limit",
			packBytes: 52428800,
			maxBytes:  52428800,
			wantOK:    true,
		},
		{
			name:      "one byte over limit",
			packBytes: 52428801,
			maxBytes:  52428800,
			wantOK:    false,
			wantCode:  CodeSizeLimit,
		},
		{
			name:      "zero bytes",
			packBytes: 0,
			maxBytes:  52428800,
			wantOK:    true,
		},
		{
			name:      "disabled check (maxBytes=0)",
			packBytes: 999999999,
			maxBytes:  0,
			wantOK:    true,
		},
		{
			name:      "disabled check (maxBytes negative)",
			packBytes: 999999999,
			maxBytes:  -1,
			wantOK:    true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r, ok := CheckPackSize(tc.packBytes, tc.maxBytes)
			if ok != tc.wantOK {
				t.Errorf("ok: got %v, want %v", ok, tc.wantOK)
			}
			if !ok {
				if r.Code != tc.wantCode {
					t.Errorf("code: got %q, want %q", r.Code, tc.wantCode)
				}
				if r.Message == "" {
					t.Error("rejection message should not be empty")
				}
				if r.Details == nil {
					t.Error("rejection details should not be nil")
				}
			}
		})
	}
}
