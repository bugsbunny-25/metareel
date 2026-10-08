package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/bugsbunny-25/metareel/internal/repository/sqlc"
)

type APIKeyRepository struct {
	q *sqlc.Queries
}

func NewAPIKeyRepository(db *sql.DB) *APIKeyRepository {
	return &APIKeyRepository{q: sqlc.New(db)}
}

type APIKeyRecord struct {
	ID         int64
	Name       string
	KeyPrefix  string
	KeyHash    string
	CreatedAt  time.Time
	RotatedAt  *time.Time
	LastUsedAt *time.Time
}

func (r *APIKeyRepository) ListAPIKeys(ctx context.Context) ([]APIKeyRecord, error) {
	rows, err := r.q.ListAPIKeys(ctx)
	if err != nil {
		return nil, fmt.Errorf("list api keys: %w", err)
	}
	out := make([]APIKeyRecord, 0, len(rows))
	for _, row := range rows {
		out = append(out, apiKeyFromRow(row))
	}
	return out, nil
}

func (r *APIKeyRepository) CreateAPIKey(ctx context.Context, name, prefix, hash string) (APIKeyRecord, error) {
	row, err := r.q.CreateAPIKey(ctx, sqlc.CreateAPIKeyParams{Name: name, KeyPrefix: prefix, KeyHash: hash})
	if err != nil {
		return APIKeyRecord{}, fmt.Errorf("create api key: %w", err)
	}
	return apiKeyFromRow(row), nil
}

// RotateAPIKey replaces a key's secret; sql.ErrNoRows if it does not exist.
func (r *APIKeyRepository) RotateAPIKey(ctx context.Context, id int64, prefix, hash string) (APIKeyRecord, error) {
	row, err := r.q.RotateAPIKey(ctx, sqlc.RotateAPIKeyParams{ID: id, KeyPrefix: prefix, KeyHash: hash})
	if err != nil {
		return APIKeyRecord{}, err
	}
	return apiKeyFromRow(row), nil
}

// RenameAPIKey sql.ErrNoRows if it does not exist.
func (r *APIKeyRepository) RenameAPIKey(ctx context.Context, id int64, name string) (APIKeyRecord, error) {
	row, err := r.q.RenameAPIKey(ctx, sqlc.RenameAPIKeyParams{ID: id, Name: name})
	if err != nil {
		return APIKeyRecord{}, err
	}
	return apiKeyFromRow(row), nil
}

// DeleteAPIKey reports whether a key was deleted.
func (r *APIKeyRepository) DeleteAPIKey(ctx context.Context, id int64) (bool, error) {
	n, err := r.q.DeleteAPIKey(ctx, id)
	if err != nil {
		return false, fmt.Errorf("delete api key: %w", err)
	}
	return n > 0, nil
}

func (r *APIKeyRepository) TouchAPIKey(ctx context.Context, id int64) error {
	return r.q.TouchAPIKey(ctx, id)
}

func (r *APIKeyRepository) ListAppSettings(ctx context.Context) (map[string]string, error) {
	rows, err := r.q.ListAppSettings(ctx)
	if err != nil {
		return nil, fmt.Errorf("list app settings: %w", err)
	}
	out := make(map[string]string, len(rows))
	for _, row := range rows {
		out[row.Key] = row.Value
	}
	return out, nil
}

func (r *APIKeyRepository) UpsertAppSetting(ctx context.Context, key, value string) error {
	if err := r.q.UpsertAppSetting(ctx, sqlc.UpsertAppSettingParams{Key: key, Value: value}); err != nil {
		return fmt.Errorf("save app setting %s: %w", key, err)
	}
	return nil
}

func apiKeyFromRow(row sqlc.ApiKey) APIKeyRecord {
	rec := APIKeyRecord{ID: row.ID, Name: row.Name, KeyPrefix: row.KeyPrefix, KeyHash: row.KeyHash, CreatedAt: row.CreatedAt}
	if row.RotatedAt.Valid {
		t := row.RotatedAt.Time
		rec.RotatedAt = &t
	}
	if row.LastUsedAt.Valid {
		t := row.LastUsedAt.Time
		rec.LastUsedAt = &t
	}
	return rec
}
