package store

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/chrishaylesai/hookwatch/internal/models"
	"zombiezen.com/go/sqlite"
	"zombiezen.com/go/sqlite/sqlitex"
)

const (
	defaultPageSize = 50
	maxPageSize     = 100
	timestampLayout = time.RFC3339
)

var ErrNotFound = errors.New("store: not found")

// TokenListParams controls token pagination and ordering.
type TokenListParams struct {
	Limit  int
	Offset int
	SortBy string
	Order  string
}

// TokenPage is a paginated token result set.
type TokenPage struct {
	Tokens []*models.Token
	Total  int
	Limit  int
	Offset int
}

// RequestListParams controls request pagination and ordering.
type RequestListParams struct {
	Limit  int
	Offset int
	SortBy string
	Order  string
}

// RequestPage is a paginated request result set.
type RequestPage struct {
	Requests []*models.Request
	Total    int
	Limit    int
	Offset   int
}

// Store wraps a SQLite connection pool.
type Store struct {
	pool *sqlitex.Pool
}

// Open creates or opens the SQLite database in the given directory.
func Open(dataDir string) (*Store, error) {
	dbPath := filepath.Join(dataDir, "hookwatch.db")
	pool, err := sqlitex.NewPool(dbPath, sqlitex.PoolOptions{
		PoolSize:    10,
		PrepareConn: prepareConn,
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

func prepareConn(conn *sqlite.Conn) error {
	return sqlitex.ExecuteTransient(conn, `PRAGMA foreign_keys = ON;`, nil)
}

// Close closes the database pool.
func (s *Store) Close() error {
	return s.pool.Close()
}

func (s *Store) take(ctx context.Context) (*sqlite.Conn, error) {
	conn, err := s.pool.Take(ctx)
	if err != nil {
		return nil, fmt.Errorf("get sqlite connection: %w", err)
	}
	return conn, nil
}

func (s *Store) migrate() error {
	conn, err := s.take(context.Background())
	if err != nil {
		return fmt.Errorf("failed to get connection for migration: %w", err)
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
		`CREATE INDEX IF NOT EXISTS idx_requests_token_created ON requests(token_id, created_at DESC)`,
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
	conn, err := s.take(ctx)
	if err != nil {
		return err
	}
	defer s.pool.Put(conn)

	query := `INSERT INTO tokens (
		uuid, owner_id, receive_mode, view_mode, receive_secret_hash, receive_secret_prefix,
		default_status, default_content, default_content_type, timeout, cors,
		created_at, updated_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	err = sqlitex.ExecuteTransient(conn, query, &sqlitex.ExecOptions{
		Args: []any{
			token.UUID, nullableString(token.OwnerID), token.ReceiveMode, token.ViewMode,
			nullableString(token.ReceiveSecretHash), nullableString(token.ReceiveSecretPrefix),
			token.DefaultStatus, token.DefaultContent, token.DefaultContentType,
			token.Timeout, boolToSQLite(token.CORS), formatTime(token.CreatedAt), formatTime(token.UpdatedAt),
		},
	})
	if err != nil {
		return fmt.Errorf("insert token: %w", err)
	}

	return nil
}

// GetToken retrieves a token by its UUID.
func (s *Store) GetToken(ctx context.Context, uuid string) (*models.Token, error) {
	conn, err := s.take(ctx)
	if err != nil {
		return nil, err
	}
	defer s.pool.Put(conn)

	query := `SELECT
		uuid, owner_id, receive_mode, view_mode, receive_secret_hash, receive_secret_prefix,
		default_status, default_content, default_content_type, timeout, cors,
		created_at, updated_at
	FROM tokens WHERE uuid = ?`

	var token *models.Token
	err = sqlitex.ExecuteTransient(conn, query, &sqlitex.ExecOptions{
		Args: []any{uuid},
		ResultFunc: func(stmt *sqlite.Stmt) error {
			var scanErr error
			token, scanErr = scanToken(stmt)
			return scanErr
		},
	})
	if err != nil {
		return nil, fmt.Errorf("get token: %w", err)
	}
	if token == nil {
		return nil, ErrNotFound
	}

	return token, nil
}

// ListTokens retrieves a paginated list of tokens.
func (s *Store) ListTokens(ctx context.Context, params TokenListParams) (*TokenPage, error) {
	conn, err := s.take(ctx)
	if err != nil {
		return nil, err
	}
	defer s.pool.Put(conn)

	params = normalizeTokenListParams(params)
	total, err := queryCount(conn, "SELECT COUNT(*) FROM tokens", nil)
	if err != nil {
		return nil, fmt.Errorf("count tokens: %w", err)
	}

	query := fmt.Sprintf(`SELECT
		uuid, owner_id, receive_mode, view_mode, receive_secret_hash, receive_secret_prefix,
		default_status, default_content, default_content_type, timeout, cors,
		created_at, updated_at
	FROM tokens
	ORDER BY %s %s
	LIMIT ? OFFSET ?`, tokenSortColumn(params.SortBy), sortDirection(params.Order))

	var tokens []*models.Token
	err = sqlitex.ExecuteTransient(conn, query, &sqlitex.ExecOptions{
		Args: []any{params.Limit, params.Offset},
		ResultFunc: func(stmt *sqlite.Stmt) error {
			token, scanErr := scanToken(stmt)
			if scanErr != nil {
				return scanErr
			}
			tokens = append(tokens, token)
			return nil
		},
	})
	if err != nil {
		return nil, fmt.Errorf("list tokens: %w", err)
	}

	return &TokenPage{
		Tokens: tokens,
		Total:  total,
		Limit:  params.Limit,
		Offset: params.Offset,
	}, nil
}

// UpdateToken updates an existing token.
func (s *Store) UpdateToken(ctx context.Context, token *models.Token) error {
	conn, err := s.take(ctx)
	if err != nil {
		return err
	}
	defer s.pool.Put(conn)

	query := `UPDATE tokens SET
		receive_mode = ?, view_mode = ?, receive_secret_hash = ?, receive_secret_prefix = ?,
		default_status = ?, default_content = ?, default_content_type = ?, timeout = ?, cors = ?,
		updated_at = ?
	WHERE uuid = ?`

	err = sqlitex.ExecuteTransient(conn, query, &sqlitex.ExecOptions{
		Args: []any{
			token.ReceiveMode, token.ViewMode, nullableString(token.ReceiveSecretHash), nullableString(token.ReceiveSecretPrefix),
			token.DefaultStatus, token.DefaultContent, token.DefaultContentType,
			token.Timeout, boolToSQLite(token.CORS), formatTime(token.UpdatedAt), token.UUID,
		},
	})
	if err != nil {
		return fmt.Errorf("update token: %w", err)
	}
	if conn.Changes() == 0 {
		return ErrNotFound
	}

	return nil
}

// DeleteToken removes a token.
func (s *Store) DeleteToken(ctx context.Context, uuid string) error {
	conn, err := s.take(ctx)
	if err != nil {
		return err
	}
	defer s.pool.Put(conn)

	err = sqlitex.ExecuteTransient(conn, "DELETE FROM tokens WHERE uuid = ?", &sqlitex.ExecOptions{
		Args: []any{uuid},
	})
	if err != nil {
		return fmt.Errorf("delete token: %w", err)
	}
	if conn.Changes() == 0 {
		return ErrNotFound
	}

	return nil
}

// CreateRequest inserts a new captured request into the database.
func (s *Store) CreateRequest(ctx context.Context, req *models.Request) error {
	conn, err := s.take(ctx)
	if err != nil {
		return err
	}
	defer s.pool.Put(conn)

	query := `INSERT INTO requests (
		uuid, token_id, ip, hostname, method, user_agent, content, query, headers, form_data, url, created_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	err = sqlitex.ExecuteTransient(conn, query, &sqlitex.ExecOptions{
		Args: []any{
			req.UUID, req.TokenID, req.IP, req.Hostname, req.Method, req.UserAgent,
			req.Content, req.Query, req.Headers, req.FormData, req.URL, formatTime(req.CreatedAt),
		},
	})
	if err != nil {
		return fmt.Errorf("insert request: %w", err)
	}

	return nil
}

// GetRequest retrieves a single captured request scoped to a token.
func (s *Store) GetRequest(ctx context.Context, tokenID, requestID string) (*models.Request, error) {
	conn, err := s.take(ctx)
	if err != nil {
		return nil, err
	}
	defer s.pool.Put(conn)

	query := `SELECT
		uuid, token_id, ip, hostname, method, user_agent, content, query, headers, form_data, url, created_at
	FROM requests
	WHERE token_id = ? AND uuid = ?`

	var req *models.Request
	err = sqlitex.ExecuteTransient(conn, query, &sqlitex.ExecOptions{
		Args: []any{tokenID, requestID},
		ResultFunc: func(stmt *sqlite.Stmt) error {
			var scanErr error
			req, scanErr = scanRequest(stmt)
			return scanErr
		},
	})
	if err != nil {
		return nil, fmt.Errorf("get request: %w", err)
	}
	if req == nil {
		return nil, ErrNotFound
	}

	return req, nil
}

// ListRequestsByToken retrieves a paginated list of requests for a token.
func (s *Store) ListRequestsByToken(ctx context.Context, tokenID string, params RequestListParams) (*RequestPage, error) {
	conn, err := s.take(ctx)
	if err != nil {
		return nil, err
	}
	defer s.pool.Put(conn)

	params = normalizeRequestListParams(params)
	total, err := queryCount(conn, "SELECT COUNT(*) FROM requests WHERE token_id = ?", []any{tokenID})
	if err != nil {
		return nil, fmt.Errorf("count requests: %w", err)
	}

	query := fmt.Sprintf(`SELECT
		uuid, token_id, ip, hostname, method, user_agent, content, query, headers, form_data, url, created_at
	FROM requests
	WHERE token_id = ?
	ORDER BY %s %s
	LIMIT ? OFFSET ?`, requestSortColumn(params.SortBy), sortDirection(params.Order))

	var requests []*models.Request
	err = sqlitex.ExecuteTransient(conn, query, &sqlitex.ExecOptions{
		Args: []any{tokenID, params.Limit, params.Offset},
		ResultFunc: func(stmt *sqlite.Stmt) error {
			req, scanErr := scanRequest(stmt)
			if scanErr != nil {
				return scanErr
			}
			requests = append(requests, req)
			return nil
		},
	})
	if err != nil {
		return nil, fmt.Errorf("list requests: %w", err)
	}

	return &RequestPage{
		Requests: requests,
		Total:    total,
		Limit:    params.Limit,
		Offset:   params.Offset,
	}, nil
}

// DeleteRequest removes a single request scoped to a token.
func (s *Store) DeleteRequest(ctx context.Context, tokenID, requestID string) error {
	conn, err := s.take(ctx)
	if err != nil {
		return err
	}
	defer s.pool.Put(conn)

	err = sqlitex.ExecuteTransient(conn, "DELETE FROM requests WHERE token_id = ? AND uuid = ?", &sqlitex.ExecOptions{
		Args: []any{tokenID, requestID},
	})
	if err != nil {
		return fmt.Errorf("delete request: %w", err)
	}
	if conn.Changes() == 0 {
		return ErrNotFound
	}

	return nil
}

// DeleteAllRequestsByToken removes all requests for a token.
func (s *Store) DeleteAllRequestsByToken(ctx context.Context, tokenID string) error {
	conn, err := s.take(ctx)
	if err != nil {
		return err
	}
	defer s.pool.Put(conn)

	exists, err := tokenExists(conn, tokenID)
	if err != nil {
		return fmt.Errorf("check token exists: %w", err)
	}
	if !exists {
		return ErrNotFound
	}

	err = sqlitex.ExecuteTransient(conn, "DELETE FROM requests WHERE token_id = ?", &sqlitex.ExecOptions{
		Args: []any{tokenID},
	})
	if err != nil {
		return fmt.Errorf("delete all requests: %w", err)
	}

	return nil
}

func queryCount(conn *sqlite.Conn, query string, args []any) (int, error) {
	var total int
	err := sqlitex.ExecuteTransient(conn, query, &sqlitex.ExecOptions{
		Args: args,
		ResultFunc: func(stmt *sqlite.Stmt) error {
			total = stmt.ColumnInt(0)
			return nil
		},
	})
	if err != nil {
		return 0, err
	}
	return total, nil
}

func tokenExists(conn *sqlite.Conn, tokenID string) (bool, error) {
	var exists bool
	err := sqlitex.ExecuteTransient(conn, "SELECT 1 FROM tokens WHERE uuid = ? LIMIT 1", &sqlitex.ExecOptions{
		Args: []any{tokenID},
		ResultFunc: func(stmt *sqlite.Stmt) error {
			exists = stmt.ColumnInt(0) == 1
			return nil
		},
	})
	if err != nil {
		return false, err
	}
	return exists, nil
}

func scanToken(stmt *sqlite.Stmt) (*models.Token, error) {
	token := &models.Token{
		UUID:               stmt.ColumnText(0),
		ReceiveMode:        stmt.ColumnText(2),
		ViewMode:           stmt.ColumnText(3),
		DefaultStatus:      stmt.ColumnInt(6),
		DefaultContent:     stmt.ColumnText(7),
		DefaultContentType: stmt.ColumnText(8),
		Timeout:            stmt.ColumnInt(9),
		CORS:               stmt.ColumnInt(10) != 0,
	}

	if !stmt.ColumnIsNull(1) {
		ownerID := stmt.ColumnText(1)
		token.OwnerID = &ownerID
	}
	if !stmt.ColumnIsNull(4) {
		hash := stmt.ColumnText(4)
		token.ReceiveSecretHash = &hash
	}
	if !stmt.ColumnIsNull(5) {
		prefix := stmt.ColumnText(5)
		token.ReceiveSecretPrefix = &prefix
	}

	var err error
	token.CreatedAt, err = parseTime(stmt.ColumnText(11))
	if err != nil {
		return nil, fmt.Errorf("parse token created_at: %w", err)
	}
	token.UpdatedAt, err = parseTime(stmt.ColumnText(12))
	if err != nil {
		return nil, fmt.Errorf("parse token updated_at: %w", err)
	}

	return token, nil
}

func scanRequest(stmt *sqlite.Stmt) (*models.Request, error) {
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

	var err error
	req.CreatedAt, err = parseTime(stmt.ColumnText(11))
	if err != nil {
		return nil, fmt.Errorf("parse request created_at: %w", err)
	}

	return req, nil
}

func parseTime(value string) (time.Time, error) {
	return time.Parse(timestampLayout, value)
}

func formatTime(value time.Time) string {
	return value.UTC().Format(timestampLayout)
}

func boolToSQLite(value bool) int {
	if value {
		return 1
	}
	return 0
}

func nullableString(value *string) any {
	if value == nil {
		return nil
	}
	return *value
}

func normalizeTokenListParams(params TokenListParams) TokenListParams {
	params.Limit = normalizeLimit(params.Limit)
	params.Offset = normalizeOffset(params.Offset)
	if params.SortBy == "" {
		params.SortBy = "created_at"
	}
	if params.Order == "" {
		params.Order = "desc"
	}
	return params
}

func normalizeRequestListParams(params RequestListParams) RequestListParams {
	params.Limit = normalizeLimit(params.Limit)
	params.Offset = normalizeOffset(params.Offset)
	if params.SortBy == "" {
		params.SortBy = "created_at"
	}
	if params.Order == "" {
		params.Order = "desc"
	}
	return params
}

func normalizeLimit(limit int) int {
	switch {
	case limit <= 0:
		return defaultPageSize
	case limit > maxPageSize:
		return maxPageSize
	default:
		return limit
	}
}

func normalizeOffset(offset int) int {
	if offset < 0 {
		return 0
	}
	return offset
}

func tokenSortColumn(sortBy string) string {
	switch strings.ToLower(sortBy) {
	case "updated_at":
		return "updated_at"
	default:
		return "created_at"
	}
}

func requestSortColumn(sortBy string) string {
	switch strings.ToLower(sortBy) {
	case "created_at":
		return "created_at"
	default:
		return "created_at"
	}
}

func sortDirection(order string) string {
	if strings.EqualFold(order, "asc") {
		return "ASC"
	}
	return "DESC"
}
