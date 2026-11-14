package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/bugsbunny-25/metareel/internal/repository/sqlc"
)

// UserPreferences returns a user's stored preferences JSON and when they were
// last saved; "{}" and "" when they never saved any.
func (r *TrackingRepository) UserPreferences(ctx context.Context, userID int64) (string, string, error) {
	row, err := r.q.GetUserPreferences(ctx, userID)
	if errors.Is(err, sql.ErrNoRows) {
		return "{}", "", nil
	}
	if err != nil {
		return "", "", fmt.Errorf("user preferences: %w", err)
	}
	return row.Data, row.UpdatedAt, nil
}

// SaveUserPreferences replaces a user's preferences JSON.
func (r *TrackingRepository) SaveUserPreferences(ctx context.Context, userID int64, data string) error {
	if err := r.q.SaveUserPreferences(ctx, sqlc.SaveUserPreferencesParams{UserID: userID, Data: data}); err != nil {
		return fmt.Errorf("save user preferences: %w", err)
	}
	return nil
}
