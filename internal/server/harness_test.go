package server

import (
	"bytes"
	"context"
	"crypto/rand"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/carloseduardo21/apisix-dashboard/internal/apisix"
	"github.com/carloseduardo21/apisix-dashboard/internal/audit"
	"github.com/carloseduardo21/apisix-dashboard/internal/auth"
	"github.com/carloseduardo21/apisix-dashboard/internal/secure"
	"github.com/carloseduardo21/apisix-dashboard/internal/store"
)

const testPassword = "correct horse 42 battery"

// fakeAdmin is a stand-in for the APISIX Admin API that records the raw
// request URIs it receives and answers with a fixed body.
type fakeAdmin struct {
	mu   sync.Mutex
	uris []string
	body string
}

func (f *fakeAdmin) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	f.uris = append(f.uris, r.RequestURI)
	body := f.body
	f.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	_, _ = io.WriteString(w, body)
}

func (f *fakeAdmin) requests() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.uris...)
}

type harness struct {
	t      *testing.T
	srv    *Server
	h      http.Handler
	store  *store.Store
	auth   *auth.Service
	admin  *fakeAdmin
	audit  *bytes.Buffer
	remote string // RemoteAddr of test requests
}

type harnessOpt func(*Options, *auth.Config)

func newHarness(t *testing.T, opts ...harnessOpt) *harness {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	key := make([]byte, secure.KeySize)
	_, _ = rand.Read(key)
	aead, err := secure.NewAEAD(key)
	if err != nil {
		t.Fatal(err)
	}
	var auditBuf bytes.Buffer
	al := audit.New(st, &syncWriter{w: &auditBuf})

	fa := &fakeAdmin{body: `{}`}
	upstream := httptest.NewServer(fa)
	t.Cleanup(upstream.Close)
	client, err := apisix.NewClient(upstream.URL, "test-admin-key", "", 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}

	cfg := auth.Config{
		Issuer:             "Lintel",
		SessionIdleTimeout: 15 * time.Minute,
		SessionMaxLifetime: 12 * time.Hour,
		PendingTimeout:     5 * time.Minute,
		MaxLoginAttempts:   6,
		LockoutDuration:    30 * time.Minute,
	}
	o := Options{Store: st, APISIX: client, Audit: al, CookieSecure: false}
	for _, opt := range opts {
		opt(&o, &cfg)
	}
	svc := auth.NewService(st, aead, al, cfg)
	o.Auth = svc
	srv := New(o)
	return &harness{t: t, srv: srv, h: srv.Handler(), store: st, auth: svc, admin: fa, audit: &auditBuf, remote: "192.0.2.10:40000"}
}

// syncWriter serializes writes to the audit buffer (handlers run concurrently
// in some tests).
type syncWriter struct {
	mu sync.Mutex
	w  io.Writer
}

func (s *syncWriter) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.w.Write(p)
}

// session is an authenticated browser session (MFA completed).
type session struct {
	cookie string
	csrf   string
}

// login creates a user with role and returns a session that has passed MFA.
func (h *harness) login(username, role string) session {
	h.t.Helper()
	ctx := context.Background()
	if _, err := h.auth.CreateUser(ctx, username, role, testPassword, false); err != nil {
		h.t.Fatal(err)
	}
	token, sess, _, err := h.auth.Login(ctx, username, testPassword, auth.Meta{IP: "192.0.2.1"})
	if err != nil {
		h.t.Fatal(err)
	}
	sess.MFAOK = true
	if err := h.store.UpdateSession(ctx, sess); err != nil {
		h.t.Fatal(err)
	}
	return session{cookie: token, csrf: sess.CSRFToken}
}

// do sends a request. rawPath is used verbatim (it may contain
// percent-encoded characters the client must not normalize).
func (h *harness) do(s *session, method, rawPath, body string, hdr ...string) *httptest.ResponseRecorder {
	h.t.Helper()
	var rd io.Reader
	if body != "" {
		rd = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, "http://dashboard.example.com"+rawPath, rd)
	req.RemoteAddr = h.remote
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	if s != nil {
		req.AddCookie(&http.Cookie{Name: cookieName, Value: s.cookie})
		req.Header.Set(csrfHeader, s.csrf)
	}
	for i := 0; i+1 < len(hdr); i += 2 {
		req.Header.Set(hdr[i], hdr[i+1])
	}
	rec := httptest.NewRecorder()
	h.h.ServeHTTP(rec, req)
	return rec
}
