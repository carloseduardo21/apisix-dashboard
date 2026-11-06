package server

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/carloseduardo21/apisix-dashboard/internal/secure"
)

// Background requests (UI polling without user activity) must not extend the
// idle timeout, so an unattended tab is logged out (PCI DSS 8.2.8).
func TestBackgroundRequestsDoNotExtendIdleTimeout(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	s := h.login("viewer1", "viewer")
	ctx := context.Background()
	hash := secure.HashToken(s.cookie)

	setLastSeen := func(ago time.Duration) time.Time {
		t.Helper()
		sess, err := h.store.GetSession(ctx, hash)
		if err != nil {
			t.Fatal(err)
		}
		sess.LastSeen = time.Now().Add(-ago).Truncate(time.Second)
		if err := h.store.UpdateSession(ctx, sess); err != nil {
			t.Fatal(err)
		}
		return sess.LastSeen
	}
	lastSeen := func() time.Time {
		t.Helper()
		sess, err := h.store.GetSession(ctx, hash)
		if err != nil {
			t.Fatal(err)
		}
		return sess.LastSeen
	}

	// Idle for 10 minutes: background polling is served but does not touch.
	before := setLastSeen(10 * time.Minute)
	if rec := h.do(&s, http.MethodGet, "/api/auth/me", "", backgroundHeader, "1"); rec.Code != http.StatusOK {
		t.Fatalf("background request: status %d", rec.Code)
	}
	if got := lastSeen(); !got.Equal(before) {
		t.Errorf("background request moved LastSeen from %v to %v", before, got)
	}

	// A user request does extend the session.
	if rec := h.do(&s, http.MethodGet, "/api/auth/me", ""); rec.Code != http.StatusOK {
		t.Fatalf("user request: status %d", rec.Code)
	}
	if got := lastSeen(); time.Since(got) > time.Minute {
		t.Errorf("user request did not refresh LastSeen (%v)", got)
	}

	// Past the idle timeout, polling gets the session expired.
	setLastSeen(16 * time.Minute)
	rec := h.do(&s, http.MethodGet, "/api/auth/me", "", backgroundHeader, "1")
	if rec.Code != http.StatusUnauthorized || decodeError(t, rec).Code != "auth.session_expired" {
		t.Errorf("expired session: status %d body %s", rec.Code, rec.Body.String())
	}
}
