package store

import (
	"fmt"
	"path/filepath"

	"zombiezen.com/go/sqlite/sqlitex"
)

// Store wraps a SQLite connection pool.
type Store struct {
	pool *sqlitex.Pool
}

// Open creates or opens the SQLite database in the given directory.
func Open(dataDir string) (*Store, error) {
	dbPath := filepath.Join(dataDir, "hookwatch.db")
	pool, err := sqlitex.NewPool(dbPath, sqlitex.PoolOptions{
		PoolSize: 10,
	})
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	s := &Store{pool: pool}
	if err := s.migrate(); err != nil {
		pool.Close()
		return nil, fmt.Errorf("run migrations: %w", err)
	}

	return s, nil
}

// Close closes the database pool.
func (s *Store) Close() error {
	return s.pool.Close()
}

func (s *Store) migrate() error {
	// Migrations will be added in the next task.
	return nil
}
