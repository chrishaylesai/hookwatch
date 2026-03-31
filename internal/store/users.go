package store

import (
	"context"
	"fmt"
	"time"

	"github.com/chrishaylesai/hookwatch/internal/models"
	"zombiezen.com/go/sqlite"
	"zombiezen.com/go/sqlite/sqlitex"
)

// CreateUser inserts a new user into the database.
func (s *Store) CreateUser(ctx context.Context, user *models.User) error {
	conn, err := s.take(ctx)
	if err != nil {
		return err
	}
	defer s.pool.Put(conn)

	query := `INSERT INTO users (
		id, email, display_name, password_hash, oidc_provider, oidc_subject,
		global_role, created_at, updated_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`

	err = sqlitex.ExecuteTransient(conn, query, &sqlitex.ExecOptions{
		Args: []any{
			user.ID, user.Email, user.DisplayName,
			nullableString(user.PasswordHash), nullableString(user.OIDCProvider), nullableString(user.OIDCSubject),
			user.GlobalRole, formatTime(user.CreatedAt), formatTime(user.UpdatedAt),
		},
	})
	if err != nil {
		return fmt.Errorf("insert user: %w", err)
	}

	return nil
}

// GetUser retrieves a user by ID.
func (s *Store) GetUser(ctx context.Context, id string) (*models.User, error) {
	conn, err := s.take(ctx)
	if err != nil {
		return nil, err
	}
	defer s.pool.Put(conn)

	var user *models.User
	err = sqlitex.ExecuteTransient(conn, `SELECT
		id, email, display_name, password_hash, oidc_provider, oidc_subject,
		global_role, created_at, updated_at
	FROM users WHERE id = ?`, &sqlitex.ExecOptions{
		Args: []any{id},
		ResultFunc: func(stmt *sqlite.Stmt) error {
			var scanErr error
			user, scanErr = scanUser(stmt)
			return scanErr
		},
	})
	if err != nil {
		return nil, fmt.Errorf("get user: %w", err)
	}
	if user == nil {
		return nil, ErrNotFound
	}
	return user, nil
}

// GetUserByEmail retrieves a user by email address.
func (s *Store) GetUserByEmail(ctx context.Context, email string) (*models.User, error) {
	conn, err := s.take(ctx)
	if err != nil {
		return nil, err
	}
	defer s.pool.Put(conn)

	var user *models.User
	err = sqlitex.ExecuteTransient(conn, `SELECT
		id, email, display_name, password_hash, oidc_provider, oidc_subject,
		global_role, created_at, updated_at
	FROM users WHERE email = ?`, &sqlitex.ExecOptions{
		Args: []any{email},
		ResultFunc: func(stmt *sqlite.Stmt) error {
			var scanErr error
			user, scanErr = scanUser(stmt)
			return scanErr
		},
	})
	if err != nil {
		return nil, fmt.Errorf("get user by email: %w", err)
	}
	if user == nil {
		return nil, ErrNotFound
	}
	return user, nil
}

// GetUserByOIDC retrieves a user by OIDC provider and subject.
func (s *Store) GetUserByOIDC(ctx context.Context, provider, subject string) (*models.User, error) {
	conn, err := s.take(ctx)
	if err != nil {
		return nil, err
	}
	defer s.pool.Put(conn)

	var user *models.User
	err = sqlitex.ExecuteTransient(conn, `SELECT
		id, email, display_name, password_hash, oidc_provider, oidc_subject,
		global_role, created_at, updated_at
	FROM users WHERE oidc_provider = ? AND oidc_subject = ?`, &sqlitex.ExecOptions{
		Args: []any{provider, subject},
		ResultFunc: func(stmt *sqlite.Stmt) error {
			var scanErr error
			user, scanErr = scanUser(stmt)
			return scanErr
		},
	})
	if err != nil {
		return nil, fmt.Errorf("get user by oidc: %w", err)
	}
	if user == nil {
		return nil, ErrNotFound
	}
	return user, nil
}

// CountUsers returns the total number of users.
func (s *Store) CountUsers(ctx context.Context) (int, error) {
	conn, err := s.take(ctx)
	if err != nil {
		return 0, err
	}
	defer s.pool.Put(conn)

	total, err := queryCount(conn, "SELECT COUNT(*) FROM users", nil)
	if err != nil {
		return 0, fmt.Errorf("count users: %w", err)
	}
	return total, nil
}

// ListUsers retrieves all users with pagination.
func (s *Store) ListUsers(ctx context.Context, limit, offset int) ([]*models.User, int, error) {
	conn, err := s.take(ctx)
	if err != nil {
		return nil, 0, err
	}
	defer s.pool.Put(conn)

	limit = normalizeLimit(limit)
	offset = normalizeOffset(offset)

	total, err := queryCount(conn, "SELECT COUNT(*) FROM users", nil)
	if err != nil {
		return nil, 0, fmt.Errorf("count users: %w", err)
	}

	var users []*models.User
	err = sqlitex.ExecuteTransient(conn, `SELECT
		id, email, display_name, password_hash, oidc_provider, oidc_subject,
		global_role, created_at, updated_at
	FROM users ORDER BY created_at DESC LIMIT ? OFFSET ?`, &sqlitex.ExecOptions{
		Args: []any{limit, offset},
		ResultFunc: func(stmt *sqlite.Stmt) error {
			user, scanErr := scanUser(stmt)
			if scanErr != nil {
				return scanErr
			}
			users = append(users, user)
			return nil
		},
	})
	if err != nil {
		return nil, 0, fmt.Errorf("list users: %w", err)
	}

	return users, total, nil
}

// UpdateUser updates a user's mutable fields.
func (s *Store) UpdateUser(ctx context.Context, user *models.User) error {
	conn, err := s.take(ctx)
	if err != nil {
		return err
	}
	defer s.pool.Put(conn)

	query := `UPDATE users SET
		email = ?, display_name = ?, password_hash = ?,
		global_role = ?, updated_at = ?
	WHERE id = ?`

	err = sqlitex.ExecuteTransient(conn, query, &sqlitex.ExecOptions{
		Args: []any{
			user.Email, user.DisplayName, nullableString(user.PasswordHash),
			user.GlobalRole, formatTime(user.UpdatedAt), user.ID,
		},
	})
	if err != nil {
		return fmt.Errorf("update user: %w", err)
	}
	if conn.Changes() == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteUser removes a user by ID.
func (s *Store) DeleteUser(ctx context.Context, id string) error {
	conn, err := s.take(ctx)
	if err != nil {
		return err
	}
	defer s.pool.Put(conn)

	err = sqlitex.ExecuteTransient(conn, "DELETE FROM users WHERE id = ?", &sqlitex.ExecOptions{
		Args: []any{id},
	})
	if err != nil {
		return fmt.Errorf("delete user: %w", err)
	}
	if conn.Changes() == 0 {
		return ErrNotFound
	}
	return nil
}

func scanUser(stmt *sqlite.Stmt) (*models.User, error) {
	user := &models.User{
		ID:          stmt.ColumnText(0),
		Email:       stmt.ColumnText(1),
		DisplayName: stmt.ColumnText(2),
		GlobalRole:  stmt.ColumnText(6),
	}
	if !stmt.ColumnIsNull(3) {
		hash := stmt.ColumnText(3)
		user.PasswordHash = &hash
	}
	if !stmt.ColumnIsNull(4) {
		provider := stmt.ColumnText(4)
		user.OIDCProvider = &provider
	}
	if !stmt.ColumnIsNull(5) {
		subject := stmt.ColumnText(5)
		user.OIDCSubject = &subject
	}

	var err error
	user.CreatedAt, err = parseTime(stmt.ColumnText(7))
	if err != nil {
		return nil, fmt.Errorf("parse user created_at: %w", err)
	}
	user.UpdatedAt, err = parseTime(stmt.ColumnText(8))
	if err != nil {
		return nil, fmt.Errorf("parse user updated_at: %w", err)
	}
	return user, nil
}

// CreateSession inserts a new session.
func (s *Store) CreateSession(ctx context.Context, session *models.Session) error {
	conn, err := s.take(ctx)
	if err != nil {
		return err
	}
	defer s.pool.Put(conn)

	query := `INSERT INTO sessions (id, user_id, created_at, expires_at, ip, user_agent)
	VALUES (?, ?, ?, ?, ?, ?)`

	err = sqlitex.ExecuteTransient(conn, query, &sqlitex.ExecOptions{
		Args: []any{
			session.ID, session.UserID, formatTime(session.CreatedAt),
			formatTime(session.ExpiresAt), session.IP, session.UserAgent,
		},
	})
	if err != nil {
		return fmt.Errorf("insert session: %w", err)
	}
	return nil
}

// GetSession retrieves a valid (non-expired) session by ID.
func (s *Store) GetSession(ctx context.Context, id string) (*models.Session, error) {
	conn, err := s.take(ctx)
	if err != nil {
		return nil, err
	}
	defer s.pool.Put(conn)

	now := formatTime(time.Now().UTC())
	var session *models.Session
	err = sqlitex.ExecuteTransient(conn, `SELECT
		id, user_id, created_at, expires_at, ip, user_agent
	FROM sessions WHERE id = ? AND expires_at > ?`, &sqlitex.ExecOptions{
		Args: []any{id, now},
		ResultFunc: func(stmt *sqlite.Stmt) error {
			var scanErr error
			session, scanErr = scanSession(stmt)
			return scanErr
		},
	})
	if err != nil {
		return nil, fmt.Errorf("get session: %w", err)
	}
	if session == nil {
		return nil, ErrNotFound
	}
	return session, nil
}

// DeleteSession removes a session by ID.
func (s *Store) DeleteSession(ctx context.Context, id string) error {
	conn, err := s.take(ctx)
	if err != nil {
		return err
	}
	defer s.pool.Put(conn)

	err = sqlitex.ExecuteTransient(conn, "DELETE FROM sessions WHERE id = ?", &sqlitex.ExecOptions{
		Args: []any{id},
	})
	if err != nil {
		return fmt.Errorf("delete session: %w", err)
	}
	return nil
}

// DeleteUserSessions removes all sessions for a user.
func (s *Store) DeleteUserSessions(ctx context.Context, userID string) error {
	conn, err := s.take(ctx)
	if err != nil {
		return err
	}
	defer s.pool.Put(conn)

	err = sqlitex.ExecuteTransient(conn, "DELETE FROM sessions WHERE user_id = ?", &sqlitex.ExecOptions{
		Args: []any{userID},
	})
	if err != nil {
		return fmt.Errorf("delete user sessions: %w", err)
	}
	return nil
}

// DeleteExpiredSessions removes all expired sessions.
func (s *Store) DeleteExpiredSessions(ctx context.Context) (int, error) {
	conn, err := s.take(ctx)
	if err != nil {
		return 0, err
	}
	defer s.pool.Put(conn)

	now := formatTime(time.Now().UTC())
	err = sqlitex.ExecuteTransient(conn, "DELETE FROM sessions WHERE expires_at <= ?", &sqlitex.ExecOptions{
		Args: []any{now},
	})
	if err != nil {
		return 0, fmt.Errorf("delete expired sessions: %w", err)
	}
	return conn.Changes(), nil
}

func scanSession(stmt *sqlite.Stmt) (*models.Session, error) {
	session := &models.Session{
		ID:        stmt.ColumnText(0),
		UserID:    stmt.ColumnText(1),
		IP:        stmt.ColumnText(4),
		UserAgent: stmt.ColumnText(5),
	}

	var err error
	session.CreatedAt, err = parseTime(stmt.ColumnText(2))
	if err != nil {
		return nil, fmt.Errorf("parse session created_at: %w", err)
	}
	session.ExpiresAt, err = parseTime(stmt.ColumnText(3))
	if err != nil {
		return nil, fmt.Errorf("parse session expires_at: %w", err)
	}
	return session, nil
}
