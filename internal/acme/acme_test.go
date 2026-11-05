package acme

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/carloseduardo21/apisix-dashboard/internal/store"
)

func TestNormalizeDomains(t *testing.T) {
	got, err := NormalizeDomains([]string{" Example.COM. ", "*.example.com", "example.com", ""})
	if err != nil || strings.Join(got, ",") != "example.com,*.example.com" {
		t.Fatalf("got %v, %v", got, err)
	}
	for _, bad := range []string{"exa mple.com", "*.*.example.com", "localhost", "-a.com", "a.com/x", "*example.com"} {
		if _, err := NormalizeDomains([]string{bad}); err == nil {
			t.Errorf("%q should be rejected", bad)
		}
	}
	if _, err := NormalizeDomains(nil); err == nil {
		t.Error("empty list should be rejected")
	}
}

func TestDefaultSSLID(t *testing.T) {
	if id := DefaultSSLID("*.example.com.br"); id != "acme-wildcard-example-com-br" || !ValidSSLID(id) {
		t.Fatal(id)
	}
}

func TestDue(t *testing.T) {
	const day = 24 * time.Hour
	m := &Manager{Options: Options{RenewBefore: 30 * day}}
	now := time.Now()
	cases := []struct {
		name string
		c    store.Certificate
		want bool
	}{
		{"never issued", store.Certificate{Status: StatusPending}, true},
		{"valid", store.Certificate{Status: StatusValid, NotBefore: now.Add(-30 * day), NotAfter: now.Add(60 * day)}, false},
		{"expiring", store.Certificate{Status: StatusValid, NotBefore: now.Add(-70 * day), NotAfter: now.Add(20 * day)}, true},
		{"short-lived, fresh", store.Certificate{Status: StatusValid, NotBefore: now.Add(-day), NotAfter: now.Add(5 * day)}, false},
		{"short-lived, 2/3 used", store.Certificate{Status: StatusValid, NotBefore: now.Add(-4 * day), NotAfter: now.Add(2 * day)}, true},
		{"backing off", store.Certificate{Status: StatusError, NextAttemptAt: now.Add(time.Hour)}, false},
		{"retry due", store.Certificate{Status: StatusError, NextAttemptAt: now.Add(-time.Minute)}, true},
		{"running", store.Certificate{Status: StatusIssuing}, false},
	}
	for _, tc := range cases {
		if got := m.Due(&tc.c, now); got != tc.want {
			t.Errorf("%s: got %v", tc.name, got)
		}
	}
}

func TestBackoff(t *testing.T) {
	if backoff(1) != time.Hour || backoff(2) != 2*time.Hour || backoff(10) != 24*time.Hour {
		t.Fatal(backoff(1), backoff(2), backoff(10))
	}
}

func TestCloudflare(t *testing.T) {
	var created, deleted string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok" {
			w.WriteHeader(http.StatusForbidden)
			json.NewEncoder(w).Encode(map[string]any{"success": false, "errors": []any{map[string]any{"code": 9109, "message": "Invalid access token"}}})
			return
		}
		ok := func(result any) {
			json.NewEncoder(w).Encode(map[string]any{"success": true, "errors": []any{}, "result": result})
		}
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/zones":
			if r.URL.Query().Get("name") == "example.com" {
				ok([]any{map[string]any{"id": "z1", "name": "example.com", "status": "active", "name_servers": []string{"ns1.example"}}})
			} else {
				ok([]any{})
			}
		case r.Method == http.MethodPost && r.URL.Path == "/zones/z1/dns_records":
			var body map[string]any
			json.NewDecoder(r.Body).Decode(&body)
			created = body["name"].(string) + "=" + body["content"].(string)
			ok(map[string]any{"id": "r1"})
		case r.Method == http.MethodDelete && r.URL.Path == "/zones/z1/dns_records/r1":
			deleted = "r1"
			ok(map[string]any{"id": "r1"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	ctx := context.Background()

	cf := NewCloudflare(srv.URL, "tok", srv.Client())
	ref, err := cf.CreateTXT(ctx, "_acme-challenge.www.example.com", "v4lue")
	if err != nil || ref != "z1/r1" || created != "_acme-challenge.www.example.com=v4lue" {
		t.Fatalf("create: %v %q %q", err, ref, created)
	}
	if err := cf.DeleteTXT(ctx, ref); err != nil || deleted != "r1" {
		t.Fatalf("delete: %v", err)
	}
	if _, err := cf.ZoneFor(ctx, "_acme-challenge.other.org"); err == nil {
		t.Fatal("expected missing zone")
	}
	bad := NewCloudflare(srv.URL, "wrong", srv.Client())
	if err := bad.Verify(ctx); err == nil || !strings.Contains(err.Error(), "Invalid access token") {
		t.Fatalf("verify: %v", err)
	}
}
