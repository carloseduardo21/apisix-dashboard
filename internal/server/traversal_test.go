package server

import (
	"net/http"
	"strings"
	"testing"
)

// Percent-encoded dot segments are not cleaned by ServeMux. They must never
// reach the Admin API, where nginx would resolve them and escape the
// allowlisted resource (e.g. "secrets/../ssls" returns the SSL list).
func TestAPISIXRejectsDotSegments(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.admin.body = `{"total":1,"list":[{"value":{"id":"1","cert":"C","key":"encrypted-private-key"}}]}`
	viewer := h.login("viewer1", "viewer")
	operator := h.login("operator1", "operator")

	paths := []string{
		"/api/apisix/secrets%2F..%2Fssls",
		"/api/apisix/secrets/%2e%2e/ssls",
		"/api/apisix/secrets/%2E%2E/ssls",
		"/api/apisix/secrets/..%2Fssls",
		"/api/apisix/secrets/vault/%2e%2e",
		"/api/apisix/routes/%2e%2e",
		"/api/apisix/routes/%2e",
		"/api/apisix/consumers/%2e%2e/credentials/x",
		"/api/apisix/consumers/jack/credentials/%2e%2e",
	}
	for _, p := range paths {
		for _, tc := range []struct {
			name   string
			s      *session
			method string
			body   string
		}{
			{"viewer GET", &viewer, http.MethodGet, ""},
			{"operator PUT", &operator, http.MethodPut, `{"cert":"C","key":"K"}`},
			{"operator DELETE", &operator, http.MethodDelete, ""},
		} {
			rec := h.do(tc.s, tc.method, p, tc.body)
			if rec.Code != http.StatusNotFound {
				t.Errorf("%s %s: status %d, want 404 (body %s)", tc.name, p, rec.Code, rec.Body.String())
			}
			if strings.Contains(rec.Body.String(), "encrypted-private-key") {
				t.Errorf("%s %s: private key leaked: %s", tc.name, p, rec.Body.String())
			}
		}
	}
	if got := h.admin.requests(); len(got) != 0 {
		t.Errorf("no request may reach the Admin API, got %v", got)
	}
}

// SSL-shaped objects are masked whatever resource was requested.
func TestSSLKeysMaskedInEveryResponse(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.admin.body = `{"total":1,"list":[{"value":{"id":"1","cert":"C","key":"encrypted-private-key"}}]}`
	viewer := h.login("viewer1", "viewer")
	for _, p := range []string{"/api/apisix/ssls", "/api/apisix/secrets"} {
		rec := h.do(&viewer, http.MethodGet, p, "")
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: status %d", p, rec.Code)
		}
		if strings.Contains(rec.Body.String(), "encrypted-private-key") {
			t.Errorf("%s: private key not masked: %s", p, rec.Body.String())
		}
	}
}
