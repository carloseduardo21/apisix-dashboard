package server

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/carloseduardo21/apisix-dashboard/internal/auth"
)

func TestLimiter(t *testing.T) {
	t.Parallel()
	l := newLimiter(6) // burst 6, one token every 10 s
	now := time.Unix(1_000_000, 0)
	l.now = func() time.Time { return now }

	for i := range 6 {
		if ok, _, _ := l.allow("a"); !ok {
			t.Fatalf("attempt %d should be allowed", i+1)
		}
	}
	ok, retry, first := l.allow("a")
	if ok || !first || retry <= 0 || retry > 10*time.Second {
		t.Fatalf("7th attempt: ok=%v first=%v retry=%v", ok, first, retry)
	}
	if _, _, first := l.allow("a"); first {
		t.Error("only the first rejection is reported as first")
	}
	if ok, _, _ := l.allow("b"); !ok {
		t.Error("other clients are not affected")
	}
	now = now.Add(10 * time.Second)
	if ok, _, _ := l.allow("a"); !ok {
		t.Error("a token is refilled after 10 s")
	}
	if ok, _, first := l.allow("a"); ok || !first {
		t.Errorf("after the refill is used: ok=%v first=%v", ok, first)
	}

	now = now.Add(time.Hour)
	l.sweep(now)
	if len(l.buckets) != 0 {
		t.Errorf("idle buckets should be swept, %d left", len(l.buckets))
	}
}

func TestClientKey(t *testing.T) {
	t.Parallel()
	tests := map[string]string{
		"203.0.113.5":         "203.0.113.5",
		"::ffff:203.0.113.5":  "203.0.113.5",
		"2001:db8:1:2::1":     "2001:db8:1:2::/64",
		"2001:db8:1:2:aaaa::": "2001:db8:1:2::/64",
		"not-an-ip":           "not-an-ip",
	}
	for in, want := range tests {
		if got := clientKey(in); got != want {
			t.Errorf("clientKey(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestLoginThrottle(t *testing.T) {
	t.Parallel()
	h := newHarness(t, func(o *Options, _ *auth.Config) { o.LoginRatePerMinute = 3 })
	body := `{"username":"nobody","password":"wrong-password-1"}`
	for i := range 3 {
		if rec := h.do(nil, http.MethodPost, "/api/auth/login", body); rec.Code != http.StatusUnauthorized {
			t.Fatalf("attempt %d: status %d, want 401", i+1, rec.Code)
		}
	}
	for range 2 {
		rec := h.do(nil, http.MethodPost, "/api/auth/login", body)
		if rec.Code != http.StatusTooManyRequests {
			t.Fatalf("status %d, want 429", rec.Code)
		}
		if rec.Header().Get("Retry-After") == "" {
			t.Error("missing Retry-After")
		}
		if b := decodeError(t, rec); b.Code != "auth.too_many_requests" || b.Params["seconds"] == "" {
			t.Errorf("unexpected body %+v", b)
		}
	}
	if n := strings.Count(h.audit.String(), `"action":"auth.throttled"`); n != 1 {
		t.Errorf("auth.throttled recorded %d times, want 1", n)
	}

	// Another client is not affected, and the MFA step shares the limit.
	h.remote = "192.0.2.99:40000"
	if rec := h.do(nil, http.MethodPost, "/api/auth/login", body); rec.Code != http.StatusUnauthorized {
		t.Errorf("other client: status %d, want 401", rec.Code)
	}
	for range 2 {
		h.do(nil, http.MethodPost, "/api/auth/mfa/verify", `{"code":"000000"}`)
	}
	if rec := h.do(nil, http.MethodPost, "/api/auth/mfa/verify", `{"code":"000000"}`); rec.Code != http.StatusTooManyRequests {
		t.Errorf("mfa verify: status %d, want 429", rec.Code)
	}
}
