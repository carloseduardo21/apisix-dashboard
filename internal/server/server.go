// Package server wires the HTTP API and the embedded frontend.
package server

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"time"

	"github.com/carloseduardo21/apisix-dashboard/internal/acme"
	"github.com/carloseduardo21/apisix-dashboard/internal/apisix"
	"github.com/carloseduardo21/apisix-dashboard/internal/audit"
	"github.com/carloseduardo21/apisix-dashboard/internal/auth"
	"github.com/carloseduardo21/apisix-dashboard/internal/i18n"
	"github.com/carloseduardo21/apisix-dashboard/internal/kong"
	"github.com/carloseduardo21/apisix-dashboard/internal/store"
)

const (
	cookieName = "dash_session"
	csrfHeader = "X-CSRF-Token"
	// backgroundHeader marks requests the UI sends without user activity
	// (polling); they do not extend the idle timeout.
	backgroundHeader = "X-Background"
	maxBodySize      = 1 << 20
)

type Options struct {
	Store        *store.Store
	Auth         *auth.Service
	APISIX       *apisix.Client
	Catalog      *apisix.Catalog
	ACME         *acme.Manager  // nil when automatic issuance is disabled
	Kong         *kong.Migrator // nil when the Kong migration is disabled
	Audit        *audit.Logger
	Static       fs.FS
	CookieSecure bool
	// TrustProxy is the legacy DASH_TRUST_PROXY_HEADERS: without
	// TrustedProxies, only the direct peer is trusted as a proxy.
	TrustProxy     bool
	TrustedProxies []netip.Prefix
	// LoginRatePerMinute limits authentication attempts per client IP.
	LoginRatePerMinute int
	TLS                bool
	Version            string
}

type Server struct {
	Options
	limiter *limiter
}

func New(o Options) *Server { return &Server{Options: o, limiter: newLimiter(o.LoginRatePerMinute)} }

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	// Authentication. "pending" endpoints are reachable before MFA.
	// Endpoints that check a password or an MFA code are rate limited per IP.
	mux.HandleFunc("POST /api/auth/login", s.throttle(s.handleLogin))
	mux.HandleFunc("GET /api/auth/me", s.require(access{pending: true, mustChange: true}, s.handleMe))
	mux.HandleFunc("POST /api/auth/logout", s.require(access{pending: true, mustChange: true}, s.handleLogout))
	mux.HandleFunc("POST /api/auth/mfa/verify", s.throttle(s.require(access{pending: true}, s.handleMFAVerify)))
	mux.HandleFunc("POST /api/auth/mfa/enroll/start", s.require(access{pending: true}, s.handleEnrollStart))
	mux.HandleFunc("POST /api/auth/mfa/enroll/finish", s.throttle(s.require(access{pending: true}, s.handleEnrollFinish)))
	mux.HandleFunc("POST /api/auth/password", s.throttle(s.require(access{mustChange: true}, s.handleChangePassword)))

	// Dashboard users and audit trail.
	mux.HandleFunc("GET /api/users", s.require(access{admin: true}, s.handleListUsers))
	mux.HandleFunc("POST /api/users", s.require(access{admin: true}, s.handleCreateUser))
	mux.HandleFunc("PATCH /api/users/{id}", s.require(access{admin: true}, s.handleUpdateUser))
	mux.HandleFunc("DELETE /api/users/{id}", s.require(access{admin: true}, s.handleDeleteUser))
	mux.HandleFunc("POST /api/users/{id}/password", s.require(access{admin: true}, s.handleResetPassword))
	mux.HandleFunc("POST /api/users/{id}/reset-mfa", s.require(access{admin: true}, s.handleResetMFA))
	mux.HandleFunc("POST /api/users/{id}/unlock", s.require(access{admin: true}, s.handleUnlock))
	mux.HandleFunc("GET /api/audit", s.require(access{admin: true}, s.handleListAudit))

	// Automatic certificates (ACME DNS-01).
	mux.HandleFunc("GET /api/acme", s.require(access{}, s.handleACME))
	mux.HandleFunc("GET /api/acme/provider", s.require(access{admin: true}, s.handleACMEProvider))
	mux.HandleFunc("POST /api/acme/certificates", s.require(access{admin: true}, s.handleCreateCertificate))
	mux.HandleFunc("POST /api/acme/certificates/{id}/issue", s.require(access{admin: true}, s.handleIssueCertificate))
	mux.HandleFunc("DELETE /api/acme/certificates/{id}", s.require(access{admin: true}, s.handleDeleteCertificate))

	// Migration from Kong. Every endpoint requires a role that can write.
	mux.HandleFunc("GET /api/migration/kong", s.require(access{write: true}, s.handleKong))
	mux.HandleFunc("PUT /api/migration/kong/settings", s.require(access{write: true}, s.handleKongSettings))
	mux.HandleFunc("GET /api/migration/kong/inventory", s.require(access{write: true}, s.handleKongInventory))
	mux.HandleFunc("POST /api/migration/kong/plan", s.require(access{write: true}, s.handleKongPlan))
	mux.HandleFunc("POST /api/migration/kong/apply", s.require(access{write: true}, s.handleKongApply))
	mux.HandleFunc("GET /api/migration/kong/job", s.require(access{write: true}, s.handleKongJob))
	mux.HandleFunc("GET /api/migration/kong/history", s.require(access{write: true}, s.handleKongHistory))

	// APISIX Admin API.
	mux.HandleFunc("GET /api/status", s.require(access{}, s.handleStatus))
	mux.HandleFunc("GET /api/apisix/plugins/{name}", s.require(access{}, s.handlePlugins))
	mux.HandleFunc("GET /api/apisix/schemas", s.require(access{}, s.handleSchemas))
	mux.HandleFunc("/api/apisix/{path...}", s.require(access{}, s.handleAPISIX))

	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		notFound(w)
	})
	mux.Handle("/", spaHandler(s.Static))

	cop := http.NewCrossOriginProtection()
	return s.securityHeaders(cop.Handler(mux))
}

func (s *Server) securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; "+
			"img-src 'self' data:; font-src 'self' data:; connect-src 'self'; object-src 'none'; "+
			"frame-ancestors 'none'; base-uri 'none'; form-action 'self'")
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Cross-Origin-Opener-Policy", "same-origin")
		h.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		if s.TLS || s.CookieSecure {
			h.Set("Strict-Transport-Security", "max-age=31536000")
		}
		if strings.HasPrefix(r.URL.Path, "/api/") {
			h.Set("Cache-Control", "no-store")
		}
		next.ServeHTTP(w, r)
	})
}

// ---- authentication middleware ----

type access struct {
	pending    bool // allowed before MFA is completed
	mustChange bool // allowed while a password change is required
	admin      bool
	write      bool // operator or admin
}

type principal struct {
	sess *store.Session
	user *store.User
	meta auth.Meta
}

type handler func(w http.ResponseWriter, r *http.Request, p *principal)

func (s *Server) require(a access, h handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie(cookieName)
		if err != nil || c.Value == "" {
			writeError(w, http.StatusUnauthorized, "auth.not_authenticated", "not authenticated")
			return
		}
		sess, user, err := s.Auth.Authenticate(r.Context(), c.Value, r.Header.Get(backgroundHeader) != "1")
		if errors.Is(err, auth.ErrSessionExpired) {
			s.clearCookie(w)
			writeUserError(w, http.StatusUnauthorized, err)
			return
		}
		if err != nil {
			internalError(w, err)
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			if subtle.ConstantTimeCompare([]byte(r.Header.Get(csrfHeader)), []byte(sess.CSRFToken)) != 1 {
				writeError(w, http.StatusForbidden, "auth.csrf_invalid", "invalid CSRF token")
				return
			}
		}
		if !sess.MFAOK && !a.pending {
			writeError(w, http.StatusUnauthorized, "auth.mfa_pending", "MFA verification pending")
			return
		}
		if sess.MFAOK && user.MustChangePassword && !a.mustChange {
			writeError(w, http.StatusForbidden, "auth.password_change_required", "password change required")
			return
		}
		if a.admin && !auth.IsAdmin(user.Role) {
			writeError(w, http.StatusForbidden, "auth.admin_only", "restricted to administrators")
			return
		}
		if a.write && !auth.CanWrite(user.Role) {
			writeError(w, http.StatusForbidden, "auth.read_only_role", "your role does not allow changes")
			return
		}
		h(w, r, &principal{sess: sess, user: user, meta: s.meta(r)})
	}
}

func (s *Server) meta(r *http.Request) auth.Meta {
	return auth.Meta{IP: s.clientIP(r), UserAgent: truncate(r.UserAgent(), 256)}
}

// clientIP returns the address recorded in sessions and the audit trail and
// used for rate limiting. Forwarded headers are only honored when the direct
// peer is a trusted proxy; the client is then the rightmost X-Forwarded-For
// entry that is not a trusted proxy (entries further left are set by the
// client and cannot be trusted).
func (s *Server) clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	peer, err := netip.ParseAddr(host)
	if err != nil {
		return host
	}
	peer = peer.Unmap()
	if !s.trustedProxy(peer, peer) {
		return peer.String()
	}
	var hops []string
	for _, v := range r.Header.Values("X-Forwarded-For") {
		hops = append(hops, strings.Split(v, ",")...)
	}
	if len(hops) == 0 {
		if a, err := netip.ParseAddr(strings.TrimSpace(r.Header.Get("X-Real-IP"))); err == nil {
			return a.Unmap().String()
		}
		return peer.String()
	}
	client := peer
	for i := len(hops) - 1; i >= 0; i-- {
		a, err := netip.ParseAddr(strings.TrimSpace(hops[i]))
		if err != nil {
			break // malformed entry: stop at the last address we could trust
		}
		client = a.Unmap()
		if !s.trustedProxy(client, peer) {
			break
		}
	}
	return client.String()
}

// trustedProxy reports whether a may set forwarded headers. With the legacy
// DASH_TRUST_PROXY_HEADERS and no CIDR list, only the direct peer is trusted.
func (s *Server) trustedProxy(a, peer netip.Addr) bool {
	if len(s.TrustedProxies) > 0 {
		for _, p := range s.TrustedProxies {
			if p.Contains(a) {
				return true
			}
		}
		return false
	}
	return s.TrustProxy && a == peer
}

func (s *Server) setCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{ // #nosec G124 -- HttpOnly+SameSite=Strict; Secure is configurable (DASH_COOKIE_SECURE) and defaults to true
		Name:     cookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   s.CookieSecure,
		SameSite: http.SameSiteStrictMode,
	})
}

func (s *Server) clearCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{ // #nosec G124 -- same attributes as setCookie
		Name:     cookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		Expires:  time.Unix(0, 0),
		HttpOnly: true,
		Secure:   s.CookieSecure,
		SameSite: http.SameSiteStrictMode,
	})
}

// ---- helpers ----

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v) // headers already sent; client may have gone away
}

// errorBody is the JSON shape of every error response. "error" is English;
// the UI translates "code" with "params" (see internal/i18n).
type errorBody struct {
	Error   string            `json:"error"`
	Code    string            `json:"code"`
	Params  map[string]string `json:"params,omitempty"`
	Details []i18n.Message    `json:"details,omitempty"`
}

func writeMessages(w http.ResponseWriter, status int, msgs ...i18n.Message) {
	body := errorBody{Error: msgs[0].String(), Code: msgs[0].Code, Params: msgs[0].Params}
	if len(msgs) > 1 {
		texts := make([]string, len(msgs))
		for i, m := range msgs {
			texts[i] = m.String()
		}
		multi := i18n.M("api.multiple_errors", "multiple problems were found")
		body = errorBody{Error: strings.Join(texts, "; "), Code: multi.Code, Details: msgs}
	}
	writeJSON(w, status, body)
}

// writeError writes a user-facing error (see i18n.M for the arguments).
func writeError(w http.ResponseWriter, status int, code, template string, kv ...any) {
	writeMessages(w, status, i18n.M(code, template, kv...))
}

// writeUserError writes the user-facing message(s) carried by err and reports
// whether it did; errors without one must be handled by the caller.
func writeUserError(w http.ResponseWriter, status int, err error) bool {
	msgs, ok := i18n.Messages(err)
	if ok {
		writeMessages(w, status, msgs...)
	}
	return ok
}

func notFound(w http.ResponseWriter) {
	writeError(w, http.StatusNotFound, "api.not_found", "not found")
}

func internalError(w http.ResponseWriter, err error) {
	slog.Error("request failed", "err", err)
	writeError(w, http.StatusInternalServerError, "api.internal_error", "internal error")
}

// handleErr maps errors from the auth package to responses.
func handleErr(w http.ResponseWriter, err error) {
	var ve *auth.ValidationError
	switch {
	case errors.As(err, &ve):
		writeUserError(w, http.StatusBadRequest, err)
	case errors.Is(err, auth.ErrInvalidCredentials), errors.Is(err, auth.ErrInvalidCode):
		writeUserError(w, http.StatusUnauthorized, err)
	case errors.Is(err, auth.ErrMFAState):
		writeUserError(w, http.StatusConflict, err)
	case errors.Is(err, auth.ErrBusy):
		w.Header().Set("Retry-After", "1")
		writeUserError(w, http.StatusTooManyRequests, err)
	case errors.Is(err, store.ErrNotFound):
		notFound(w)
	default:
		internalError(w, err)
	}
}

func readJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	b, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBodySize))
	if err != nil {
		writeError(w, http.StatusRequestEntityTooLarge, "api.body_too_large", "request body too large")
		return false
	}
	if err := json.Unmarshal(b, v); err != nil {
		writeError(w, http.StatusBadRequest, "api.invalid_json", "invalid JSON")
		return false
	}
	return true
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

// StartSessionJanitor periodically deletes expired sessions.
func (s *Server) StartSessionJanitor(ctx context.Context) {
	go func() {
		t := time.NewTicker(time.Minute)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				if err := s.Auth.PurgeExpiredSessions(ctx); err != nil {
					slog.Error("purge sessions", "err", err)
				}
			}
		}
	}()
}
