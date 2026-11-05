// Package auth implements local authentication: argon2id passwords, mandatory
// TOTP MFA, account lockout and server-side sessions.
package auth

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/carloseduardo21/apisix-dashboard/internal/audit"
	"github.com/carloseduardo21/apisix-dashboard/internal/secure"
	"github.com/carloseduardo21/apisix-dashboard/internal/store"
)

var (
	ErrInvalidCredentials = errors.New("usuário ou senha inválidos")
	ErrInvalidCode        = errors.New("código MFA inválido")
	ErrSessionExpired     = errors.New("sessão expirada")
	ErrMFAState           = errors.New("operação MFA não permitida neste estado")
)

type Config struct {
	Issuer             string
	SessionIdleTimeout time.Duration
	SessionMaxLifetime time.Duration
	PendingTimeout     time.Duration // time allowed to complete MFA after the password step
	MaxLoginAttempts   int
	LockoutDuration    time.Duration
}

// Meta describes the client performing a request, for sessions and audit.
type Meta struct {
	IP        string
	UserAgent string
}

type Service struct {
	store     *store.Store
	aead      *secure.AEAD
	audit     *audit.Logger
	cfg       Config
	dummyHash string
	now       func() time.Time
}

func NewService(st *store.Store, aead *secure.AEAD, al *audit.Logger, cfg Config) *Service {
	return &Service{
		store:     st,
		aead:      aead,
		audit:     al,
		cfg:       cfg,
		dummyHash: HashPassword(secure.RandomToken(16)),
		now:       time.Now,
	}
}

func (s *Service) Config() Config { return s.cfg }

func totpAD(userID int64) []byte       { return fmt.Appendf(nil, "totp:user:%d", userID) }
func pendingAD(sessHash string) []byte { return []byte("totp:pending:" + sessHash) }

func (s *Service) record(ctx context.Context, actor string, m Meta, action, outcome, detail string) {
	s.audit.Record(ctx, store.AuditEvent{Actor: actor, IP: m.IP, Action: action, Outcome: outcome, Detail: detail})
}

// Login checks the password and opens a session that still requires MFA.
// It returns the raw session token to be set as a cookie.
func (s *Service) Login(ctx context.Context, username, password string, m Meta) (string, *store.Session, *store.User, error) {
	u, err := s.store.GetUserByUsername(ctx, username)
	if errors.Is(err, store.ErrNotFound) {
		VerifyPassword(password, s.dummyHash) // keep timing uniform
		s.record(ctx, username, m, "auth.login", audit.Failure, "unknown user")
		return "", nil, nil, ErrInvalidCredentials
	}
	if err != nil {
		return "", nil, nil, err
	}
	now := s.now()
	switch {
	case u.Disabled:
		VerifyPassword(password, s.dummyHash)
		s.record(ctx, username, m, "auth.login", audit.Failure, "account disabled")
		return "", nil, nil, ErrInvalidCredentials
	case now.Before(u.LockedUntil):
		VerifyPassword(password, s.dummyHash)
		s.record(ctx, username, m, "auth.login", audit.Failure, "account locked")
		return "", nil, nil, ErrInvalidCredentials
	}
	if !VerifyPassword(password, u.PasswordHash) {
		if err := s.registerFailure(ctx, u, m, "auth.login", "invalid password"); err != nil {
			return "", nil, nil, err
		}
		return "", nil, nil, ErrInvalidCredentials
	}

	token := secure.RandomToken(32)
	sess := &store.Session{
		IDHash:    secure.HashToken(token),
		UserID:    u.ID,
		CSRFToken: secure.RandomToken(32),
		IP:        m.IP,
		UserAgent: m.UserAgent,
		CreatedAt: now,
		LastSeen:  now,
	}
	if err := s.store.CreateSession(ctx, sess); err != nil {
		return "", nil, nil, err
	}
	s.record(ctx, username, m, "auth.password", audit.Success, "awaiting MFA")
	return token, sess, u, nil
}

// registerFailure counts a failed authentication and locks the account after
// MaxLoginAttempts (PCI DSS 8.3.4).
func (s *Service) registerFailure(ctx context.Context, u *store.User, m Meta, action, reason string) error {
	now := s.now()
	u.FailedAttempts++
	u.UpdatedAt = now
	locked := u.FailedAttempts >= s.cfg.MaxLoginAttempts
	if locked {
		u.LockedUntil = now.Add(s.cfg.LockoutDuration)
		u.FailedAttempts = 0
	}
	if err := s.store.UpdateUser(ctx, u); err != nil {
		return err
	}
	s.record(ctx, u.Username, m, action, audit.Failure, reason)
	if locked {
		s.record(ctx, u.Username, m, "auth.lockout", audit.Success,
			fmt.Sprintf("locked for %s after %d failed attempts", s.cfg.LockoutDuration, s.cfg.MaxLoginAttempts))
		return s.store.DeleteUserSessions(ctx, u.ID, "")
	}
	return nil
}

// dropSession deletes an invalid session. A failure is only logged: the
// session is rejected anyway and the janitor purges it later.
func (s *Service) dropSession(ctx context.Context, idHash string) {
	if err := s.store.DeleteSession(ctx, idHash); err != nil {
		slog.Error("auth: delete invalid session", "err", err)
	}
}

// Authenticate resolves a session token, enforcing idle and absolute timeouts.
func (s *Service) Authenticate(ctx context.Context, token string) (*store.Session, *store.User, error) {
	sess, err := s.store.GetSession(ctx, secure.HashToken(token))
	if errors.Is(err, store.ErrNotFound) {
		return nil, nil, ErrSessionExpired
	}
	if err != nil {
		return nil, nil, err
	}
	now := s.now()
	idle := s.cfg.SessionIdleTimeout
	if !sess.MFAOK {
		idle = s.cfg.PendingTimeout
	}
	if now.Sub(sess.LastSeen) > idle || now.Sub(sess.CreatedAt) > s.cfg.SessionMaxLifetime {
		s.dropSession(ctx, sess.IDHash)
		return nil, nil, ErrSessionExpired
	}
	u, err := s.store.GetUserByID(ctx, sess.UserID)
	if errors.Is(err, store.ErrNotFound) || (err == nil && (u.Disabled || now.Before(u.LockedUntil))) {
		s.dropSession(ctx, sess.IDHash)
		return nil, nil, ErrSessionExpired
	}
	if err != nil {
		return nil, nil, err
	}
	sess.LastSeen = now
	if err := s.store.UpdateSession(ctx, sess); err != nil {
		return nil, nil, err
	}
	return sess, u, nil
}

func (s *Service) completeLogin(ctx context.Context, sess *store.Session, u *store.User, step int64, m Meta, action string) error {
	now := s.now()
	u.TOTPLastStep = step
	u.FailedAttempts = 0
	u.LastLoginAt = now
	u.UpdatedAt = now
	if err := s.store.UpdateUser(ctx, u); err != nil {
		return err
	}
	sess.MFAOK = true
	sess.PendingTOTP = nil
	sess.LastSeen = now
	if err := s.store.UpdateSession(ctx, sess); err != nil {
		return err
	}
	s.record(ctx, u.Username, m, action, audit.Success, "")
	return nil
}

// VerifyMFA completes the login of an enrolled user.
func (s *Service) VerifyMFA(ctx context.Context, sess *store.Session, u *store.User, code string, m Meta) error {
	if sess.MFAOK || u.TOTPSecret == nil {
		return ErrMFAState
	}
	secret, err := s.aead.Open(u.TOTPSecret, totpAD(u.ID))
	if err != nil {
		return fmt.Errorf("decrypt totp secret: %w", err)
	}
	step, ok := verifyTOTP(secret, code, s.now(), u.TOTPLastStep)
	if !ok {
		if err := s.registerFailure(ctx, u, m, "auth.mfa", "invalid TOTP code"); err != nil {
			return err
		}
		return ErrInvalidCode
	}
	return s.completeLogin(ctx, sess, u, step, m, "auth.login")
}

// BeginEnrollment generates a TOTP secret for a user without MFA. The secret
// is kept (encrypted) in the session until the user proves they stored it.
func (s *Service) BeginEnrollment(ctx context.Context, sess *store.Session, u *store.User) (secret, uri string, err error) {
	if sess.MFAOK || u.TOTPSecret != nil {
		return "", "", ErrMFAState
	}
	raw := newTOTPSecret()
	sess.PendingTOTP = s.aead.Seal(raw, pendingAD(sess.IDHash))
	if err := s.store.UpdateSession(ctx, sess); err != nil {
		return "", "", err
	}
	return b32.EncodeToString(raw), totpURI(s.cfg.Issuer, u.Username, raw), nil
}

func (s *Service) FinishEnrollment(ctx context.Context, sess *store.Session, u *store.User, code string, m Meta) error {
	if sess.MFAOK || u.TOTPSecret != nil || sess.PendingTOTP == nil {
		return ErrMFAState
	}
	raw, err := s.aead.Open(sess.PendingTOTP, pendingAD(sess.IDHash))
	if err != nil {
		return fmt.Errorf("decrypt pending totp secret: %w", err)
	}
	step, ok := verifyTOTP(raw, code, s.now(), 0)
	if !ok {
		if err := s.registerFailure(ctx, u, m, "auth.mfa_enroll", "invalid TOTP code"); err != nil {
			return err
		}
		return ErrInvalidCode
	}
	u.TOTPSecret = s.aead.Seal(raw, totpAD(u.ID))
	s.record(ctx, u.Username, m, "auth.mfa_enroll", audit.Success, "")
	return s.completeLogin(ctx, sess, u, step, m, "auth.login")
}

func (s *Service) Logout(ctx context.Context, sess *store.Session, u *store.User, m Meta) error {
	s.record(ctx, u.Username, m, "auth.logout", audit.Success, "")
	return s.store.DeleteSession(ctx, sess.IDHash)
}

// ChangePassword is the self-service change. Other sessions are revoked.
func (s *Service) ChangePassword(ctx context.Context, sess *store.Session, u *store.User, current, next string, m Meta) error {
	if !VerifyPassword(current, u.PasswordHash) {
		if err := s.registerFailure(ctx, u, m, "auth.password_change", "invalid current password"); err != nil {
			return err
		}
		return validationErr("senha atual incorreta")
	}
	if err := s.SetPassword(ctx, u, next, false); err != nil {
		return err
	}
	s.record(ctx, u.Username, m, "auth.password_change", audit.Success, "")
	return s.store.DeleteUserSessions(ctx, u.ID, sess.IDHash)
}

// SetPassword validates and stores a new password, rejecting the last
// PasswordHistory passwords (PCI DSS 8.3.7).
func (s *Service) SetPassword(ctx context.Context, u *store.User, pw string, mustChange bool) error {
	if err := ValidatePassword(u.Username, pw); err != nil {
		return err
	}
	if u.ID != 0 {
		recent, err := s.store.RecentPasswordHashes(ctx, u.ID, PasswordHistory)
		if err != nil {
			return err
		}
		for _, h := range recent {
			if VerifyPassword(pw, h) {
				return validationErr("a nova senha não pode ser igual a uma das últimas %d", PasswordHistory)
			}
		}
	}
	now := s.now()
	u.PasswordHash = HashPassword(pw)
	u.MustChangePassword = mustChange
	u.UpdatedAt = now
	if u.ID == 0 {
		return nil // caller creates the user and records history
	}
	if err := s.store.UpdateUser(ctx, u); err != nil {
		return err
	}
	return s.store.AddPasswordHistory(ctx, u.ID, u.PasswordHash, now)
}

// CreateUser validates and persists a new user.
func (s *Service) CreateUser(ctx context.Context, username, role, password string, mustChange bool) (*store.User, error) {
	if !validUsername(username) {
		return nil, validationErr("nome de usuário inválido: use 3 a 64 caracteres entre letras, números, '.', '_' ou '-'")
	}
	if !ValidRole(role) {
		return nil, validationErr("perfil inválido: %q", role)
	}
	now := s.now()
	u := &store.User{Username: username, Role: role, CreatedAt: now}
	if err := s.SetPassword(ctx, u, password, mustChange); err != nil {
		return nil, err
	}
	if err := s.store.CreateUser(ctx, u); err != nil {
		if errors.Is(err, store.ErrConflict) {
			return nil, validationErr("o usuário %q já existe", username)
		}
		return nil, err
	}
	if err := s.store.AddPasswordHistory(ctx, u.ID, u.PasswordHash, now); err != nil {
		return nil, err
	}
	return u, nil
}

func validUsername(s string) bool {
	if len(s) < 3 || len(s) > 64 {
		return false
	}
	for _, r := range s {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '.' || r == '_' || r == '-') {
			return false
		}
	}
	return true
}

// PurgeExpiredSessions removes sessions past their timeouts.
func (s *Service) PurgeExpiredSessions(ctx context.Context) error {
	now := s.now()
	return s.store.PurgeSessions(ctx, now.Add(-s.cfg.SessionIdleTimeout), now.Add(-s.cfg.PendingTimeout), now.Add(-s.cfg.SessionMaxLifetime))
}
