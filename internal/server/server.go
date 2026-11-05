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
	"strings"
	"time"

	"github.com/carloseduardo21/apisix-dashboard/internal/acme"
	"github.com/carloseduardo21/apisix-dashboard/internal/apisix"
	"github.com/carloseduardo21/apisix-dashboard/internal/audit"
	"github.com/carloseduardo21/apisix-dashboard/internal/auth"
	"github.com/carloseduardo21/apisix-dashboard/internal/kong"
	"github.com/carloseduardo21/apisix-dashboard/internal/store"
)

const (
	cookieName  = "dash_session"
	csrfHeader  = "X-CSRF-Token"
	maxBodySize = 1 << 20
)

type Options struct {
	Store        *store.Store
	Auth         *auth.Service
	APISIX       *apisix.Client
	Catalog      *apisix.Catalog
	ACME         *acme.Manager // nil when automatic issuance is disabled
	Kong         *kong.Migrator // nil when the Kong migration is disabled
	Audit        *audit.Logger
	Static       fs.FS
	CookieSecure bool
	TrustProxy   bool
	TLS          bool
	Version      string
}

type Server struct {
	Options
}

func New(o Options) *Server { return &Server{Options: o} }

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	// Authentication. "pending" endpoints are reachable before MFA.
	mux.HandleFunc("POST /api/auth/login", s.handleLogin)
	mux.HandleFunc("GET /api/auth/me", s.require(access{pending: true, mustChange: true}, s.handleMe))
	mux.HandleFunc("POST /api/auth/logout", s.require(access{pending: true, mustChange: true}, s.handleLogout))
	mux.HandleFunc("POST /api/auth/mfa/verify", s.require(access{pending: true}, s.handleMFAVerify))
	mux.HandleFunc("POST /api/auth/mfa/enroll/start", s.require(access{pending: true}, s.handleEnrollStart))
	mux.HandleFunc("POST /api/auth/mfa/enroll/finish", s.require(access{pending: true}, s.handleEnrollFinish))
	mux.HandleFunc("POST /api/auth/password", s.require(access{mustChange: true}, s.handleChangePassword))

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
		writeError(w, http.StatusNotFound, "not found")
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
			writeError(w, http.StatusUnauthorized, "não autenticado")
			return
		}
		sess, user, err := s.Auth.Authenticate(r.Context(), c.Value)
		if errors.Is(err, auth.ErrSessionExpired) {
			s.clearCookie(w)
			writeError(w, http.StatusUnauthorized, err.Error())
			return
		}
		if err != nil {
			internalError(w, err)
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			if subtle.ConstantTimeCompare([]byte(r.Header.Get(csrfHeader)), []byte(sess.CSRFToken)) != 1 {
				writeError(w, http.StatusForbidden, "token CSRF inválido")
				return
			}
		}
		if !sess.MFAOK && !a.pending {
			writeError(w, http.StatusUnauthorized, "MFA pendente")
			return
		}
		if sess.MFAOK && user.MustChangePassword && !a.mustChange {
			writeError(w, http.StatusForbidden, "troca de senha obrigatória")
			return
		}
		if a.admin && !auth.IsAdmin(user.Role) {
			writeError(w, http.StatusForbidden, "acesso restrito a administradores")
			return
		}
		if a.write && !auth.CanWrite(user.Role) {
			writeError(w, http.StatusForbidden, "seu perfil não permite alterações")
			return
		}
		h(w, r, &principal{sess: sess, user: user, meta: s.meta(r)})
	}
}

func (s *Server) meta(r *http.Request) auth.Meta {
	return auth.Meta{IP: s.clientIP(r), UserAgent: truncate(r.UserAgent(), 256)}
}

func (s *Server) clientIP(r *http.Request) string {
	if s.TrustProxy {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			return strings.TrimSpace(strings.Split(xff, ",")[0])
		}
		if xr := r.Header.Get("X-Real-IP"); xr != "" {
			return strings.TrimSpace(xr)
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func (s *Server) setCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   s.CookieSecure,
		SameSite: http.SameSiteStrictMode,
	})
}

func (s *Server) clearCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
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
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func internalError(w http.ResponseWriter, err error) {
	slog.Error("request failed", "err", err)
	writeError(w, http.StatusInternalServerError, "erro interno")
}

// handleErr maps errors from the auth package to responses.
func handleErr(w http.ResponseWriter, err error) {
	var ve *auth.ValidationError
	switch {
	case errors.As(err, &ve):
		writeError(w, http.StatusBadRequest, ve.Error())
	case errors.Is(err, auth.ErrInvalidCredentials), errors.Is(err, auth.ErrInvalidCode):
		writeError(w, http.StatusUnauthorized, err.Error())
	case errors.Is(err, auth.ErrMFAState):
		writeError(w, http.StatusConflict, err.Error())
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, "não encontrado")
	default:
		internalError(w, err)
	}
}

func readJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	b, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBodySize))
	if err != nil {
		writeError(w, http.StatusRequestEntityTooLarge, "corpo da requisição muito grande")
		return false
	}
	if err := json.Unmarshal(b, v); err != nil {
		writeError(w, http.StatusBadRequest, "JSON inválido")
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
