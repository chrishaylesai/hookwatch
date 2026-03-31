package store

import (
	"context"
	"log/slog"
	"time"

	"zombiezen.com/go/sqlite/sqlitex"
)

// TouchTokenExpiry extends a token's expiry from the provided base time.
func (s *Store) TouchTokenExpiry(ctx context.Context, tokenID string, now time.Time) (time.Time, error) {
	conn, err := s.take(ctx)
	if err != nil {
		return time.Time{}, err
	}
	defer s.pool.Put(conn)

	expiresAt := now.UTC().Add(s.config.TokenTTL)
	err = sqlitex.ExecuteTransient(conn, "UPDATE tokens SET expires_at = ? WHERE uuid = ?", &sqlitex.ExecOptions{
		Args: []any{formatTime(expiresAt), tokenID},
	})
	if err != nil {
		return time.Time{}, err
	}
	if conn.Changes() == 0 {
		return time.Time{}, ErrNotFound
	}

	return expiresAt, nil
}

// DeleteExpiredTokens removes expired tokens and cascades their requests.
func (s *Store) DeleteExpiredTokens(ctx context.Context, now time.Time) (int, error) {
	conn, err := s.take(ctx)
	if err != nil {
		return 0, err
	}
	defer s.pool.Put(conn)

	err = sqlitex.ExecuteTransient(conn, "DELETE FROM tokens WHERE expires_at <= ?", &sqlitex.ExecOptions{
		Args: []any{formatTime(now.UTC())},
	})
	if err != nil {
		return 0, err
	}

	return int(conn.Changes()), nil
}

// RunTokenCleanup periodically removes expired tokens until the context is canceled.
func (s *Store) RunTokenCleanup(ctx context.Context, interval time.Duration, logger *slog.Logger) {
	if interval <= 0 {
		interval = DefaultTokenCleanupInterval
	}
	if logger == nil {
		logger = slog.Default()
	}

	cleanup := func() {
		deleted, err := s.DeleteExpiredTokens(ctx, time.Now())
		if err != nil {
			logger.Error("failed to clean expired tokens", "error", err)
			return
		}
		if deleted > 0 {
			logger.Info("cleaned expired tokens", "deleted", deleted)
		}
	}

	cleanup()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			cleanup()
		}
	}
}
