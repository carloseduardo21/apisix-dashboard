package server

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/carloseduardo21/apisix-dashboard/internal/audit"
	"github.com/carloseduardo21/apisix-dashboard/internal/auth"
	"github.com/carloseduardo21/apisix-dashboard/internal/store"
)

type userView struct {
	ID                 int64      `json:"id"`
	Username           string     `json:"username"`
	Role               string     `json:"role"`
	Disabled           bool       `json:"disabled"`
	Locked             bool       `json:"locked"`
	MFAEnrolled        bool       `json:"mfa_enrolled"`
	MustChangePassword bool       `json:"must_change_password"`
	CreatedAt          time.Time  `json:"created_at"`
	LastLoginAt        *time.Time `json:"last_login_at"`
}

func viewUser(u *store.User) userView {
	v := userView{
		ID:                 u.ID,
		Username:           u.Username,
		Role:               u.Role,
		Disabled:           u.Disabled,
		Locked:             time.Now().Before(u.LockedUntil),
		MFAEnrolled:        u.TOTPSecret != nil,
		MustChangePassword: u.MustChangePassword,
		CreatedAt:          u.CreatedAt,
	}
	if !u.LastLoginAt.IsZero() {
		v.LastLoginAt = &u.LastLoginAt
	}
	return v
}

func (s *Server) recordUser(ctx context.Context, p *principal, action, target, outcome, detail string) {
	s.Audit.Record(ctx, store.AuditEvent{
		Actor: p.user.Username, IP: p.meta.IP, Action: action,
		Resource: "dashboard_users", ResourceID: target, Outcome: outcome, Detail: detail,
	})
}

func (s *Server) handleListUsers(w http.ResponseWriter, r *http.Request, p *principal) {
	users, err := s.Store.ListUsers(r.Context())
	if err != nil {
		internalError(w, err)
		return
	}
	out := make([]userView, 0, len(users))
	for _, u := range users {
		out = append(out, viewUser(u))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleCreateUser(w http.ResponseWriter, r *http.Request, p *principal) {
	var req struct {
		Username string `json:"username"`
		Role     string `json:"role"`
		Password string `json:"password"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	// Passwords set by an admin are temporary (PCI DSS 8.3.5).
	u, err := s.Auth.CreateUser(r.Context(), req.Username, req.Role, req.Password, true)
	if err != nil {
		s.recordUser(r.Context(), p, "user.create", req.Username, audit.Failure, err.Error())
		handleErr(w, err)
		return
	}
	s.recordUser(r.Context(), p, "user.create", u.Username, audit.Success, "role="+u.Role)
	writeJSON(w, http.StatusCreated, viewUser(u))
}

// targetUser loads the {id} user of the request.
func (s *Server) targetUser(w http.ResponseWriter, r *http.Request) (*store.User, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		notFound(w)
		return nil, false
	}
	u, err := s.Store.GetUserByID(r.Context(), id)
	if err != nil {
		handleErr(w, err)
		return nil, false
	}
	return u, true
}

// keepsAnAdmin reports whether demoting/disabling/deleting u still leaves an
// enabled admin.
func (s *Server) keepsAnAdmin(ctx context.Context, u *store.User) (bool, error) {
	if u.Role != auth.RoleAdmin || u.Disabled {
		return true, nil
	}
	n, err := s.Store.CountActiveAdmins(ctx, auth.RoleAdmin)
	return n > 1, err
}

func (s *Server) handleUpdateUser(w http.ResponseWriter, r *http.Request, p *principal) {
	u, ok := s.targetUser(w, r)
	if !ok {
		return
	}
	var req struct {
		Role     *string `json:"role"`
		Disabled *bool   `json:"disabled"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	if u.ID == p.user.ID {
		writeError(w, http.StatusBadRequest, "users.cannot_change_self", "you cannot change your own role or status")
		return
	}
	demoting := (req.Role != nil && *req.Role != auth.RoleAdmin) || (req.Disabled != nil && *req.Disabled)
	if demoting {
		ok, err := s.keepsAnAdmin(r.Context(), u)
		if err != nil {
			internalError(w, err)
			return
		}
		if !ok {
			writeError(w, http.StatusBadRequest, "users.last_admin", "at least one active administrator is required")
			return
		}
	}

	detail := ""
	if req.Role != nil {
		if !auth.ValidRole(*req.Role) {
			writeError(w, http.StatusBadRequest, "users.invalid_role", "invalid role")
			return
		}
		detail += "role " + u.Role + "->" + *req.Role + " "
		u.Role = *req.Role
	}
	if req.Disabled != nil {
		detail += "disabled=" + strconv.FormatBool(*req.Disabled)
		u.Disabled = *req.Disabled
	}
	u.UpdatedAt = time.Now()
	if err := s.Store.UpdateUser(r.Context(), u); err != nil {
		internalError(w, err)
		return
	}
	// Privilege changes take effect immediately.
	if err := s.Store.DeleteUserSessions(r.Context(), u.ID, ""); err != nil {
		internalError(w, err)
		return
	}
	s.recordUser(r.Context(), p, "user.update", u.Username, audit.Success, detail)
	writeJSON(w, http.StatusOK, viewUser(u))
}

func (s *Server) handleDeleteUser(w http.ResponseWriter, r *http.Request, p *principal) {
	u, ok := s.targetUser(w, r)
	if !ok {
		return
	}
	if u.ID == p.user.ID {
		writeError(w, http.StatusBadRequest, "users.cannot_delete_self", "you cannot delete your own user")
		return
	}
	if ok, err := s.keepsAnAdmin(r.Context(), u); err != nil {
		internalError(w, err)
		return
	} else if !ok {
		writeError(w, http.StatusBadRequest, "users.last_admin", "at least one active administrator is required")
		return
	}
	if err := s.Store.DeleteUser(r.Context(), u.ID); err != nil {
		internalError(w, err)
		return
	}
	s.recordUser(r.Context(), p, "user.delete", u.Username, audit.Success, "")
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleResetPassword(w http.ResponseWriter, r *http.Request, p *principal) {
	u, ok := s.targetUser(w, r)
	if !ok {
		return
	}
	var req struct {
		Password string `json:"password"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	if err := s.Auth.SetPassword(r.Context(), u, req.Password, true); err != nil {
		handleErr(w, err)
		return
	}
	u.FailedAttempts = 0
	u.LockedUntil = time.Time{}
	if err := s.Store.UpdateUser(r.Context(), u); err != nil {
		internalError(w, err)
		return
	}
	if err := s.Store.DeleteUserSessions(r.Context(), u.ID, ""); err != nil {
		internalError(w, err)
		return
	}
	s.recordUser(r.Context(), p, "user.password_reset", u.Username, audit.Success, "")
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleResetMFA(w http.ResponseWriter, r *http.Request, p *principal) {
	u, ok := s.targetUser(w, r)
	if !ok {
		return
	}
	u.TOTPSecret = nil
	u.TOTPLastStep = 0
	u.UpdatedAt = time.Now()
	if err := s.Store.UpdateUser(r.Context(), u); err != nil {
		internalError(w, err)
		return
	}
	if err := s.Store.DeleteUserSessions(r.Context(), u.ID, ""); err != nil {
		internalError(w, err)
		return
	}
	s.recordUser(r.Context(), p, "user.mfa_reset", u.Username, audit.Success, "")
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleUnlock(w http.ResponseWriter, r *http.Request, p *principal) {
	u, ok := s.targetUser(w, r)
	if !ok {
		return
	}
	u.FailedAttempts = 0
	u.LockedUntil = time.Time{}
	u.UpdatedAt = time.Now()
	if err := s.Store.UpdateUser(r.Context(), u); err != nil {
		internalError(w, err)
		return
	}
	s.recordUser(r.Context(), p, "user.unlock", u.Username, audit.Success, "")
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleListAudit(w http.ResponseWriter, r *http.Request, p *principal) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	before, _ := strconv.ParseInt(r.URL.Query().Get("before"), 10, 64)
	events, err := s.Store.ListAudit(r.Context(), limit, before)
	if err != nil {
		internalError(w, err)
		return
	}
	resp := map[string]any{"items": events}
	if len(events) == limit {
		resp["next_before"] = events[len(events)-1].ID
	}
	writeJSON(w, http.StatusOK, resp)
}
