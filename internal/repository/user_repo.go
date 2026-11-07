package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/bugsbunny-25/metareel/internal/repository/sqlc"
)

// UserRepository stores user accounts and their sign-in sessions.
type UserRepository struct {
	q *sqlc.Queries
}

func NewUserRepository(db *sql.DB) *UserRepository {
	return &UserRepository{q: sqlc.New(db)}
}

type UserRecord struct {
	ID           int64
	Username     string
	PasswordHash string
	Role         string
	Disabled     bool
	CreatedAt    time.Time
	UpdatedAt    time.Time
	LastLoginAt  *time.Time
}

// SessionRecord is an active session with its user.
type SessionRecord struct {
	ID        int64
	UserID    int64
	Username  string
	Role      string
	ExpiresAt time.Time
}

func (r *UserRepository) CountUsers(ctx context.Context) (int64, error) {
	n, err := r.q.CountUsers(ctx)
	if err != nil {
		return 0, fmt.Errorf("count users: %w", err)
	}
	return n, nil
}

func (r *UserRepository) ListUsers(ctx context.Context) ([]UserRecord, error) {
	rows, err := r.q.ListUsers(ctx)
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	out := make([]UserRecord, 0, len(rows))
	for _, row := range rows {
		out = append(out, userFromRow(row))
	}
	return out, nil
}

// GetUserByUsername returns sql.ErrNoRows when there is no such user
// (usernames compare case-insensitively).
func (r *UserRepository) GetUserByUsername(ctx context.Context, username string) (UserRecord, error) {
	row, err := r.q.GetUserByUsername(ctx, username)
	if err != nil {
		return UserRecord{}, err
	}
	return userFromRow(row), nil
}

func (r *UserRepository) CreateUser(ctx context.Context, username, passwordHash, role string) (UserRecord, error) {
	row, err := r.q.CreateUser(ctx, sqlc.CreateUserParams{Username: username, PasswordHash: passwordHash, Role: role})
	if err != nil {
		return UserRecord{}, fmt.Errorf("create user: %w", err)
	}
	return userFromRow(row), nil
}

// SetUserPassword, SetUserRole and SetUserDisabled return sql.ErrNoRows when
// there is no such user.
func (r *UserRepository) SetUserPassword(ctx context.Context, username, passwordHash string) error {
	return affected(r.q.SetUserPassword(ctx, sqlc.SetUserPasswordParams{Username: username, PasswordHash: passwordHash}))
}

func (r *UserRepository) SetUserRole(ctx context.Context, username, role string) error {
	return affected(r.q.SetUserRole(ctx, sqlc.SetUserRoleParams{Username: username, Role: role}))
}

func (r *UserRepository) SetUserDisabled(ctx context.Context, username string, disabled bool) error {
	return affected(r.q.SetUserDisabled(ctx, sqlc.SetUserDisabledParams{Username: username, Disabled: disabled}))
}

func (r *UserRepository) RecordUserLogin(ctx context.Context, id int64) error {
	return r.q.RecordUserLogin(ctx, id)
}

// CreateSession stores a session (by token digest) that expires after ttl.
func (r *UserRepository) CreateSession(ctx context.Context, userID int64, tokenHash string, ttl time.Duration, userAgent, ip string) (time.Time, error) {
	row, err := r.q.CreateSession(ctx, sqlc.CreateSessionParams{
		UserID: userID, TokenHash: tokenHash, Ttl: sqliteOffset(ttl), UserAgent: userAgent, Ip: ip,
	})
	if err != nil {
		return time.Time{}, fmt.Errorf("create session: %w", err)
	}
	return row.ExpiresAt, nil
}

// GetActiveSession returns sql.ErrNoRows unless the session exists, has not
// expired, was used within idle and belongs to an enabled user.
func (r *UserRepository) GetActiveSession(ctx context.Context, tokenHash string, idle time.Duration) (SessionRecord, error) {
	row, err := r.q.GetActiveSession(ctx, sqlc.GetActiveSessionParams{TokenHash: tokenHash, Idle: sqliteOffset(-idle)})
	if err != nil {
		return SessionRecord{}, err
	}
	return SessionRecord{ID: row.ID, UserID: row.UserID, Username: row.Username, Role: row.Role, ExpiresAt: row.ExpiresAt}, nil
}

func (r *UserRepository) TouchSession(ctx context.Context, id int64) error {
	return r.q.TouchSession(ctx, id)
}

// DeleteSession removes a session by token digest; ok is false when there
// was none.
func (r *UserRepository) DeleteSession(ctx context.Context, tokenHash string) (ok bool, err error) {
	if _, err := r.q.DeleteSessionByHash(ctx, tokenHash); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, fmt.Errorf("delete session: %w", err)
	}
	return true, nil
}

func (r *UserRepository) DeleteUserSessions(ctx context.Context, username string) (int64, error) {
	n, err := r.q.DeleteUserSessions(ctx, username)
	if err != nil {
		return 0, fmt.Errorf("delete user sessions: %w", err)
	}
	return n, nil
}

// DeleteExpiredSessions removes sessions past their expiry or unused for idle.
func (r *UserRepository) DeleteExpiredSessions(ctx context.Context, idle time.Duration) (int64, error) {
	n, err := r.q.DeleteExpiredSessions(ctx, sqliteOffset(-idle))
	if err != nil {
		return 0, fmt.Errorf("delete expired sessions: %w", err)
	}
	return n, nil
}

// sqliteOffset formats a duration as a datetime() modifier ("+3600 seconds").
func sqliteOffset(d time.Duration) string {
	return fmt.Sprintf("%+d seconds", int64(d/time.Second))
}

func affected(n int64, err error) error {
	if err != nil {
		return err
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func userFromRow(row sqlc.User) UserRecord {
	rec := UserRecord{
		ID: row.ID, Username: row.Username, PasswordHash: row.PasswordHash, Role: row.Role,
		Disabled: row.Disabled, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}
	if row.LastLoginAt.Valid {
		t := row.LastLoginAt.Time
		rec.LastLoginAt = &t
	}
	return rec
}
