package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// fakeSleeper tracks total sleep duration and individual calls so tests can
// assert on polling cadence without incurring real wall-clock delays.
type fakeSleeper struct {
	calls    []time.Duration
	total    time.Duration
}

func (f *fakeSleeper) sleep(d time.Duration) {
	f.calls = append(f.calls, d)
	f.total += d
}

// TestDeviceFlowHappyPath: mock portal immediately returns an access token
// on the first poll.
func TestDeviceFlowHappyPath(t *testing.T) {
	stateDir := t.TempDir()
	t.Setenv("JAMSESH_DATA_DIR", stateDir)

	portal := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/auth/device/authorize":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(deviceAuthResponse{
				DeviceCode:      "dev-code-abc",
				UserCode:        "ABCD-1234",
				VerificationURI: "https://example.com/activate",
				ExpiresIn:       300,
				Interval:        1,
			})
		case "/api/auth/token":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(deviceTokenResponse{
				AccessToken:  "access-token-happy",
				RefreshToken: "refresh-token-happy",
				TokenType:    "Bearer",
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer portal.Close()

	fs := &fakeSleeper{}
	err := deviceFlow(context.Background(), portal.URL, fs.sleep)
	if err != nil {
		t.Fatalf("deviceFlow error: %v", err)
	}

	token, err := os.ReadFile(filepath.Join(stateDir, "token"))
	if err != nil {
		t.Fatalf("reading token: %v", err)
	}
	if strings.TrimSpace(string(token)) != "access-token-happy" {
		t.Errorf("token = %q, want %q", strings.TrimSpace(string(token)), "access-token-happy")
	}

	refresh, err := os.ReadFile(filepath.Join(stateDir, "refresh_token"))
	if err != nil {
		t.Fatalf("reading refresh_token: %v", err)
	}
	if strings.TrimSpace(string(refresh)) != "refresh-token-happy" {
		t.Errorf("refresh_token = %q, want %q", strings.TrimSpace(string(refresh)), "refresh-token-happy")
	}

	// Verify the fake sleeper was called at least once.
	if len(fs.calls) == 0 {
		t.Error("expected at least one sleep call (interval enforcement)")
	}
	// Sleep duration should match the interval returned by the server (1s).
	for i, d := range fs.calls {
		if d != 1*time.Second {
			t.Errorf("sleep call %d = %v, want 1s", i, d)
		}
	}
}

// TestDeviceFlowAuthorizationPending: server returns authorization_pending
// for two polls, then succeeds.
func TestDeviceFlowAuthorizationPending(t *testing.T) {
	stateDir := t.TempDir()
	t.Setenv("JAMSESH_DATA_DIR", stateDir)

	var pollCount atomic.Int32

	portal := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/auth/device/authorize":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(deviceAuthResponse{
				DeviceCode:      "dev-code-pending",
				UserCode:        "PEND-5678",
				VerificationURI: "https://example.com/activate",
				ExpiresIn:       300,
				Interval:        1,
			})
		case "/api/auth/token":
			n := pollCount.Add(1)
			w.Header().Set("Content-Type", "application/json")
			if n <= 2 {
				_ = json.NewEncoder(w).Encode(deviceTokenResponse{
					Error: errAuthorizationPending,
				})
			} else {
				_ = json.NewEncoder(w).Encode(deviceTokenResponse{
					AccessToken:  "access-after-pending",
					RefreshToken: "refresh-after-pending",
				})
			}
		default:
			http.NotFound(w, r)
		}
	}))
	defer portal.Close()

	fs := &fakeSleeper{}
	err := deviceFlow(context.Background(), portal.URL, fs.sleep)
	if err != nil {
		t.Fatalf("deviceFlow error: %v", err)
	}

	if pollCount.Load() != 3 {
		t.Errorf("expected 3 poll attempts, got %d", pollCount.Load())
	}
	if len(fs.calls) != 3 {
		t.Errorf("expected 3 sleep calls, got %d", len(fs.calls))
	}
}

// TestDeviceFlowSlowDown: server returns slow_down once; the client must
// add 5 s to the interval.
func TestDeviceFlowSlowDown(t *testing.T) {
	stateDir := t.TempDir()
	t.Setenv("JAMSESH_DATA_DIR", stateDir)

	var pollCount atomic.Int32

	portal := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/auth/device/authorize":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(deviceAuthResponse{
				DeviceCode:      "dev-code-slow",
				UserCode:        "SLOW-ABCD",
				VerificationURI: "https://example.com/activate",
				ExpiresIn:       300,
				Interval:        2,
			})
		case "/api/auth/token":
			n := pollCount.Add(1)
			w.Header().Set("Content-Type", "application/json")
			switch n {
			case 1:
				_ = json.NewEncoder(w).Encode(deviceTokenResponse{Error: errSlowDown})
			case 2:
				_ = json.NewEncoder(w).Encode(deviceTokenResponse{
					AccessToken: "access-slow-ok",
				})
			default:
				_ = json.NewEncoder(w).Encode(deviceTokenResponse{Error: "unexpected"})
			}
		default:
			http.NotFound(w, r)
		}
	}))
	defer portal.Close()

	fs := &fakeSleeper{}
	err := deviceFlow(context.Background(), portal.URL, fs.sleep)
	if err != nil {
		t.Fatalf("deviceFlow error: %v", err)
	}

	// First sleep: 2s (initial interval). After slow_down, interval becomes 7s.
	// Second sleep: 7s.
	if len(fs.calls) < 2 {
		t.Fatalf("expected at least 2 sleep calls, got %d", len(fs.calls))
	}
	if fs.calls[0] != 2*time.Second {
		t.Errorf("first sleep = %v, want 2s", fs.calls[0])
	}
	if fs.calls[1] != 7*time.Second {
		t.Errorf("second sleep (after slow_down) = %v, want 7s (2s + 5s)", fs.calls[1])
	}
}

// TestDeviceFlowExpiredToken: server returns expired_token; deviceFlow must
// return an error without writing tokens.
func TestDeviceFlowExpiredToken(t *testing.T) {
	stateDir := t.TempDir()
	t.Setenv("JAMSESH_DATA_DIR", stateDir)

	portal := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/auth/device/authorize":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(deviceAuthResponse{
				DeviceCode:      "dev-code-expired",
				UserCode:        "EXPD-1234",
				VerificationURI: "https://example.com/activate",
				ExpiresIn:       300,
				Interval:        1,
			})
		case "/api/auth/token":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(deviceTokenResponse{
				Error: errExpiredToken,
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer portal.Close()

	fs := &fakeSleeper{}
	err := deviceFlow(context.Background(), portal.URL, fs.sleep)
	if err == nil {
		t.Fatal("expected error on expired_token, got nil")
	}
	if !strings.Contains(err.Error(), "expired") {
		t.Errorf("error = %q, want it to mention 'expired'", err.Error())
	}

	// No token file should have been written.
	if _, statErr := os.Stat(filepath.Join(stateDir, "token")); !os.IsNotExist(statErr) {
		t.Error("token file should not exist after expired_token error")
	}
}

// TestDeviceFlowAccessDenied: server returns access_denied.
func TestDeviceFlowAccessDenied(t *testing.T) {
	stateDir := t.TempDir()
	t.Setenv("JAMSESH_DATA_DIR", stateDir)

	portal := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/auth/device/authorize":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(deviceAuthResponse{
				DeviceCode:      "dev-code-denied",
				UserCode:        "DENY-4321",
				VerificationURI: "https://example.com/activate",
				ExpiresIn:       300,
				Interval:        1,
			})
		case "/api/auth/token":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(deviceTokenResponse{
				Error: errAccessDenied,
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer portal.Close()

	fs := &fakeSleeper{}
	err := deviceFlow(context.Background(), portal.URL, fs.sleep)
	if err == nil {
		t.Fatal("expected error on access_denied, got nil")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "denied") {
		t.Errorf("error = %q, want it to mention 'denied'", err.Error())
	}
}

// TestDeviceFlowContextCancellation: cancelling the context stops polling.
func TestDeviceFlowContextCancellation(t *testing.T) {
	stateDir := t.TempDir()
	t.Setenv("JAMSESH_DATA_DIR", stateDir)

	// A portal that always returns authorization_pending — we'll cancel the
	// context before it would ever succeed.
	portal := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/auth/device/authorize":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(deviceAuthResponse{
				DeviceCode:      "dev-code-cancel",
				UserCode:        "CANC-0000",
				VerificationURI: "https://example.com/activate",
				ExpiresIn:       300,
				Interval:        1,
			})
		case "/api/auth/token":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(deviceTokenResponse{
				Error: errAuthorizationPending,
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer portal.Close()

	ctx, cancel := context.WithCancel(context.Background())

	// Use a sleep that cancels after the first call.
	callCount := 0
	fakeSleep := func(d time.Duration) {
		callCount++
		if callCount >= 2 {
			cancel()
		}
	}

	err := deviceFlow(ctx, portal.URL, fakeSleep)
	// After context cancellation the next HTTP request should fail.
	if err == nil {
		t.Error("expected error after context cancellation, got nil")
	}
}

// TestDeviceAuthorizationRequest verifies the shape of the request sent to
// /api/auth/device/authorize.
func TestDeviceAuthorizationRequest(t *testing.T) {
	var gotForm url.Values
	portal := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		gotForm = r.Form
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(deviceAuthResponse{
			DeviceCode:      "dc",
			UserCode:        "UC12",
			VerificationURI: "https://x.example/activate",
			ExpiresIn:       300,
			Interval:        5,
		})
	}))
	defer portal.Close()

	resp, err := requestDeviceAuthorization(context.Background(), portal.URL)
	if err != nil {
		t.Fatalf("requestDeviceAuthorization: %v", err)
	}

	if gotForm.Get("client_id") == "" {
		t.Error("request missing client_id")
	}
	if gotForm.Get("scope") == "" {
		t.Error("request missing scope")
	}

	// Response fields are mapped correctly.
	if resp.DeviceCode != "dc" {
		t.Errorf("DeviceCode = %q, want %q", resp.DeviceCode, "dc")
	}
	if resp.Interval != 5 {
		t.Errorf("Interval = %d, want 5", resp.Interval)
	}
}

// TestPollDeviceTokenRequest verifies the shape of the token poll request.
func TestPollDeviceTokenRequest(t *testing.T) {
	var gotForm url.Values
	portal := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		gotForm = r.Form
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(deviceTokenResponse{
			AccessToken: "at",
		})
	}))
	defer portal.Close()

	tok, err := pollDeviceToken(context.Background(), portal.URL, "my-device-code")
	if err != nil {
		t.Fatalf("pollDeviceToken: %v", err)
	}

	if tok.AccessToken != "at" {
		t.Errorf("AccessToken = %q, want %q", tok.AccessToken, "at")
	}
	if gotForm.Get("grant_type") != "urn:ietf:params:oauth:grant-type:device_code" {
		t.Errorf("grant_type = %q", gotForm.Get("grant_type"))
	}
	if gotForm.Get("device_code") != "my-device-code" {
		t.Errorf("device_code = %q, want %q", gotForm.Get("device_code"), "my-device-code")
	}
	if gotForm.Get("client_id") == "" {
		t.Error("request missing client_id")
	}
}
