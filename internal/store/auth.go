package store

import (
	"database/sql"
	"errors"
	"time"
)

// SessionInfo holds the identity attached to a session. For local (single-user)
// auth mode all identity fields except IsLocal are empty. For OIDC mode they are
// populated from the ID token / userinfo claims.
type SessionInfo struct {
	Token    string
	Sub      string
	Username string
	Email    string
	// Groups is the raw OIDC groups claim serialised as a JSON string array.
	Groups  string
	IsLocal bool
}

// migrateAuth performs additive schema upgrades for OIDC support on the existing
// sessions table and creates the audit_log table. It is safe to run repeatedly.
func (s *Store) migrateAuth() error {
	// Add identity columns to sessions if they do not already exist. SQLite has
	// no "ADD COLUMN IF NOT EXISTS", so inspect the table first.
	cols, err := s.columns("sessions")
	if err != nil {
		return err
	}
	adds := map[string]string{
		"sub":      "TEXT NOT NULL DEFAULT ''",
		"username": "TEXT NOT NULL DEFAULT ''",
		"email":    "TEXT NOT NULL DEFAULT ''",
		"groups":   "TEXT NOT NULL DEFAULT '[]'",
		"is_local": "INTEGER NOT NULL DEFAULT 0",
	}
	for col, def := range adds {
		if cols[col] {
			continue
		}
		if _, err := s.db.Exec("ALTER TABLE sessions ADD COLUMN " + col + " " + def); err != nil {
			return err
		}
	}

	_, err = s.db.Exec(`
		CREATE TABLE IF NOT EXISTS audit_log (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			created_at INTEGER NOT NULL,
			sub        TEXT NOT NULL DEFAULT '',
			username   TEXT NOT NULL DEFAULT '',
			action     TEXT NOT NULL,
			target     TEXT NOT NULL DEFAULT '',
			detail     TEXT NOT NULL DEFAULT ''
		);
		CREATE INDEX IF NOT EXISTS idx_audit_created_at ON audit_log(created_at DESC);
	`)
	return err
}

// columns returns the set of column names present on a table.
func (s *Store) columns(table string) (map[string]bool, error) {
	rows, err := s.db.Query("SELECT name FROM pragma_table_info(?)", table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[string]bool)
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		out[name] = true
	}
	return out, rows.Err()
}

// CreateOIDCSession stores a session carrying OIDC identity and the raw groups
// claim (JSON array string).
func (s *Store) CreateOIDCSession(token string, ttl time.Duration, sub, username, email, groupsJSON string) error {
	now := time.Now()
	if groupsJSON == "" {
		groupsJSON = "[]"
	}
	_, err := s.db.Exec(
		`INSERT INTO sessions (token, created_at, expires_at, sub, username, email, groups, is_local)
		 VALUES (?, ?, ?, ?, ?, ?, ?, 0)`,
		token, now.Unix(), now.Add(ttl).Unix(), sub, username, email, groupsJSON,
	)
	return err
}

// CreateLocalSession stores a session for the single-user local auth mode.
func (s *Store) CreateLocalSession(token string, ttl time.Duration) error {
	now := time.Now()
	_, err := s.db.Exec(
		`INSERT INTO sessions (token, created_at, expires_at, is_local) VALUES (?, ?, ?, 1)`,
		token, now.Unix(), now.Add(ttl).Unix(),
	)
	return err
}

// SessionByToken returns the session identity for a valid, unexpired token.
// found is false when the token is unknown or expired. Expired sessions are
// lazily pruned.
func (s *Store) SessionByToken(token string) (SessionInfo, bool, error) {
	_, _ = s.db.Exec(`DELETE FROM sessions WHERE expires_at < ?`, time.Now().Unix())

	var info SessionInfo
	var isLocal int
	err := s.db.QueryRow(
		`SELECT token, sub, username, email, groups, is_local FROM sessions
		 WHERE token = ? AND expires_at >= ?`,
		token, time.Now().Unix(),
	).Scan(&info.Token, &info.Sub, &info.Username, &info.Email, &info.Groups, &isLocal)
	if errors.Is(err, sql.ErrNoRows) {
		return SessionInfo{}, false, nil
	}
	if err != nil {
		return SessionInfo{}, false, err
	}
	info.IsLocal = isLocal != 0
	return info, true, nil
}

// AuditEntry is one recorded state-changing action.
type AuditEntry struct {
	ID        int64
	CreatedAt time.Time
	Sub       string
	Username  string
	Action    string
	Target    string
	Detail    string
}

// AppendAudit records an action performed by a user.
func (s *Store) AppendAudit(sub, username, action, target, detail string) error {
	_, err := s.db.Exec(
		`INSERT INTO audit_log (created_at, sub, username, action, target, detail)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		time.Now().Unix(), sub, username, action, target, detail,
	)
	return err
}

// ListAudit returns the most recent audit entries, newest first, up to limit.
func (s *Store) ListAudit(limit int) ([]AuditEntry, error) {
	if limit <= 0 || limit > 1000 {
		limit = 200
	}
	rows, err := s.db.Query(
		`SELECT id, created_at, sub, username, action, target, detail
		 FROM audit_log ORDER BY id DESC LIMIT ?`, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AuditEntry
	for rows.Next() {
		var e AuditEntry
		var ts int64
		if err := rows.Scan(&e.ID, &ts, &e.Sub, &e.Username, &e.Action, &e.Target, &e.Detail); err != nil {
			return nil, err
		}
		e.CreatedAt = time.Unix(ts, 0)
		out = append(out, e)
	}
	return out, rows.Err()
}
