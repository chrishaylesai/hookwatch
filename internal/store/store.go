package store

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/chrishaylesai/hookwatch/internal/models"
	"zombiezen.com/go/sqlite"
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
	conn := s.pool.Get(context.Background())
	if conn == nil {
		return fmt.Errorf("failed to get connection for migration")
	}
	defer s.pool.Put(conn)

	queries := []string{
		`CREATE TABLE IF NOT EXISTS users (
			id TEXT PRIMARY KEY,
			email TEXT NOT NULL UNIQUE,
			display_name TEXT NOT NULL DEFAULT '',
			password_hash TEXT,
			oidc_provider TEXT,
			oidc_subject TEXT,
			global_role TEXT NOT NULL DEFAULT 'user',
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL,
			UNIQUE(oidc_provider, oidc_subject)
		)`,
		`CREATE TABLE IF NOT EXISTS sessions (
			id TEXT PRIMARY KEY,
			user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			created_at DATETIME NOT NULL,
			expires_at DATETIME NOT NULL,
			ip TEXT,
			user_agent TEXT
		)`,
		`CREATE INDEX IF NOT EXISTS idx_sessions_user ON sessions(user_id)`,
		`CREATE INDEX IF NOT EXISTS idx_sessions_expires ON sessions(expires_at)`,
		`CREATE TABLE IF NOT EXISTS tokens (
			uuid TEXT PRIMARY KEY,
			owner_id TEXT REFERENCES users(id) ON DELETE SET NULL,
			receive_mode TEXT NOT NULL DEFAULT 'public',
			view_mode TEXT NOT NULL DEFAULT 'public',
			receive_secret_hash TEXT,
			receive_secret_prefix TEXT,
			default_status INTEGER NOT NULL DEFAULT 200,
			default_content TEXT NOT NULL DEFAULT '',
			default_content_type TEXT NOT NULL DEFAULT 'text/plain',
			timeout INTEGER NOT NULL DEFAULT 0,
			cors INTEGER NOT NULL DEFAULT 0,
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_tokens_owner ON tokens(owner_id)`,
		`CREATE TABLE IF NOT EXISTS requests (
			uuid TEXT PRIMARY KEY,
			token_id TEXT NOT NULL REFERENCES tokens(uuid) ON DELETE CASCADE,
			ip TEXT NOT NULL,
			hostname TEXT NOT NULL,
			method TEXT NOT NULL,
			user_agent TEXT NOT NULL,
			content TEXT NOT NULL,
			query TEXT NOT NULL,
			headers TEXT NOT NULL,
			form_data TEXT NOT NULL,
			url TEXT NOT NULL,
			created_at DATETIME NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_requests_token ON requests(token_id)`,
		`CREATE INDEX IF NOT EXISTS idx_requests_created ON requests(created_at)`,
		`CREATE TABLE IF NOT EXISTS hook_grants (
			id TEXT PRIMARY KEY,
			token_id TEXT NOT NULL REFERENCES tokens(uuid) ON DELETE CASCADE,
			user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			role TEXT NOT NULL DEFAULT 'viewer',
			granted_by TEXT NOT NULL REFERENCES users(id),
			created_at DATETIME NOT NULL,
			UNIQUE(token_id, user_id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_grants_token ON hook_grants(token_id)`,
		`CREATE INDEX IF NOT EXISTS idx_grants_user ON hook_grants(user_id)`,
	}

	for _, q := range queries {
		if err := sqlitex.ExecuteTransient(conn, q, nil); err != nil {
			return fmt.Errorf("execute migration (%s): %w", q, err)
		}
	}

	return nil
}

// CreateToken inserts a new token into the database.
func (s *Store) CreateToken(ctx context.Context, token *models.Token) error {
	conn := s.pool.Get(ctx)
	if conn == nil {
		return fmt.Errorf("failed to get connection")
	}
	defer s.pool.Put(conn)

	query := `INSERT INTO tokens (
		uuid, owner_id, receive_mode, view_mode, receive_secret_hash, receive_secret_prefix,
		default_status, default_content, default_content_type, timeout, cors,
		created_at, updated_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	err := sqlitex.ExecuteTransient(conn, query, &sqlitex.ExecOptions{
		Args: []any{
			token.UUID, token.OwnerID, token.ReceiveMode, token.ViewMode,
			token.ReceiveSecretHash, token.ReceiveSecretPrefix,
			token.DefaultStatus, token.DefaultContent, token.DefaultContentType,
			token.Timeout, token.CORS, token.CreatedAt.Format(time.RFC3339), token.UpdatedAt.Format(time.RFC3339),
		},
	})
	if err != nil {
		return fmt.Errorf("insert token: %w", err)
	}

	return nil
}

// GetToken retrieves a token by its UUID.
func (s *Store) GetToken(ctx context.Context, uuid string) (*models.Token, error) {
	conn := s.pool.Get(ctx)
	if conn == nil {
		return nil, fmt.Errorf("failed to get connection")
	}
	defer s.pool.Put(conn)

	query := `SELECT 
		uuid, owner_id, receive_mode, view_mode, receive_secret_hash, receive_secret_prefix,
		default_status, default_content, default_content_type, timeout, cors,
		created_at, updated_at
	FROM tokens WHERE uuid = ?`

	var token models.Token
	var createdAt, updatedAt string
	var found bool
	err := sqlitex.ExecuteTransient(conn, query, &sqlitex.ExecOptions{
		Args: []any{uuid},
		ResultFunc: func(stmt *sqlite.Stmt) error {
			found = true
			token.UUID = stmt.ColumnText(0)
			if !stmt.ColumnIsNull(1) {
				ownerID := stmt.ColumnText(1)
				token.OwnerID = &ownerID
			}
			token.ReceiveMode = stmt.ColumnText(2)
			token.ViewMode = stmt.ColumnText(3)
			if !stmt.ColumnIsNull(4) {
				hash := stmt.ColumnText(4)
				token.ReceiveSecretHash = &hash
			}
			if !stmt.ColumnIsNull(5) {
				prefix := stmt.ColumnText(5)
				token.ReceiveSecretPrefix = &prefix
			}
			token.DefaultStatus = stmt.ColumnInt(6)
			token.DefaultContent = stmt.ColumnText(7)
			token.DefaultContentType = stmt.ColumnText(8)
			token.Timeout = stmt.ColumnInt(9)
			token.CORS = stmt.ColumnInt(10) != 0
			createdAt = stmt.ColumnText(11)
			updatedAt = stmt.ColumnText(12)
			return nil
		},
	})
	if err != nil {
		return nil, fmt.Errorf("get token: %w", err)
	}

	if !found {
		return nil, nil // Not found
	}

	token.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	token.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)

	return &token, nil
}

// UpdateToken updates an existing token.
func (s *Store) UpdateToken(ctx context.Context, token *models.Token) error {
	conn := s.pool.Get(ctx)
	if conn == nil {
		return fmt.Errorf("failed to get connection")
	}
	defer s.pool.Put(conn)

	query := `UPDATE tokens SET 
		receive_mode = ?, view_mode = ?, receive_secret_hash = ?, receive_secret_prefix = ?,
		default_status = ?, default_content = ?, default_content_type = ?, timeout = ?, cors = ?,
		updated_at = ?
	WHERE uuid = ?`

	err := sqlitex.ExecuteTransient(conn, query, &sqlitex.ExecOptions{
		Args: []any{
			token.ReceiveMode, token.ViewMode, token.ReceiveSecretHash, token.ReceiveSecretPrefix,
			token.DefaultStatus, token.DefaultContent, token.DefaultContentType,
			token.Timeout, token.CORS, token.UpdatedAt.Format(time.RFC3339), token.UUID,
		},
	})
	if err != nil {
		return fmt.Errorf("update token: %w", err)
	}

	return nil
}

// CreateRequest inserts a new captured request into the database.
func (s *Store) CreateRequest(ctx context.Context, req *models.Request) error {
	conn := s.pool.Get(ctx)
	if conn == nil {
		return fmt.Errorf("failed to get connection")
	}
	defer s.pool.Put(conn)

	query := `INSERT INTO requests (
		uuid, token_id, ip, hostname, method, user_agent, content, query, headers, form_data, url, created_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	err := sqlitex.ExecuteTransient(conn, query, &sqlitex.ExecOptions{
		Args: []any{
			req.UUID, req.TokenID, req.IP, req.Hostname, req.Method, req.UserAgent,
			req.Content, req.Query, req.Headers, req.FormData, req.URL, req.CreatedAt.Format(time.RFC3339),
		},
	})
	if err != nil {
		return fmt.Errorf("insert request: %w", err)
	}

	return nil
}

// GetRequestsByToken retrieves a paginated list of requests for a token, sorted by creation date descending.
func (s *Store) GetRequestsByToken(ctx context.Context, tokenID string, limit, offset int) ([]*models.Request, error) {
	conn := s.pool.Get(ctx)
	if conn == nil {
		return nil, fmt.Errorf("failed to get connection")
	}
	defer s.pool.Put(conn)

	query := `SELECT 
		uuid, token_id, ip, hostname, method, user_agent, content, query, headers, form_data, url, created_at
	FROM requests 
	WHERE token_id = ? 
	ORDER BY created_at DESC 
	LIMIT ? OFFSET ?`

	var requests []*models.Request
	err := sqlitex.ExecuteTransient(conn, query, &sqlitex.ExecOptions{
		Args: []any{tokenID, limit, offset},
		ResultFunc: func(stmt *sqlite.Stmt) error {
			req := &models.Request{
				UUID:      stmt.ColumnText(0),
				TokenID:   stmt.ColumnText(1),
				IP:        stmt.ColumnText(2),
				Hostname:  stmt.ColumnText(3),
				Method:    stmt.ColumnText(4),
				UserAgent: stmt.ColumnText(5),
				Content:   stmt.ColumnText(6),
				Query:     stmt.ColumnText(7),
				Headers:   stmt.ColumnText(8),
				FormData:  stmt.ColumnText(9),
				URL:       stmt.ColumnText(10),
			}
			createdAtStr := stmt.ColumnText(11)
			req.CreatedAt, _ = time.Parse(time.RFC3339, createdAtStr)
			requests = append(requests, req)
			return nil
		},
	})
	if err != nil {
		return nil, fmt.Errorf("get requests: %w", err)
	}

	return requests, nil
}

// DeleteRequest removes a single request.
func (s *Store) DeleteRequest(ctx context.Context, uuid string) error {
	conn := s.pool.Get(ctx)
	if conn == nil {
		return fmt.Errorf("failed to get connection")
	}
	defer s.pool.Put(conn)

	err := sqlitex.ExecuteTransient(conn, "DELETE FROM requests WHERE uuid = ?", &sqlitex.ExecOptions{
		Args: []any{uuid},
	})
	if err != nil {
		return fmt.Errorf("delete request: %w", err)
	}

	return nil
}

// DeleteAllRequestsByToken removes all requests for a token.
func (s *Store) DeleteAllRequestsByToken(ctx context.Context, tokenID string) error {
	conn := s.pool.Get(ctx)
	if conn == nil {
		return fmt.Errorf("failed to get connection")
	}
	defer s.pool.Put(conn)

	err := sqlitex.ExecuteTransient(conn, "DELETE FROM requests WHERE token_id = ?", &sqlitex.ExecOptions{
		Args: []any{tokenID},
	})
	if err != nil {
		return fmt.Errorf("delete all requests: %w", err)
	}

	return nil
}
