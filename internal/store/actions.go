package store

import (
	"context"
	"fmt"

	"github.com/chrishaylesai/hookwatch/internal/models"
	"zombiezen.com/go/sqlite"
	"zombiezen.com/go/sqlite/sqlitex"
)

// CreateAction inserts a new action into the database.
func (s *Store) CreateAction(ctx context.Context, action *models.Action) error {
	conn, err := s.take(ctx)
	if err != nil {
		return err
	}
	defer s.pool.Put(conn)

	query := `INSERT INTO actions (uuid, token_id, type, config, sort_order, enabled, created_at, updated_at)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?)`

	err = sqlitex.ExecuteTransient(conn, query, &sqlitex.ExecOptions{
		Args: []any{
			action.UUID, action.TokenID, action.Type, action.Config,
			action.SortOrder, boolToSQLite(action.Enabled),
			formatTime(action.CreatedAt), formatTime(action.UpdatedAt),
		},
	})
	if err != nil {
		return fmt.Errorf("insert action: %w", err)
	}

	return nil
}

// GetAction retrieves an action by token ID and action ID.
func (s *Store) GetAction(ctx context.Context, tokenID, actionID string) (*models.Action, error) {
	conn, err := s.take(ctx)
	if err != nil {
		return nil, err
	}
	defer s.pool.Put(conn)

	query := `SELECT uuid, token_id, type, config, sort_order, enabled, created_at, updated_at
	FROM actions WHERE token_id = ? AND uuid = ?`

	var action *models.Action
	err = sqlitex.ExecuteTransient(conn, query, &sqlitex.ExecOptions{
		Args: []any{tokenID, actionID},
		ResultFunc: func(stmt *sqlite.Stmt) error {
			var scanErr error
			action, scanErr = scanAction(stmt)
			return scanErr
		},
	})
	if err != nil {
		return nil, fmt.Errorf("get action: %w", err)
	}
	if action == nil {
		return nil, ErrNotFound
	}

	return action, nil
}

// ListActionsByToken retrieves all actions for a token ordered by sort_order.
func (s *Store) ListActionsByToken(ctx context.Context, tokenID string) ([]*models.Action, error) {
	conn, err := s.take(ctx)
	if err != nil {
		return nil, err
	}
	defer s.pool.Put(conn)

	query := `SELECT uuid, token_id, type, config, sort_order, enabled, created_at, updated_at
	FROM actions WHERE token_id = ? ORDER BY sort_order ASC`

	var actions []*models.Action
	err = sqlitex.ExecuteTransient(conn, query, &sqlitex.ExecOptions{
		Args: []any{tokenID},
		ResultFunc: func(stmt *sqlite.Stmt) error {
			action, scanErr := scanAction(stmt)
			if scanErr != nil {
				return scanErr
			}
			actions = append(actions, action)
			return nil
		},
	})
	if err != nil {
		return nil, fmt.Errorf("list actions: %w", err)
	}

	return actions, nil
}

// UpdateAction updates an existing action.
func (s *Store) UpdateAction(ctx context.Context, action *models.Action) error {
	conn, err := s.take(ctx)
	if err != nil {
		return err
	}
	defer s.pool.Put(conn)

	query := `UPDATE actions SET type = ?, config = ?, sort_order = ?, enabled = ?, updated_at = ?
	WHERE token_id = ? AND uuid = ?`

	err = sqlitex.ExecuteTransient(conn, query, &sqlitex.ExecOptions{
		Args: []any{
			action.Type, action.Config, action.SortOrder, boolToSQLite(action.Enabled),
			formatTime(action.UpdatedAt), action.TokenID, action.UUID,
		},
	})
	if err != nil {
		return fmt.Errorf("update action: %w", err)
	}
	if conn.Changes() == 0 {
		return ErrNotFound
	}

	return nil
}

// DeleteAction removes an action.
func (s *Store) DeleteAction(ctx context.Context, tokenID, actionID string) error {
	conn, err := s.take(ctx)
	if err != nil {
		return err
	}
	defer s.pool.Put(conn)

	err = sqlitex.ExecuteTransient(conn, "DELETE FROM actions WHERE token_id = ? AND uuid = ?", &sqlitex.ExecOptions{
		Args: []any{tokenID, actionID},
	})
	if err != nil {
		return fmt.Errorf("delete action: %w", err)
	}
	if conn.Changes() == 0 {
		return ErrNotFound
	}

	return nil
}

// ReorderActions updates the sort_order of actions based on the provided ID order.
func (s *Store) ReorderActions(ctx context.Context, tokenID string, actionIDs []string) error {
	conn, err := s.take(ctx)
	if err != nil {
		return err
	}
	defer s.pool.Put(conn)

	endFn, err := sqlitex.ImmediateTransaction(conn)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer endFn(&err)

	for i, id := range actionIDs {
		err = sqlitex.ExecuteTransient(conn, "UPDATE actions SET sort_order = ? WHERE token_id = ? AND uuid = ?", &sqlitex.ExecOptions{
			Args: []any{i, tokenID, id},
		})
		if err != nil {
			return fmt.Errorf("reorder action %s: %w", id, err)
		}
		if conn.Changes() == 0 {
			return fmt.Errorf("action %s not found for token %s", id, tokenID)
		}
	}

	return nil
}

// NextActionSortOrder returns the next sort_order value for a new action in a token.
func (s *Store) NextActionSortOrder(ctx context.Context, tokenID string) (int, error) {
	conn, err := s.take(ctx)
	if err != nil {
		return 0, err
	}
	defer s.pool.Put(conn)

	var maxOrder int
	err = sqlitex.ExecuteTransient(conn, "SELECT COALESCE(MAX(sort_order), -1) FROM actions WHERE token_id = ?", &sqlitex.ExecOptions{
		Args: []any{tokenID},
		ResultFunc: func(stmt *sqlite.Stmt) error {
			maxOrder = stmt.ColumnInt(0)
			return nil
		},
	})
	if err != nil {
		return 0, fmt.Errorf("get max sort order: %w", err)
	}

	return maxOrder + 1, nil
}

func scanAction(stmt *sqlite.Stmt) (*models.Action, error) {
	action := &models.Action{
		UUID:      stmt.ColumnText(0),
		TokenID:   stmt.ColumnText(1),
		Type:      stmt.ColumnText(2),
		Config:    stmt.ColumnText(3),
		SortOrder: stmt.ColumnInt(4),
		Enabled:   stmt.ColumnInt(5) != 0,
	}

	var err error
	action.CreatedAt, err = parseTime(stmt.ColumnText(6))
	if err != nil {
		return nil, fmt.Errorf("parse action created_at: %w", err)
	}
	action.UpdatedAt, err = parseTime(stmt.ColumnText(7))
	if err != nil {
		return nil, fmt.Errorf("parse action updated_at: %w", err)
	}

	return action, nil
}
