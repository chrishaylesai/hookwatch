package store

import (
	"context"
	"fmt"

	"github.com/chrishaylesai/hookwatch/internal/models"
	"zombiezen.com/go/sqlite"
	"zombiezen.com/go/sqlite/sqlitex"
)

// CreateHookGrant inserts a new hook grant.
func (s *Store) CreateHookGrant(ctx context.Context, grant *models.HookGrant) error {
	conn, err := s.take(ctx)
	if err != nil {
		return err
	}
	defer s.pool.Put(conn)

	query := `INSERT INTO hook_grants (id, token_id, user_id, role, granted_by, created_at)
	VALUES (?, ?, ?, ?, ?, ?)`

	err = sqlitex.ExecuteTransient(conn, query, &sqlitex.ExecOptions{
		Args: []any{
			grant.ID, grant.TokenID, grant.UserID,
			grant.Role, grant.GrantedBy, formatTime(grant.CreatedAt),
		},
	})
	if err != nil {
		return fmt.Errorf("insert hook grant: %w", err)
	}
	return nil
}

// ListHookGrants returns all grants for a token.
func (s *Store) ListHookGrants(ctx context.Context, tokenID string) ([]*models.HookGrant, error) {
	conn, err := s.take(ctx)
	if err != nil {
		return nil, err
	}
	defer s.pool.Put(conn)

	var grants []*models.HookGrant
	err = sqlitex.ExecuteTransient(conn, `SELECT
		id, token_id, user_id, role, granted_by, created_at
	FROM hook_grants WHERE token_id = ? ORDER BY created_at ASC`, &sqlitex.ExecOptions{
		Args: []any{tokenID},
		ResultFunc: func(stmt *sqlite.Stmt) error {
			grant, scanErr := scanGrant(stmt)
			if scanErr != nil {
				return scanErr
			}
			grants = append(grants, grant)
			return nil
		},
	})
	if err != nil {
		return nil, fmt.Errorf("list hook grants: %w", err)
	}
	return grants, nil
}

// GetHookGrant retrieves a specific grant for a user on a token.
func (s *Store) GetHookGrant(ctx context.Context, tokenID, userID string) (*models.HookGrant, error) {
	conn, err := s.take(ctx)
	if err != nil {
		return nil, err
	}
	defer s.pool.Put(conn)

	var grant *models.HookGrant
	err = sqlitex.ExecuteTransient(conn, `SELECT
		id, token_id, user_id, role, granted_by, created_at
	FROM hook_grants WHERE token_id = ? AND user_id = ?`, &sqlitex.ExecOptions{
		Args: []any{tokenID, userID},
		ResultFunc: func(stmt *sqlite.Stmt) error {
			var scanErr error
			grant, scanErr = scanGrant(stmt)
			return scanErr
		},
	})
	if err != nil {
		return nil, fmt.Errorf("get hook grant: %w", err)
	}
	if grant == nil {
		return nil, ErrNotFound
	}
	return grant, nil
}

// DeleteHookGrant removes a grant by its ID.
func (s *Store) DeleteHookGrant(ctx context.Context, grantID string) error {
	conn, err := s.take(ctx)
	if err != nil {
		return err
	}
	defer s.pool.Put(conn)

	err = sqlitex.ExecuteTransient(conn, "DELETE FROM hook_grants WHERE id = ?", &sqlitex.ExecOptions{
		Args: []any{grantID},
	})
	if err != nil {
		return fmt.Errorf("delete hook grant: %w", err)
	}
	if conn.Changes() == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteHookGrantByTokenAndUser removes a grant for a specific user on a token.
func (s *Store) DeleteHookGrantByTokenAndUser(ctx context.Context, tokenID, userID string) error {
	conn, err := s.take(ctx)
	if err != nil {
		return err
	}
	defer s.pool.Put(conn)

	err = sqlitex.ExecuteTransient(conn, "DELETE FROM hook_grants WHERE token_id = ? AND user_id = ?", &sqlitex.ExecOptions{
		Args: []any{tokenID, userID},
	})
	if err != nil {
		return fmt.Errorf("delete hook grant by token and user: %w", err)
	}
	if conn.Changes() == 0 {
		return ErrNotFound
	}
	return nil
}

func scanGrant(stmt *sqlite.Stmt) (*models.HookGrant, error) {
	grant := &models.HookGrant{
		ID:        stmt.ColumnText(0),
		TokenID:   stmt.ColumnText(1),
		UserID:    stmt.ColumnText(2),
		Role:      stmt.ColumnText(3),
		GrantedBy: stmt.ColumnText(4),
	}

	var err error
	grant.CreatedAt, err = parseTime(stmt.ColumnText(5))
	if err != nil {
		return nil, fmt.Errorf("parse grant created_at: %w", err)
	}
	return grant, nil
}
