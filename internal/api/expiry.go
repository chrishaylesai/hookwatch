package api

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/chrishaylesai/hookwatch/internal/models"
	"github.com/chrishaylesai/hookwatch/internal/store"
)

var timeNow = time.Now

var errTokenExpired = errors.New("token expired")

func loadActiveToken(ctx context.Context, db *store.Store, tokenID string, touch bool) (*models.Token, error) {
	token, err := db.GetToken(ctx, tokenID)
	if err != nil {
		return nil, err
	}

	now := timeNow().UTC()
	if !token.Persistent && !token.ExpiresAt.After(now) {
		return nil, errTokenExpired
	}

	if touch && !token.Persistent {
		expiresAt, err := db.TouchTokenExpiry(ctx, tokenID, now)
		if err != nil {
			return nil, err
		}
		token.ExpiresAt = expiresAt
	}

	return token, nil
}

func refreshTokenExpiry(ctx context.Context, db *store.Store, token *models.Token) error {
	if token.Persistent {
		return nil
	}
	expiresAt, err := db.TouchTokenExpiry(ctx, token.UUID, timeNow().UTC())
	if err != nil {
		return err
	}
	token.ExpiresAt = expiresAt
	return nil
}

func isTokenExpiredError(err error) bool {
	return errors.Is(err, errTokenExpired)
}

func writeTokenExpired(w http.ResponseWriter) {
	writeError(w, http.StatusGone, "token expired")
}

func writeRequestQuotaExceeded(w http.ResponseWriter) {
	writeError(w, http.StatusGone, "request quota exceeded")
}
