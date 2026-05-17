package pusherr_test

import (
	"encoding/json"
	"testing"

	"jamsesh/cmd/jamsesh/pusherr"
)

func jsonBody(t *testing.T, code, message string, details map[string]any) []byte {
	t.Helper()
	m := map[string]any{"error": code, "message": message}
	if details != nil {
		m["details"] = details
	}
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("jsonBody: %v", err)
	}
	return b
}

func TestClassify_networkError(t *testing.T) {
	r := pusherr.Classify(0, nil)
	if r.Class != pusherr.Transient {
		t.Errorf("network error: class = %v, want Transient", r.Class)
	}
}

func TestClassify_ok(t *testing.T) {
	for _, status := range []int{200, 201, 204} {
		r := pusherr.Classify(status, nil)
		if r.Class != pusherr.OK {
			t.Errorf("status %d: class = %v, want OK", status, r.Class)
		}
	}
}

func TestClassify_5xx_transient(t *testing.T) {
	for _, status := range []int{500, 502, 503, 504} {
		r := pusherr.Classify(status, nil)
		if r.Class != pusherr.Transient {
			t.Errorf("status %d: class = %v, want Transient", status, r.Class)
		}
	}
}

func TestClassify_4xx_permanentPushCode(t *testing.T) {
	codes := []string{
		"push.scope_violation",
		"push.ref_namespace_violation",
		"push.missing_trailer",
		"push.size_limit",
		"push.force_push_rejected",
	}
	for _, code := range codes {
		body := jsonBody(t, code, "rejected", nil)
		r := pusherr.Classify(422, body)
		if r.Class != pusherr.Permanent {
			t.Errorf("code %q: class = %v, want Permanent", code, r.Class)
		}
		if r.Code != code {
			t.Errorf("code %q: Result.Code = %q, want same", code, r.Code)
		}
	}
}

func TestClassify_4xx_permanentAuthCodes(t *testing.T) {
	codes := []string{
		"auth.invalid_token",
		"auth.insufficient_permission",
		"auth.expired_token",
	}
	for _, code := range codes {
		body := jsonBody(t, code, "unauthorized", nil)
		r := pusherr.Classify(401, body)
		if r.Class != pusherr.Permanent {
			t.Errorf("code %q: class = %v, want Permanent", code, r.Class)
		}
	}
}

func TestClassify_4xx_genericPermanent(t *testing.T) {
	// Any 4xx without a known code is still Permanent (safer default).
	body := jsonBody(t, "unknown.code", "some error", nil)
	r := pusherr.Classify(400, body)
	if r.Class != pusherr.Permanent {
		t.Errorf("generic 4xx: class = %v, want Permanent", r.Class)
	}
}

func TestClassify_5xx_withBody(t *testing.T) {
	body := jsonBody(t, "internal", "server blew up", nil)
	r := pusherr.Classify(500, body)
	if r.Class != pusherr.Transient {
		t.Errorf("5xx with body: class = %v, want Transient", r.Class)
	}
	if r.Message != "server blew up" {
		t.Errorf("5xx with body: Message = %q, want %q", r.Message, "server blew up")
	}
}

func TestClassify_details(t *testing.T) {
	details := map[string]any{"paths": []any{"foo/bar.go"}}
	body := jsonBody(t, "push.scope_violation", "out of scope", details)
	r := pusherr.Classify(422, body)
	if r.Class != pusherr.Permanent {
		t.Errorf("class = %v, want Permanent", r.Class)
	}
	if r.Details == nil {
		t.Error("Details is nil, want non-nil")
	}
}

func TestClassify_malformedBody(t *testing.T) {
	// Malformed JSON body should not panic; code/message remain empty.
	r := pusherr.Classify(500, []byte("not json"))
	if r.Class != pusherr.Transient {
		t.Errorf("5xx malformed body: class = %v, want Transient", r.Class)
	}
	if r.Code != "" {
		t.Errorf("Code should be empty for malformed body, got %q", r.Code)
	}
}
