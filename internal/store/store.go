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
var ErrQuotaExceeded = errors.New("store: request quota exceeded")

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
	Method string
	IP     string
	Search string
	Since  time.Time
	Until  time.Time
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
	pool   *sqlitex.Pool
	config Config
}

// Open creates or opens the SQLite database in the given directory.
func Open(dataDir string, cfg Config) (*Store, error) {
	dbPath := filepath.Join(dataDir, "hookwatch.db")
	pool, err := sqlitex.NewPool(dbPath, sqlitex.PoolOptions{
		PoolSize:    10,
		PrepareConn: prepareConn,
	})
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	s := &Store{
		pool:   pool,
		config: normalizeConfig(cfg),
	}
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
			max_requests INTEGER NOT NULL DEFAULT 500,
			timeout INTEGER NOT NULL DEFAULT 0,
			cors INTEGER NOT NULL DEFAULT 0,
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL,
			expires_at DATETIME
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
		`CREATE TABLE IF NOT EXISTS actions (
			uuid TEXT PRIMARY KEY,
			token_id TEXT NOT NULL REFERENCES tokens(uuid) ON DELETE CASCADE,
			type TEXT NOT NULL,
			config TEXT NOT NULL DEFAULT '{}',
			sort_order INTEGER NOT NULL DEFAULT 0,
			enabled INTEGER NOT NULL DEFAULT 1,
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_actions_token_order ON actions(token_id, sort_order)`,
		`CREATE TABLE IF NOT EXISTS action_logs (
			uuid TEXT PRIMARY KEY,
			action_id TEXT NOT NULL REFERENCES actions(uuid) ON DELETE CASCADE,
			request_id TEXT NOT NULL REFERENCES requests(uuid) ON DELETE CASCADE,
			status TEXT NOT NULL DEFAULT 'pending',
			result TEXT NOT NULL DEFAULT '{}',
			started_at DATETIME,
			completed_at DATETIME
		)`,
		`CREATE INDEX IF NOT EXISTS idx_action_logs_request ON action_logs(request_id)`,
		`CREATE INDEX IF NOT EXISTS idx_action_logs_action ON action_logs(action_id)`,
		`CREATE TABLE IF NOT EXISTS rate_limits (
			ip TEXT NOT NULL,
			token_id TEXT NOT NULL,
			window_start DATETIME NOT NULL,
			request_count INTEGER NOT NULL DEFAULT 1,
			PRIMARY KEY (ip, token_id, window_start)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_rate_limits_window ON rate_limits(window_start)`,
	}

	for _, q := range queries {
		if err := sqlitex.ExecuteTransient(conn, q, nil); err != nil {
			return fmt.Errorf("execute migration (%s): %w", q, err)
		}
	}

	if err := ensureTokenExpiryColumn(conn); err != nil {
		return fmt.Errorf("ensure token expiry column: %w", err)
	}
	if err := s.backfillTokenExpiry(conn); err != nil {
		return fmt.Errorf("backfill token expiry: %w", err)
	}
	if err := ensureTokenMaxRequestsColumn(conn); err != nil {
		return fmt.Errorf("ensure token max_requests column: %w", err)
	}
	if err := s.backfillTokenMaxRequests(conn); err != nil {
		return fmt.Errorf("backfill token max_requests: %w", err)
	}
	if err := sqlitex.ExecuteTransient(conn, `CREATE INDEX IF NOT EXISTS idx_tokens_expires_at ON tokens(expires_at)`, nil); err != nil {
		return fmt.Errorf("create token expiry index: %w", err)
	}
	if err := ensureRequestSizeColumn(conn); err != nil {
		return fmt.Errorf("ensure request size column: %w", err)
	}
	if err := backfillRequestSize(conn); err != nil {
		return fmt.Errorf("backfill request size: %w", err)
	}
	if err := ensureTokenRateLimitColumn(conn); err != nil {
		return fmt.Errorf("ensure token rate_limit column: %w", err)
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

	applyDefaultTokenDefaults(token, s.config)

	query := `INSERT INTO tokens (
		uuid, owner_id, receive_mode, view_mode, receive_secret_hash, receive_secret_prefix,
		default_status, default_content, default_content_type, max_requests, timeout, cors,
		created_at, updated_at, expires_at, rate_limit
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	err = sqlitex.ExecuteTransient(conn, query, &sqlitex.ExecOptions{
		Args: []any{
			token.UUID, nullableString(token.OwnerID), token.ReceiveMode, token.ViewMode,
			nullableString(token.ReceiveSecretHash), nullableString(token.ReceiveSecretPrefix),
			token.DefaultStatus, token.DefaultContent, token.DefaultContentType, token.MaxRequests,
			token.Timeout, boolToSQLite(token.CORS), formatTime(token.CreatedAt), formatTime(token.UpdatedAt),
			formatTime(token.ExpiresAt), token.RateLimit,
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
		default_status, default_content, default_content_type, max_requests, timeout, cors,
		created_at, updated_at, expires_at, rate_limit
	FROM tokens WHERE uuid = ?`

	var token *models.Token
	err = sqlitex.ExecuteTransient(conn, query, &sqlitex.ExecOptions{
		Args: []any{uuid},
		ResultFunc: func(stmt *sqlite.Stmt) error {
			var scanErr error
			token, scanErr = s.scanToken(stmt)
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
	activeAt := formatTime(time.Now().UTC())
	total, err := queryCount(conn, "SELECT COUNT(*) FROM tokens WHERE expires_at > ?", []any{activeAt})
	if err != nil {
		return nil, fmt.Errorf("count tokens: %w", err)
	}

	query := fmt.Sprintf(`SELECT
		uuid, owner_id, receive_mode, view_mode, receive_secret_hash, receive_secret_prefix,
		default_status, default_content, default_content_type, max_requests, timeout, cors,
		created_at, updated_at, expires_at, rate_limit
	FROM tokens
	WHERE expires_at > ?
	ORDER BY %s %s
	LIMIT ? OFFSET ?`, tokenSortColumn(params.SortBy), sortDirection(params.Order))

	var tokens []*models.Token
	err = sqlitex.ExecuteTransient(conn, query, &sqlitex.ExecOptions{
		Args: []any{activeAt, params.Limit, params.Offset},
		ResultFunc: func(stmt *sqlite.Stmt) error {
			token, scanErr := s.scanToken(stmt)
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

	applyDefaultTokenDefaults(token, s.config)

	query := `UPDATE tokens SET
		receive_mode = ?, view_mode = ?, receive_secret_hash = ?, receive_secret_prefix = ?,
		default_status = ?, default_content = ?, default_content_type = ?, max_requests = ?, timeout = ?, cors = ?,
		updated_at = ?, expires_at = ?, rate_limit = ?
	WHERE uuid = ?`

	err = sqlitex.ExecuteTransient(conn, query, &sqlitex.ExecOptions{
		Args: []any{
			token.ReceiveMode, token.ViewMode, nullableString(token.ReceiveSecretHash), nullableString(token.ReceiveSecretPrefix),
			token.DefaultStatus, token.DefaultContent, token.DefaultContentType, token.MaxRequests,
			token.Timeout, boolToSQLite(token.CORS), formatTime(token.UpdatedAt), formatTime(token.ExpiresAt),
			token.RateLimit, token.UUID,
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
		uuid, token_id, ip, hostname, method, user_agent, content, query, headers, form_data, url, size, created_at
	) SELECT ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?
	WHERE EXISTS (
		SELECT 1
		FROM tokens
		WHERE uuid = ?
		  AND (max_requests <= 0 OR (SELECT COUNT(*) FROM requests WHERE token_id = ?) < max_requests)
	)`

	err = sqlitex.ExecuteTransient(conn, query, &sqlitex.ExecOptions{
		Args: []any{
			req.UUID, req.TokenID, req.IP, req.Hostname, req.Method, req.UserAgent,
			req.Content, req.Query, req.Headers, req.FormData, req.URL, req.Size, formatTime(req.CreatedAt),
			req.TokenID, req.TokenID,
		},
	})
	if err != nil {
		return fmt.Errorf("insert request: %w", err)
	}
	if conn.Changes() == 0 {
		exists, existsErr := tokenExists(conn, req.TokenID)
		if existsErr != nil {
			return fmt.Errorf("check token exists: %w", existsErr)
		}
		if !exists {
			return ErrNotFound
		}
		return ErrQuotaExceeded
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
		uuid, token_id, ip, hostname, method, user_agent, content, query, headers, form_data, url, size, created_at
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
	exists, err := tokenExists(conn, tokenID)
	if err != nil {
		return nil, fmt.Errorf("check token exists: %w", err)
	}
	if !exists {
		return nil, ErrNotFound
	}

	whereClauses := []string{"token_id = ?"}
	args := []any{tokenID}

	if params.Method != "" {
		whereClauses = append(whereClauses, "method = ?")
		args = append(args, strings.ToUpper(params.Method))
	}
	if params.IP != "" {
		whereClauses = append(whereClauses, "ip = ?")
		args = append(args, params.IP)
	}
	if params.Search != "" {
		whereClauses = append(whereClauses, "content LIKE ?")
		args = append(args, "%"+params.Search+"%")
	}
	if !params.Since.IsZero() {
		whereClauses = append(whereClauses, "created_at >= ?")
		args = append(args, formatTime(params.Since))
	}
	if !params.Until.IsZero() {
		whereClauses = append(whereClauses, "created_at <= ?")
		args = append(args, formatTime(params.Until))
	}

	whereSQL := strings.Join(whereClauses, " AND ")
	total, err := queryCount(conn, "SELECT COUNT(*) FROM requests WHERE "+whereSQL, args)
	if err != nil {
		return nil, fmt.Errorf("count requests: %w", err)
	}

	query := fmt.Sprintf(`SELECT
		uuid, token_id, ip, hostname, method, user_agent, content, query, headers, form_data, url, size, created_at
	FROM requests
	WHERE %s
	ORDER BY %s %s
	LIMIT ? OFFSET ?`, whereSQL, requestSortColumn(params.SortBy), sortDirection(params.Order))

	queryArgs := append(append([]any{}, args...), params.Limit, params.Offset)

	var requests []*models.Request
	err = sqlitex.ExecuteTransient(conn, query, &sqlitex.ExecOptions{
		Args: queryArgs,
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

// CountRequestsByToken returns the number of requests stored for a token.
func (s *Store) CountRequestsByToken(ctx context.Context, tokenID string) (int, error) {
	conn, err := s.take(ctx)
	if err != nil {
		return 0, err
	}
	defer s.pool.Put(conn)

	exists, err := tokenExists(conn, tokenID)
	if err != nil {
		return 0, fmt.Errorf("check token exists: %w", err)
	}
	if !exists {
		return 0, ErrNotFound
	}

	total, err := queryCount(conn, "SELECT COUNT(*) FROM requests WHERE token_id = ?", []any{tokenID})
	if err != nil {
		return 0, fmt.Errorf("count requests: %w", err)
	}

	return total, nil
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

func (s *Store) scanToken(stmt *sqlite.Stmt) (*models.Token, error) {
	token := &models.Token{
		UUID:               stmt.ColumnText(0),
		ReceiveMode:        stmt.ColumnText(2),
		ViewMode:           stmt.ColumnText(3),
		DefaultStatus:      stmt.ColumnInt(6),
		DefaultContent:     stmt.ColumnText(7),
		DefaultContentType: stmt.ColumnText(8),
		MaxRequests:        stmt.ColumnInt(9),
		Timeout:            stmt.ColumnInt(10),
		CORS:               stmt.ColumnInt(11) != 0,
		RateLimit:          stmt.ColumnInt(15),
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
	token.CreatedAt, err = parseTime(stmt.ColumnText(12))
	if err != nil {
		return nil, fmt.Errorf("parse token created_at: %w", err)
	}
	token.UpdatedAt, err = parseTime(stmt.ColumnText(13))
	if err != nil {
		return nil, fmt.Errorf("parse token updated_at: %w", err)
	}
	if stmt.ColumnIsNull(14) {
		token.ExpiresAt = token.UpdatedAt.UTC().Add(s.config.TokenTTL)
	} else {
		token.ExpiresAt, err = parseTime(stmt.ColumnText(14))
		if err != nil {
			return nil, fmt.Errorf("parse token expires_at: %w", err)
		}
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
		Size:      stmt.ColumnInt(11),
	}

	var err error
	req.CreatedAt, err = parseTime(stmt.ColumnText(12))
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

func applyDefaultTokenDefaults(token *models.Token, cfg Config) {
	applyDefaultTokenMaxRequests(token, cfg.MaxRequests)
	if !token.ExpiresAt.IsZero() {
		return
	}

	base := token.UpdatedAt
	if base.IsZero() {
		base = token.CreatedAt
	}
	if base.IsZero() {
		base = time.Now().UTC()
	}

	token.ExpiresAt = base.UTC().Add(cfg.TokenTTL)
}

func applyDefaultTokenMaxRequests(token *models.Token, defaultMaxRequests int) {
	if token.MaxRequests != 0 {
		return
	}
	token.MaxRequests = defaultMaxRequests
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

func ensureTokenExpiryColumn(conn *sqlite.Conn) error {
	var hasExpiresAt bool
	err := sqlitex.ExecuteTransient(conn, "PRAGMA table_info(tokens)", &sqlitex.ExecOptions{
		ResultFunc: func(stmt *sqlite.Stmt) error {
			if stmt.ColumnText(1) == "expires_at" {
				hasExpiresAt = true
			}
			return nil
		},
	})
	if err != nil {
		return err
	}
	if hasExpiresAt {
		return nil
	}
	return sqlitex.ExecuteTransient(conn, "ALTER TABLE tokens ADD COLUMN expires_at DATETIME", nil)
}

func ensureTokenMaxRequestsColumn(conn *sqlite.Conn) error {
	var hasMaxRequests bool
	err := sqlitex.ExecuteTransient(conn, "PRAGMA table_info(tokens)", &sqlitex.ExecOptions{
		ResultFunc: func(stmt *sqlite.Stmt) error {
			if stmt.ColumnText(1) == "max_requests" {
				hasMaxRequests = true
			}
			return nil
		},
	})
	if err != nil {
		return err
	}
	if hasMaxRequests {
		return nil
	}
	return sqlitex.ExecuteTransient(conn, "ALTER TABLE tokens ADD COLUMN max_requests INTEGER", nil)
}

func (s *Store) backfillTokenExpiry(conn *sqlite.Conn) error {
	type tokenExpirySeed struct {
		uuid      string
		createdAt time.Time
		updatedAt time.Time
	}

	var pending []tokenExpirySeed
	err := sqlitex.ExecuteTransient(conn, `
		SELECT uuid, created_at, updated_at
		FROM tokens
		WHERE expires_at IS NULL OR expires_at = ''
	`, &sqlitex.ExecOptions{
		ResultFunc: func(stmt *sqlite.Stmt) error {
			createdAt, err := parseTime(stmt.ColumnText(1))
			if err != nil {
				return fmt.Errorf("parse backfill created_at: %w", err)
			}
			updatedAt, err := parseTime(stmt.ColumnText(2))
			if err != nil {
				return fmt.Errorf("parse backfill updated_at: %w", err)
			}
			pending = append(pending, tokenExpirySeed{
				uuid:      stmt.ColumnText(0),
				createdAt: createdAt,
				updatedAt: updatedAt,
			})
			return nil
		},
	})
	if err != nil {
		return err
	}

	for _, token := range pending {
		base := token.updatedAt
		if base.IsZero() {
			base = token.createdAt
		}
		expiresAt := base.UTC().Add(s.config.TokenTTL)
		if err := sqlitex.ExecuteTransient(conn, "UPDATE tokens SET expires_at = ? WHERE uuid = ?", &sqlitex.ExecOptions{
			Args: []any{formatTime(expiresAt), token.uuid},
		}); err != nil {
			return err
		}
	}

	return nil
}

func (s *Store) backfillTokenMaxRequests(conn *sqlite.Conn) error {
	return sqlitex.ExecuteTransient(conn, "UPDATE tokens SET max_requests = ? WHERE max_requests IS NULL OR max_requests = 0", &sqlitex.ExecOptions{
		Args: []any{s.config.MaxRequests},
	})
}

// StreamRequestsByToken iterates over all matching requests without pagination, calling fn for each row.
func (s *Store) StreamRequestsByToken(ctx context.Context, tokenID string, params RequestListParams, fn func(*models.Request) error) error {
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

	whereClauses := []string{"token_id = ?"}
	args := []any{tokenID}

	if params.Method != "" {
		whereClauses = append(whereClauses, "method = ?")
		args = append(args, strings.ToUpper(params.Method))
	}
	if params.IP != "" {
		whereClauses = append(whereClauses, "ip = ?")
		args = append(args, params.IP)
	}
	if params.Search != "" {
		whereClauses = append(whereClauses, "content LIKE ?")
		args = append(args, "%"+params.Search+"%")
	}
	if !params.Since.IsZero() {
		whereClauses = append(whereClauses, "created_at >= ?")
		args = append(args, formatTime(params.Since))
	}
	if !params.Until.IsZero() {
		whereClauses = append(whereClauses, "created_at <= ?")
		args = append(args, formatTime(params.Until))
	}

	whereSQL := strings.Join(whereClauses, " AND ")
	query := fmt.Sprintf(`SELECT
		uuid, token_id, ip, hostname, method, user_agent, content, query, headers, form_data, url, size, created_at
	FROM requests
	WHERE %s
	ORDER BY created_at DESC`, whereSQL)

	return sqlitex.ExecuteTransient(conn, query, &sqlitex.ExecOptions{
		Args: args,
		ResultFunc: func(stmt *sqlite.Stmt) error {
			req, scanErr := scanRequest(stmt)
			if scanErr != nil {
				return scanErr
			}
			return fn(req)
		},
	})
}

func ensureRequestSizeColumn(conn *sqlite.Conn) error {
	var hasSize bool
	err := sqlitex.ExecuteTransient(conn, "PRAGMA table_info(requests)", &sqlitex.ExecOptions{
		ResultFunc: func(stmt *sqlite.Stmt) error {
			if stmt.ColumnText(1) == "size" {
				hasSize = true
			}
			return nil
		},
	})
	if err != nil {
		return err
	}
	if hasSize {
		return nil
	}
	return sqlitex.ExecuteTransient(conn, "ALTER TABLE requests ADD COLUMN size INTEGER NOT NULL DEFAULT 0", nil)
}

func backfillRequestSize(conn *sqlite.Conn) error {
	return sqlitex.ExecuteTransient(conn, "UPDATE requests SET size = LENGTH(content) WHERE size = 0 AND content != ''", nil)
}

func ensureTokenRateLimitColumn(conn *sqlite.Conn) error {
	var hasRateLimit bool
	err := sqlitex.ExecuteTransient(conn, "PRAGMA table_info(tokens)", &sqlitex.ExecOptions{
		ResultFunc: func(stmt *sqlite.Stmt) error {
			if stmt.ColumnText(1) == "rate_limit" {
				hasRateLimit = true
			}
			return nil
		},
	})
	if err != nil {
		return err
	}
	if hasRateLimit {
		return nil
	}
	return sqlitex.ExecuteTransient(conn, "ALTER TABLE tokens ADD COLUMN rate_limit INTEGER NOT NULL DEFAULT 0", nil)
}
