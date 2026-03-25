package db

import (
	"database/sql"
	"strings"
	"time"
)

func NormalizeUserRole(role string) string {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case "admin":
		return "admin"
	case "editor":
		return "editor"
	default:
		return "viewer"
	}
}

func (d *DB) CountUsers() (int, error) {
	var count int
	if err := d.sql.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func (d *DB) ListUsers() ([]User, error) {
	rows, err := d.sql.Query(`
		SELECT id, username, password_hash, role, enabled, last_login_at, created_at, updated_at
		FROM users
		ORDER BY username ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []User
	for rows.Next() {
		user, err := scanUserRow(rows)
		if err != nil {
			return nil, err
		}
		users = append(users, *user)
	}
	return users, rows.Err()
}

func (d *DB) GetUserByID(id int64) (*User, error) {
	row := d.sql.QueryRow(`
		SELECT id, username, password_hash, role, enabled, last_login_at, created_at, updated_at
		FROM users
		WHERE id = ?
	`, id)
	user, err := scanUser(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return user, err
}

func (d *DB) GetUserByUsername(username string) (*User, error) {
	row := d.sql.QueryRow(`
		SELECT id, username, password_hash, role, enabled, last_login_at, created_at, updated_at
		FROM users
		WHERE lower(username) = lower(?)
	`, strings.TrimSpace(username))
	user, err := scanUser(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return user, err
}

func (d *DB) CreateUser(username, passwordHash, role string, enabled bool) (*User, error) {
	res, err := d.sql.Exec(`
		INSERT INTO users (username, password_hash, role, enabled)
		VALUES (?, ?, ?, ?)
	`, strings.TrimSpace(username), passwordHash, NormalizeUserRole(role), enabled)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return d.GetUserByID(id)
}

func (d *DB) UpdateUser(id int64, username, role string, enabled bool, passwordHash *string) error {
	if passwordHash != nil {
		_, err := d.sql.Exec(`
			UPDATE users
			SET username = ?, password_hash = ?, role = ?, enabled = ?, updated_at = CURRENT_TIMESTAMP
			WHERE id = ?
		`, strings.TrimSpace(username), *passwordHash, NormalizeUserRole(role), enabled, id)
		return err
	}

	_, err := d.sql.Exec(`
		UPDATE users
		SET username = ?, role = ?, enabled = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, strings.TrimSpace(username), NormalizeUserRole(role), enabled, id)
	return err
}

func (d *DB) DeleteUser(id int64) error {
	_, err := d.sql.Exec(`DELETE FROM users WHERE id = ?`, id)
	return err
}

func (d *DB) UpdateUserLastLogin(id int64, at time.Time) error {
	_, err := d.sql.Exec(`UPDATE users SET last_login_at = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`, at, id)
	return err
}

func (d *DB) CreateSession(userID int64, tokenHash string, expiresAt time.Time, userAgent, remoteAddr string) (*Session, error) {
	res, err := d.sql.Exec(`
		INSERT INTO user_sessions (user_id, token_hash, expires_at, user_agent, remote_addr)
		VALUES (?, ?, ?, ?, ?)
	`, userID, tokenHash, expiresAt, strings.TrimSpace(userAgent), strings.TrimSpace(remoteAddr))
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return d.GetSessionByID(id)
}

func (d *DB) GetSessionByID(id int64) (*Session, error) {
	row := d.sql.QueryRow(`
		SELECT id, user_id, token_hash, expires_at, created_at, last_seen_at, user_agent, remote_addr
		FROM user_sessions
		WHERE id = ?
	`, id)
	session, err := scanSession(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return session, err
}

func (d *DB) GetUserBySessionTokenHash(tokenHash string) (*User, *Session, error) {
	row := d.sql.QueryRow(`
		SELECT
			u.id, u.username, u.password_hash, u.role, u.enabled, u.last_login_at, u.created_at, u.updated_at,
			s.id, s.user_id, s.token_hash, s.expires_at, s.created_at, s.last_seen_at, s.user_agent, s.remote_addr
		FROM user_sessions s
		INNER JOIN users u ON u.id = s.user_id
		WHERE s.token_hash = ?
	`, tokenHash)

	var user User
	var session Session
	var lastLogin sql.NullTime
	err := row.Scan(
		&user.ID, &user.Username, &user.PasswordHash, &user.Role, &user.Enabled, &lastLogin, &user.CreatedAt, &user.UpdatedAt,
		&session.ID, &session.UserID, &session.TokenHash, &session.ExpiresAt, &session.CreatedAt, &session.LastSeenAt, &session.UserAgent, &session.RemoteAddr,
	)
	if err == sql.ErrNoRows {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	if lastLogin.Valid {
		user.LastLoginAt = &lastLogin.Time
	}
	return &user, &session, nil
}

func (d *DB) TouchSession(tokenHash string, at time.Time) error {
	_, err := d.sql.Exec(`UPDATE user_sessions SET last_seen_at = ? WHERE token_hash = ?`, at, tokenHash)
	return err
}

func (d *DB) DeleteSession(tokenHash string) error {
	_, err := d.sql.Exec(`DELETE FROM user_sessions WHERE token_hash = ?`, tokenHash)
	return err
}

func (d *DB) DeleteSessionsByUser(userID int64) error {
	_, err := d.sql.Exec(`DELETE FROM user_sessions WHERE user_id = ?`, userID)
	return err
}

func (d *DB) DeleteExpiredSessions(now time.Time) error {
	_, err := d.sql.Exec(`DELETE FROM user_sessions WHERE expires_at <= ?`, now)
	return err
}

func scanUser(row *sql.Row) (*User, error) {
	var user User
	var lastLogin sql.NullTime
	err := row.Scan(&user.ID, &user.Username, &user.PasswordHash, &user.Role, &user.Enabled, &lastLogin, &user.CreatedAt, &user.UpdatedAt)
	if err != nil {
		return nil, err
	}
	if lastLogin.Valid {
		user.LastLoginAt = &lastLogin.Time
	}
	return &user, nil
}

func scanUserRow(rows *sql.Rows) (*User, error) {
	var user User
	var lastLogin sql.NullTime
	if err := rows.Scan(&user.ID, &user.Username, &user.PasswordHash, &user.Role, &user.Enabled, &lastLogin, &user.CreatedAt, &user.UpdatedAt); err != nil {
		return nil, err
	}
	if lastLogin.Valid {
		user.LastLoginAt = &lastLogin.Time
	}
	return &user, nil
}

func scanSession(row *sql.Row) (*Session, error) {
	var session Session
	err := row.Scan(&session.ID, &session.UserID, &session.TokenHash, &session.ExpiresAt, &session.CreatedAt, &session.LastSeenAt, &session.UserAgent, &session.RemoteAddr)
	if err != nil {
		return nil, err
	}
	return &session, nil
}
