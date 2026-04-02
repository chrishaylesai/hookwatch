package store

import (
	"context"
	"fmt"

	"github.com/chrishaylesai/hookwatch/internal/models"
	"zombiezen.com/go/sqlite"
	"zombiezen.com/go/sqlite/sqlitex"
)

// CreateActionLog inserts a new action log entry.
func (s *Store) CreateActionLog(ctx context.Context, log *models.ActionLog) error {
	conn, err := s.take(ctx)
	if err != nil {
		return err
	}
	defer s.pool.Put(conn)

	query := `INSERT INTO action_logs (uuid, action_id, request_id, status, result, started_at, completed_at)
	VALUES (?, ?, ?, ?, ?, ?, ?)`

	startedAt := ""
	if !log.StartedAt.IsZero() {
		startedAt = formatTime(log.StartedAt)
	}
	completedAt := ""
	if !log.CompletedAt.IsZero() {
		completedAt = formatTime(log.CompletedAt)
	}

	err = sqlitex.ExecuteTransient(conn, query, &sqlitex.ExecOptions{
		Args: []any{
			log.UUID, log.ActionID, log.RequestID, log.Status, log.Result,
			nullableTimeString(startedAt), nullableTimeString(completedAt),
		},
	})
	if err != nil {
		return fmt.Errorf("insert action log: %w", err)
	}

	return nil
}

// UpdateActionLog updates an existing action log entry.
func (s *Store) UpdateActionLog(ctx context.Context, log *models.ActionLog) error {
	conn, err := s.take(ctx)
	if err != nil {
		return err
	}
	defer s.pool.Put(conn)

	startedAt := ""
	if !log.StartedAt.IsZero() {
		startedAt = formatTime(log.StartedAt)
	}
	completedAt := ""
	if !log.CompletedAt.IsZero() {
		completedAt = formatTime(log.CompletedAt)
	}

	query := `UPDATE action_logs SET status = ?, result = ?, started_at = ?, completed_at = ? WHERE uuid = ?`

	err = sqlitex.ExecuteTransient(conn, query, &sqlitex.ExecOptions{
		Args: []any{
			log.Status, log.Result,
			nullableTimeString(startedAt), nullableTimeString(completedAt),
			log.UUID,
		},
	})
	if err != nil {
		return fmt.Errorf("update action log: %w", err)
	}
	if conn.Changes() == 0 {
		return ErrNotFound
	}

	return nil
}

// ListActionLogsByRequest retrieves all action logs for a specific request.
func (s *Store) ListActionLogsByRequest(ctx context.Context, requestID string) ([]*models.ActionLog, error) {
	conn, err := s.take(ctx)
	if err != nil {
		return nil, err
	}
	defer s.pool.Put(conn)

	query := `SELECT uuid, action_id, request_id, status, result, started_at, completed_at
	FROM action_logs WHERE request_id = ? ORDER BY started_at ASC`

	var logs []*models.ActionLog
	err = sqlitex.ExecuteTransient(conn, query, &sqlitex.ExecOptions{
		Args: []any{requestID},
		ResultFunc: func(stmt *sqlite.Stmt) error {
			log, scanErr := scanActionLog(stmt)
			if scanErr != nil {
				return scanErr
			}
			logs = append(logs, log)
			return nil
		},
	})
	if err != nil {
		return nil, fmt.Errorf("list action logs: %w", err)
	}

	return logs, nil
}

func scanActionLog(stmt *sqlite.Stmt) (*models.ActionLog, error) {
	log := &models.ActionLog{
		UUID:      stmt.ColumnText(0),
		ActionID:  stmt.ColumnText(1),
		RequestID: stmt.ColumnText(2),
		Status:    stmt.ColumnText(3),
		Result:    stmt.ColumnText(4),
	}

	if !stmt.ColumnIsNull(5) && stmt.ColumnText(5) != "" {
		t, err := parseTime(stmt.ColumnText(5))
		if err != nil {
			return nil, fmt.Errorf("parse action log started_at: %w", err)
		}
		log.StartedAt = t
	}
	if !stmt.ColumnIsNull(6) && stmt.ColumnText(6) != "" {
		t, err := parseTime(stmt.ColumnText(6))
		if err != nil {
			return nil, fmt.Errorf("parse action log completed_at: %w", err)
		}
		log.CompletedAt = t
	}

	return log, nil
}

func nullableTimeString(s string) any {
	if s == "" {
		return nil
	}
	return s
}
