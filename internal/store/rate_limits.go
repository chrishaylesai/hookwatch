package store

import (
	"context"
	"fmt"
	"time"

	"zombiezen.com/go/sqlite"
	"zombiezen.com/go/sqlite/sqlitex"
)

// IncrementRateLimit atomically increments the rate limit counter for an IP+token+window.
// Returns the new request count for this window.
func (s *Store) IncrementRateLimit(ctx context.Context, ip, tokenID string, window time.Time) (int, error) {
	conn, err := s.take(ctx)
	if err != nil {
		return 0, err
	}
	defer s.pool.Put(conn)

	windowStr := formatTime(window)
	query := `INSERT INTO rate_limits (ip, token_id, window_start, request_count)
	VALUES (?, ?, ?, 1)
	ON CONFLICT (ip, token_id, window_start)
	DO UPDATE SET request_count = request_count + 1`

	err = sqlitex.ExecuteTransient(conn, query, &sqlitex.ExecOptions{
		Args: []any{ip, tokenID, windowStr},
	})
	if err != nil {
		return 0, fmt.Errorf("increment rate limit: %w", err)
	}

	var count int
	err = sqlitex.ExecuteTransient(conn, "SELECT request_count FROM rate_limits WHERE ip = ? AND token_id = ? AND window_start = ?", &sqlitex.ExecOptions{
		Args: []any{ip, tokenID, windowStr},
		ResultFunc: func(stmt *sqlite.Stmt) error {
			count = stmt.ColumnInt(0)
			return nil
		},
	})
	if err != nil {
		return 0, fmt.Errorf("read rate limit count: %w", err)
	}

	return count, nil
}

// CleanupRateLimits removes rate limit entries older than the given time.
func (s *Store) CleanupRateLimits(ctx context.Context, before time.Time) (int, error) {
	conn, err := s.take(ctx)
	if err != nil {
		return 0, err
	}
	defer s.pool.Put(conn)

	err = sqlitex.ExecuteTransient(conn, "DELETE FROM rate_limits WHERE window_start < ?", &sqlitex.ExecOptions{
		Args: []any{formatTime(before)},
	})
	if err != nil {
		return 0, fmt.Errorf("cleanup rate limits: %w", err)
	}

	return conn.Changes(), nil
}
