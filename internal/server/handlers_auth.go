package server

import (
	"log/slog"
	"net/http"
	"strings"
)

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	req.Username = strings.TrimSpace(req.Username)
	if req.Username == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, "auth.credentials_required", "username and password are required")
		return
	}
	// Never reuse a session that existed before login (session fixation).
	if c, err := r.Cookie(cookieName); err == nil && c.Value != "" {
		if sess, _, err := s.Auth.Authenticate(r.Context(), c.Value, false); err == nil {
			if err := s.Store.DeleteSession(r.Context(), sess.IDHash); err != nil {
				slog.Error("login: delete previous session", "err", err)
			}
		}
	}

	token, sess, u, err := s.Auth.Login(r.Context(), truncate(req.Username, 64), req.Password, s.meta(r))
	if err != nil {
		handleErr(w, err)
		return
	}
	s.setCookie(w, token)
	next := "mfa_verify"
	if u.TOTPSecret == nil {
		next = "mfa_enroll"
	}
	writeJSON(w, http.StatusOK, map[string]string{"next": next, "csrf_token": sess.CSRFToken})
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request, p *principal) {
	cfg := s.Auth.Config()
	writeJSON(w, http.StatusOK, map[string]any{
		"username":             p.user.Username,
		"role":                 p.user.Role,
		"mfa_ok":               p.sess.MFAOK,
		"mfa_enrolled":         p.user.TOTPSecret != nil,
		"must_change_password": p.user.MustChangePassword,
		"csrf_token":           p.sess.CSRFToken,
		"idle_timeout_seconds": int(cfg.SessionIdleTimeout.Seconds()),
		"version":              s.Version,
	})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request, p *principal) {
	if err := s.Auth.Logout(r.Context(), p.sess, p.user, p.meta); err != nil {
		internalError(w, err)
		return
	}
	s.clearCookie(w)
	w.WriteHeader(http.StatusNoContent)
}

type codeRequest struct {
	Code string `json:"code"`
}

func (s *Server) handleMFAVerify(w http.ResponseWriter, r *http.Request, p *principal) {
	var req codeRequest
	if !readJSON(w, r, &req) {
		return
	}
	if err := s.Auth.VerifyMFA(r.Context(), p.sess, p.user, req.Code, p.meta); err != nil {
		handleErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleEnrollStart(w http.ResponseWriter, r *http.Request, p *principal) {
	secret, uri, err := s.Auth.BeginEnrollment(r.Context(), p.sess, p.user)
	if err != nil {
		handleErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"secret": secret, "uri": uri})
}

func (s *Server) handleEnrollFinish(w http.ResponseWriter, r *http.Request, p *principal) {
	var req codeRequest
	if !readJSON(w, r, &req) {
		return
	}
	if err := s.Auth.FinishEnrollment(r.Context(), p.sess, p.user, req.Code, p.meta); err != nil {
		handleErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleChangePassword(w http.ResponseWriter, r *http.Request, p *principal) {
	var req struct {
		Current string `json:"current_password"`
		New     string `json:"new_password"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	if err := s.Auth.ChangePassword(r.Context(), p.sess, p.user, req.Current, req.New, p.meta); err != nil {
		handleErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
