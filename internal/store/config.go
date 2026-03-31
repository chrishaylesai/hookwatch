package store

import "time"

const (
	DefaultMaxRequests          = 500
	DefaultTokenTTL             = 7 * 24 * time.Hour
	DefaultTokenCleanupInterval = time.Hour
)

// Config controls store-backed operational defaults.
type Config struct {
	TokenTTL    time.Duration
	MaxRequests int
}

func normalizeConfig(cfg Config) Config {
	if cfg.TokenTTL <= 0 {
		cfg.TokenTTL = DefaultTokenTTL
	}
	if cfg.MaxRequests <= 0 {
		cfg.MaxRequests = DefaultMaxRequests
	}
	return cfg
}
