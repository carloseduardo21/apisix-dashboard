// Package store persists dashboard state (users, sessions, audit trail) in an
// embedded SQLite database. APISIX configuration itself is never stored here.
package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	_ "modernc.org/sqlite" // SQLite driver (pure Go, registers "sqlite")
)

var (
	ErrNotFound = errors.New("not found")
	ErrConflict = errors.New("already exists")
)

const schema = `
CREATE TABLE IF NOT EXISTS users (
	id                   INTEGER PRIMARY KEY,
	username             TEXT NOT NULL UNIQUE,
	password_hash        TEXT NOT NULL,
	role                 TEXT NOT NULL,
	totp_secret          BLOB,
	totp_last_step       INTEGER NOT NULL DEFAULT 0,
	failed_attempts      INTEGER NOT NULL DEFAULT 0,
	locked_until         INTEGER NOT NULL DEFAULT 0,
	disabled             INTEGER NOT NULL DEFAULT 0,
	must_change_password INTEGER NOT NULL DEFAULT 0,
	created_at           INTEGER NOT NULL,
	updated_at           INTEGER NOT NULL,
	last_login_at        INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS password_history (
	id            INTEGER PRIMARY KEY,
	user_id       INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	password_hash TEXT NOT NULL,
	created_at    INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS sessions (
	id_hash      TEXT PRIMARY KEY,
	user_id      INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	mfa_ok       INTEGER NOT NULL DEFAULT 0,
	csrf_token   TEXT NOT NULL,
	pending_totp BLOB,
	ip           TEXT NOT NULL DEFAULT '',
	user_agent   TEXT NOT NULL DEFAULT '',
	created_at   INTEGER NOT NULL,
	last_seen    INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS sessions_user ON sessions(user_id);

CREATE TABLE IF NOT EXISTS audit_events (
	id          INTEGER PRIMARY KEY,
	ts          INTEGER NOT NULL,
	actor       TEXT NOT NULL,
	ip          TEXT NOT NULL DEFAULT '',
	action      TEXT NOT NULL,
	resource    TEXT NOT NULL DEFAULT '',
	resource_id TEXT NOT NULL DEFAULT '',
	outcome     TEXT NOT NULL,
	detail      TEXT NOT NULL DEFAULT '',
	before      TEXT,
	after       TEXT
);
CREATE INDEX IF NOT EXISTS audit_ts ON audit_events(ts);
CREATE INDEX IF NOT EXISTS audit_resource ON audit_events(resource, resource_id);

CREATE TABLE IF NOT EXISTS acme_accounts (
	directory  TEXT PRIMARY KEY,
	account_key BLOB NOT NULL,
	uri        TEXT NOT NULL DEFAULT '',
	created_at INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS acme_certificates (
	id              INTEGER PRIMARY KEY,
	domains         TEXT NOT NULL,
	ssl_id          TEXT NOT NULL UNIQUE,
	environment     TEXT NOT NULL,
	key_type        TEXT NOT NULL,
	status          TEXT NOT NULL,
	last_error      TEXT NOT NULL DEFAULT '',
	log             TEXT NOT NULL DEFAULT '[]',
	not_before      INTEGER NOT NULL DEFAULT 0,
	not_after       INTEGER NOT NULL DEFAULT 0,
	fingerprint     TEXT NOT NULL DEFAULT '',
	issuer          TEXT NOT NULL DEFAULT '',
	failures        INTEGER NOT NULL DEFAULT 0,
	next_attempt_at INTEGER NOT NULL DEFAULT 0,
	last_attempt_at INTEGER NOT NULL DEFAULT 0,
	issued_at       INTEGER NOT NULL DEFAULT 0,
	created_by      TEXT NOT NULL,
	created_at      INTEGER NOT NULL,
	updated_at      INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS settings (
	key        TEXT PRIMARY KEY,
	value      TEXT NOT NULL,
	updated_by TEXT NOT NULL DEFAULT '',
	updated_at INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS kong_migrations (
	id          INTEGER PRIMARY KEY,
	kind        TEXT NOT NULL,
	kong_id     TEXT NOT NULL,
	kong_name   TEXT NOT NULL,
	resource    TEXT NOT NULL,
	apisix_id   TEXT NOT NULL,
	body_hash   TEXT NOT NULL,
	migrated_by TEXT NOT NULL,
	migrated_at INTEGER NOT NULL,
	UNIQUE (kind, kong_id)
);
`

type Store struct{ db *sql.DB }

func Open(path string) (*Store, error) {
	// Create the file up front so it is not world readable.
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600) // #nosec G304 -- database path derived from DASH_DATA_DIR
	if err != nil {
		return nil, err
	}
	f.Close()

	dsn := "file:" + path + "?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)&_pragma=synchronous(NORMAL)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	// A single connection serializes writes and keeps SQLite happy; the
	// dashboard has very low traffic.
	db.SetMaxOpenConns(1)
	if _, err := db.ExecContext(context.Background(), schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error { return s.db.Close() }

func toUnix(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.Unix()
}

func fromUnix(v int64) time.Time {
	if v == 0 {
		return time.Time{}
	}
	return time.Unix(v, 0)
}

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}

type scanner interface{ Scan(dest ...any) error }

// ---- users ----

type User struct {
	ID                 int64
	Username           string
	PasswordHash       string
	Role               string
	TOTPSecret         []byte // encrypted
	TOTPLastStep       int64
	FailedAttempts     int
	LockedUntil        time.Time
	Disabled           bool
	MustChangePassword bool
	CreatedAt          time.Time
	UpdatedAt          time.Time
	LastLoginAt        time.Time
}

const userCols = `id, username, password_hash, role, totp_secret, totp_last_step, failed_attempts,
	locked_until, disabled, must_change_password, created_at, updated_at, last_login_at`

func scanUser(row scanner) (*User, error) {
	var u User
	var locked, created, updated, lastLogin int64
	var disabled, mustChange int
	err := row.Scan(&u.ID, &u.Username, &u.PasswordHash, &u.Role, &u.TOTPSecret, &u.TOTPLastStep,
		&u.FailedAttempts, &locked, &disabled, &mustChange, &created, &updated, &lastLogin)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	u.LockedUntil = fromUnix(locked)
	u.Disabled = disabled == 1
	u.MustChangePassword = mustChange == 1
	u.CreatedAt = fromUnix(created)
	u.UpdatedAt = fromUnix(updated)
	u.LastLoginAt = fromUnix(lastLogin)
	return &u, nil
}

func (s *Store) CreateUser(ctx context.Context, u *User) error {
	res, err := s.db.ExecContext(ctx, `INSERT INTO users (username, password_hash, role, must_change_password, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)`, u.Username, u.PasswordHash, u.Role, b2i(u.MustChangePassword), toUnix(u.CreatedAt), toUnix(u.UpdatedAt))
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return ErrConflict
		}
		return err
	}
	u.ID, err = res.LastInsertId()
	return err
}

func (s *Store) GetUserByID(ctx context.Context, id int64) (*User, error) {
	return scanUser(s.db.QueryRowContext(ctx, `SELECT `+userCols+` FROM users WHERE id = ?`, id))
}

func (s *Store) GetUserByUsername(ctx context.Context, username string) (*User, error) {
	return scanUser(s.db.QueryRowContext(ctx, `SELECT `+userCols+` FROM users WHERE username = ?`, username))
}

func (s *Store) ListUsers(ctx context.Context) ([]*User, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+userCols+` FROM users ORDER BY username`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*User
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

func (s *Store) CountUsers(ctx context.Context) (n int, err error) {
	err = s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&n)
	return n, err
}

// CountActiveAdmins counts enabled users with the given admin role.
func (s *Store) CountActiveAdmins(ctx context.Context, role string) (n int, err error) {
	err = s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users WHERE role = ? AND disabled = 0`, role).Scan(&n)
	return n, err
}

// UpdateUser writes every mutable field of u.
func (s *Store) UpdateUser(ctx context.Context, u *User) error {
	_, err := s.db.ExecContext(ctx, `UPDATE users SET password_hash = ?, role = ?, totp_secret = ?, totp_last_step = ?,
		failed_attempts = ?, locked_until = ?, disabled = ?, must_change_password = ?, updated_at = ?, last_login_at = ?
		WHERE id = ?`,
		u.PasswordHash, u.Role, u.TOTPSecret, u.TOTPLastStep, u.FailedAttempts, toUnix(u.LockedUntil),
		b2i(u.Disabled), b2i(u.MustChangePassword), toUnix(u.UpdatedAt), toUnix(u.LastLoginAt), u.ID)
	return err
}

func (s *Store) DeleteUser(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM users WHERE id = ?`, id)
	return err
}

func (s *Store) AddPasswordHistory(ctx context.Context, userID int64, hash string, at time.Time) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO password_history (user_id, password_hash, created_at) VALUES (?, ?, ?)`,
		userID, hash, toUnix(at))
	return err
}

func (s *Store) RecentPasswordHashes(ctx context.Context, userID int64, n int) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT password_hash FROM password_history WHERE user_id = ? ORDER BY id DESC LIMIT ?`, userID, n)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var h string
		if err := rows.Scan(&h); err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

// ---- sessions ----

type Session struct {
	IDHash      string
	UserID      int64
	MFAOK       bool
	CSRFToken   string
	PendingTOTP []byte // encrypted secret while enrolling MFA
	IP          string
	UserAgent   string
	CreatedAt   time.Time
	LastSeen    time.Time
}

func (s *Store) CreateSession(ctx context.Context, se *Session) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO sessions (id_hash, user_id, mfa_ok, csrf_token, ip, user_agent, created_at, last_seen)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, se.IDHash, se.UserID, b2i(se.MFAOK), se.CSRFToken, se.IP, se.UserAgent,
		toUnix(se.CreatedAt), toUnix(se.LastSeen))
	return err
}

func (s *Store) GetSession(ctx context.Context, idHash string) (*Session, error) {
	var se Session
	var mfa int
	var created, seen int64
	err := s.db.QueryRowContext(ctx, `SELECT id_hash, user_id, mfa_ok, csrf_token, pending_totp, ip, user_agent, created_at, last_seen
		FROM sessions WHERE id_hash = ?`, idHash).
		Scan(&se.IDHash, &se.UserID, &mfa, &se.CSRFToken, &se.PendingTOTP, &se.IP, &se.UserAgent, &created, &seen)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	se.MFAOK = mfa == 1
	se.CreatedAt = fromUnix(created)
	se.LastSeen = fromUnix(seen)
	return &se, nil
}

func (s *Store) UpdateSession(ctx context.Context, se *Session) error {
	_, err := s.db.ExecContext(ctx, `UPDATE sessions SET mfa_ok = ?, pending_totp = ?, last_seen = ? WHERE id_hash = ?`,
		b2i(se.MFAOK), se.PendingTOTP, toUnix(se.LastSeen), se.IDHash)
	return err
}

func (s *Store) DeleteSession(ctx context.Context, idHash string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE id_hash = ?`, idHash)
	return err
}

// DeleteUserSessions removes all sessions of a user except keepHash (may be empty).
func (s *Store) DeleteUserSessions(ctx context.Context, userID int64, keepHash string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE user_id = ? AND id_hash <> ?`, userID, keepHash)
	return err
}

// PurgeSessions deletes sessions idle since before idleCutoff (pendingCutoff
// for sessions that never completed MFA) or created before maxCutoff.
func (s *Store) PurgeSessions(ctx context.Context, idleCutoff, pendingCutoff, maxCutoff time.Time) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE (mfa_ok = 1 AND last_seen < ?) OR (mfa_ok = 0 AND last_seen < ?) OR created_at < ?`,
		toUnix(idleCutoff), toUnix(pendingCutoff), toUnix(maxCutoff))
	return err
}

// ---- audit ----

type AuditEvent struct {
	ID         int64           `json:"id"`
	Time       time.Time       `json:"ts"`
	Actor      string          `json:"actor"`
	IP         string          `json:"ip"`
	Action     string          `json:"action"`
	Resource   string          `json:"resource,omitempty"`
	ResourceID string          `json:"resource_id,omitempty"`
	Outcome    string          `json:"outcome"`
	Detail     string          `json:"detail,omitempty"`
	Before     json.RawMessage `json:"before,omitempty"`
	After      json.RawMessage `json:"after,omitempty"`
}

func nullJSON(m json.RawMessage) sql.NullString {
	if len(m) == 0 {
		return sql.NullString{}
	}
	return sql.NullString{String: string(m), Valid: true}
}

func (s *Store) InsertAudit(ctx context.Context, e *AuditEvent) error {
	res, err := s.db.ExecContext(ctx, `INSERT INTO audit_events (ts, actor, ip, action, resource, resource_id, outcome, detail, before, after)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, e.Time.UnixMilli(), e.Actor, e.IP, e.Action, e.Resource, e.ResourceID,
		e.Outcome, e.Detail, nullJSON(e.Before), nullJSON(e.After))
	if err != nil {
		return err
	}
	e.ID, err = res.LastInsertId()
	return err
}

// ListAudit returns events newest first. beforeID > 0 paginates.
func (s *Store) ListAudit(ctx context.Context, limit int, beforeID int64) ([]*AuditEvent, error) {
	q := `SELECT id, ts, actor, ip, action, resource, resource_id, outcome, detail, before, after FROM audit_events`
	args := []any{}
	if beforeID > 0 {
		q += ` WHERE id < ?`
		args = append(args, beforeID)
	}
	q += ` ORDER BY id DESC LIMIT ?`
	args = append(args, limit)

	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*AuditEvent{}
	for rows.Next() {
		var e AuditEvent
		var ts int64
		var before, after sql.NullString
		if err := rows.Scan(&e.ID, &ts, &e.Actor, &e.IP, &e.Action, &e.Resource, &e.ResourceID, &e.Outcome, &e.Detail, &before, &after); err != nil {
			return nil, err
		}
		e.Time = time.UnixMilli(ts)
		if before.Valid {
			e.Before = json.RawMessage(before.String)
		}
		if after.Valid {
			e.After = json.RawMessage(after.String)
		}
		out = append(out, &e)
	}
	return out, rows.Err()
}

// ObjectHistory returns the successful audit events with the given actions
// for one APISIX object, oldest first (used to find out how a migration
// created or changed it).
func (s *Store) ObjectHistory(ctx context.Context, resource, resourceID string, actions ...string) ([]*AuditEvent, error) {
	if len(actions) == 0 {
		return []*AuditEvent{}, nil
	}
	placeholders := "?" + strings.Repeat(", ?", len(actions)-1)
	q := `SELECT id, ts, actor, ip, action, resource, resource_id, outcome, detail, before, after FROM audit_events
		WHERE resource = ? AND resource_id = ? AND outcome = 'success' AND action IN (` + placeholders + `) ORDER BY id` // #nosec G202 -- only "?" placeholders are concatenated; every value is bound
	args := []any{resource, resourceID}
	for _, a := range actions {
		args = append(args, a)
	}
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*AuditEvent{}
	for rows.Next() {
		var e AuditEvent
		var ts int64
		var before, after sql.NullString
		if err := rows.Scan(&e.ID, &ts, &e.Actor, &e.IP, &e.Action, &e.Resource, &e.ResourceID, &e.Outcome, &e.Detail, &before, &after); err != nil {
			return nil, err
		}
		e.Time = time.UnixMilli(ts)
		if before.Valid {
			e.Before = json.RawMessage(before.String)
		}
		if after.Valid {
			e.After = json.RawMessage(after.String)
		}
		out = append(out, &e)
	}
	return out, rows.Err()
}

// ---- ACME ----

// ACMEAccount is the CA account used to order certificates. Key is the
// encrypted PKCS#8 account key.
type ACMEAccount struct {
	Directory string
	Key       []byte
	URI       string
	CreatedAt time.Time
}

func (s *Store) GetACMEAccount(ctx context.Context, directory string) (*ACMEAccount, error) {
	var a ACMEAccount
	var created int64
	err := s.db.QueryRowContext(ctx, `SELECT directory, account_key, uri, created_at FROM acme_accounts WHERE directory = ?`, directory).
		Scan(&a.Directory, &a.Key, &a.URI, &created)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	a.CreatedAt = fromUnix(created)
	return &a, nil
}

func (s *Store) SaveACMEAccount(ctx context.Context, a *ACMEAccount) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO acme_accounts (directory, account_key, uri, created_at) VALUES (?, ?, ?, ?)
		ON CONFLICT(directory) DO UPDATE SET account_key = excluded.account_key, uri = excluded.uri`,
		a.Directory, a.Key, a.URI, toUnix(a.CreatedAt))
	return err
}

// Certificate is a certificate the dashboard issues and renews. Private keys
// are never stored here: they only exist in APISIX.
type Certificate struct {
	ID            int64
	Domains       []string
	SSLID         string
	Environment   string
	KeyType       string
	Status        string
	LastError     string
	Log           json.RawMessage
	NotBefore     time.Time
	NotAfter      time.Time
	Fingerprint   string
	Issuer        string
	Failures      int
	NextAttemptAt time.Time
	LastAttemptAt time.Time
	IssuedAt      time.Time
	CreatedBy     string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

const certCols = `id, domains, ssl_id, environment, key_type, status, last_error, log, not_before, not_after,
	fingerprint, issuer, failures, next_attempt_at, last_attempt_at, issued_at, created_by, created_at, updated_at`

func scanCert(row scanner) (*Certificate, error) {
	var c Certificate
	var domains, log string
	var nb, na, next, last, issued, created, updated int64
	err := row.Scan(&c.ID, &domains, &c.SSLID, &c.Environment, &c.KeyType, &c.Status, &c.LastError, &log, &nb, &na,
		&c.Fingerprint, &c.Issuer, &c.Failures, &next, &last, &issued, &c.CreatedBy, &created, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal([]byte(domains), &c.Domains); err != nil {
		return nil, err
	}
	c.Log = json.RawMessage(log)
	c.NotBefore, c.NotAfter = fromUnix(nb), fromUnix(na)
	c.NextAttemptAt, c.LastAttemptAt, c.IssuedAt = fromUnix(next), fromUnix(last), fromUnix(issued)
	c.CreatedAt, c.UpdatedAt = fromUnix(created), fromUnix(updated)
	return &c, nil
}

func (s *Store) CreateCertificate(ctx context.Context, c *Certificate) error {
	domains, _ := json.Marshal(c.Domains)
	res, err := s.db.ExecContext(ctx, `INSERT INTO acme_certificates (domains, ssl_id, environment, key_type, status, created_by, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, string(domains), c.SSLID, c.Environment, c.KeyType, c.Status, c.CreatedBy,
		toUnix(c.CreatedAt), toUnix(c.UpdatedAt))
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return ErrConflict
		}
		return err
	}
	c.ID, err = res.LastInsertId()
	return err
}

func (s *Store) GetCertificate(ctx context.Context, id int64) (*Certificate, error) {
	return scanCert(s.db.QueryRowContext(ctx, `SELECT `+certCols+` FROM acme_certificates WHERE id = ?`, id))
}

func (s *Store) GetCertificateBySSLID(ctx context.Context, sslID string) (*Certificate, error) {
	return scanCert(s.db.QueryRowContext(ctx, `SELECT `+certCols+` FROM acme_certificates WHERE ssl_id = ?`, sslID))
}

func (s *Store) ListCertificates(ctx context.Context) ([]*Certificate, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+certCols+` FROM acme_certificates ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*Certificate{}
	for rows.Next() {
		c, err := scanCert(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// UpdateCertificate writes the issuance state of c.
func (s *Store) UpdateCertificate(ctx context.Context, c *Certificate) error {
	log := string(c.Log)
	if log == "" {
		log = "[]"
	}
	_, err := s.db.ExecContext(ctx, `UPDATE acme_certificates SET status = ?, last_error = ?, log = ?, not_before = ?, not_after = ?,
		fingerprint = ?, issuer = ?, failures = ?, next_attempt_at = ?, last_attempt_at = ?, issued_at = ?, updated_at = ?
		WHERE id = ?`,
		c.Status, c.LastError, log, toUnix(c.NotBefore), toUnix(c.NotAfter), c.Fingerprint, c.Issuer, c.Failures,
		toUnix(c.NextAttemptAt), toUnix(c.LastAttemptAt), toUnix(c.IssuedAt), toUnix(c.UpdatedAt), c.ID)
	return err
}

func (s *Store) DeleteCertificate(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM acme_certificates WHERE id = ?`, id)
	return err
}

// ---- settings ----

// GetSetting returns a JSON document stored under key.
func (s *Store) GetSetting(ctx context.Context, key string) (json.RawMessage, error) {
	var v string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = ?`, key).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return json.RawMessage(v), err
}

func (s *Store) PutSetting(ctx context.Context, key string, value json.RawMessage, by string, at time.Time) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO settings (key, value, updated_by, updated_at) VALUES (?, ?, ?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_by = excluded.updated_by, updated_at = excluded.updated_at`,
		key, string(value), by, toUnix(at))
	return err
}

// ---- Kong migration ----

// KongMigration records an object migrated from Kong to APISIX, so later
// steps know what the migration owns and whether Kong changed since.
type KongMigration struct {
	ID         int64
	Kind       string
	KongID     string
	KongName   string
	Resource   string
	APISIXID   string
	BodyHash   string
	MigratedBy string
	MigratedAt time.Time
}

// Path is the Admin API path of the migrated object.
func (m *KongMigration) Path() string { return m.Resource + "/" + m.APISIXID }

func (s *Store) UpsertKongMigration(ctx context.Context, m *KongMigration) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO kong_migrations (kind, kong_id, kong_name, resource, apisix_id, body_hash, migrated_by, migrated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(kind, kong_id) DO UPDATE SET kong_name = excluded.kong_name, resource = excluded.resource, apisix_id = excluded.apisix_id,
			body_hash = excluded.body_hash, migrated_by = excluded.migrated_by, migrated_at = excluded.migrated_at`,
		m.Kind, m.KongID, m.KongName, m.Resource, m.APISIXID, m.BodyHash, m.MigratedBy, toUnix(m.MigratedAt))
	return err
}

func (s *Store) DeleteKongMigration(ctx context.Context, kind, kongID string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM kong_migrations WHERE kind = ? AND kong_id = ?`, kind, kongID)
	return err
}

func (s *Store) ListKongMigrations(ctx context.Context) ([]*KongMigration, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, kind, kong_id, kong_name, resource, apisix_id, body_hash, migrated_by, migrated_at
		FROM kong_migrations ORDER BY migrated_at DESC, id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*KongMigration{}
	for rows.Next() {
		var m KongMigration
		var at int64
		if err := rows.Scan(&m.ID, &m.Kind, &m.KongID, &m.KongName, &m.Resource, &m.APISIXID, &m.BodyHash, &m.MigratedBy, &at); err != nil {
			return nil, err
		}
		m.MigratedAt = fromUnix(at)
		out = append(out, &m)
	}
	return out, rows.Err()
}
