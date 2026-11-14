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

// NewSession is a session to store (by token digest) that expires after TTL.
type NewSession struct {
	UserID         int64
	TokenHash      string
	TTL            time.Duration
	UserAgent      string
	IP             string
	DeviceName     string
	RememberDevice bool
}

// SessionIdle is how long a session may go unused: Default for ordinary
// sessions, Device for "remember this device" ones.
type SessionIdle struct {
	Default time.Duration
	Device  time.Duration
}

// CreateSession stores a session and returns its id and expiry.
func (r *UserRepository) CreateSession(ctx context.Context, s NewSession) (int64, time.Time, error) {
	row, err := r.q.CreateSession(ctx, sqlc.CreateSessionParams{
		UserID: s.UserID, TokenHash: s.TokenHash, Ttl: sqliteOffset(s.TTL), UserAgent: s.UserAgent, Ip: s.IP,
		DeviceName: s.DeviceName, RememberDevice: s.RememberDevice,
	})
	if err != nil {
		return 0, time.Time{}, fmt.Errorf("create session: %w", err)
	}
	return row.ID, row.ExpiresAt, nil
}

// GetActiveSession returns sql.ErrNoRows unless the session exists, has not
// expired, was used within its idle window and belongs to an enabled user.
func (r *UserRepository) GetActiveSession(ctx context.Context, tokenHash string, idle SessionIdle) (SessionRecord, error) {
	row, err := r.q.GetActiveSession(ctx, sqlc.GetActiveSessionParams{
		TokenHash: tokenHash, Idle: sqliteOffset(-idle.Default), DeviceIdle: sqliteOffset(-idle.Device),
	})
	if err != nil {
		return SessionRecord{}, err
	}
	return SessionRecord{ID: row.ID, UserID: row.UserID, Username: row.Username, Role: row.Role, ExpiresAt: row.ExpiresAt}, nil
}

// ActiveSessionDigest returns the token digest of an active session by id
// (same rules as GetActiveSession), or sql.ErrNoRows.
func (r *UserRepository) ActiveSessionDigest(ctx context.Context, id int64, idle SessionIdle) (string, error) {
	return r.q.GetActiveSessionDigest(ctx, sqlc.GetActiveSessionDigestParams{
		ID: id, Idle: sqliteOffset(-idle.Default), DeviceIdle: sqliteOffset(-idle.Device),
	})
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

// DeleteExpiredSessions removes sessions past their expiry or unused for
// their idle window.
func (r *UserRepository) DeleteExpiredSessions(ctx context.Context, idle SessionIdle) (int64, error) {
	n, err := r.q.DeleteExpiredSessions(ctx, sqlc.DeleteExpiredSessionsParams{
		Idle: sqliteOffset(-idle.Default), DeviceIdle: sqliteOffset(-idle.Device),
	})
	if err != nil {
		return 0, fmt.Errorf("delete expired sessions: %w", err)
	}
	return n, nil
}

func (r *UserRepository) CountEnabledAdmins(ctx context.Context) (int64, error) {
	n, err := r.q.CountEnabledAdmins(ctx)
	if err != nil {
		return 0, fmt.Errorf("count admins: %w", err)
	}
	return n, nil
}

// DeleteUser deletes a user and (by cascade) their sessions; sql.ErrNoRows
// when there is no such user.
func (r *UserRepository) DeleteUser(ctx context.Context, username string) error {
	return affected(r.q.DeleteUser(ctx, username))
}

// SessionInfo describes an active session for listings.
type SessionInfo struct {
	ID         int64
	UserID     int64
	Username   string
	CreatedAt  time.Time
	LastSeenAt time.Time
	ExpiresAt  time.Time
	UserAgent  string
	IP         string
	DeviceName string
	// RememberDevice marks a "remember this device" session (longer TTL and
	// idle timeout).
	RememberDevice bool
}

// ListActiveSessions lists sessions that have not expired and were used
// within their idle window.
func (r *UserRepository) ListActiveSessions(ctx context.Context, idle SessionIdle) ([]SessionInfo, error) {
	rows, err := r.q.ListActiveSessions(ctx, sqlc.ListActiveSessionsParams{
		Idle: sqliteOffset(-idle.Default), DeviceIdle: sqliteOffset(-idle.Device),
	})
	if err != nil {
		return nil, fmt.Errorf("list sessions: %w", err)
	}
	out := make([]SessionInfo, 0, len(rows))
	for _, row := range rows {
		out = append(out, SessionInfo{ID: row.ID, UserID: row.UserID, Username: row.Username, CreatedAt: row.CreatedAt,
			LastSeenAt: row.LastSeenAt, ExpiresAt: row.ExpiresAt, UserAgent: row.UserAgent, IP: row.Ip,
			DeviceName: row.DeviceName, RememberDevice: row.RememberDevice})
	}
	return out, nil
}

// DeleteSessionByID removes one session and returns its token digest;
// sql.ErrNoRows when there is none.
func (r *UserRepository) DeleteSessionByID(ctx context.Context, id int64) (string, error) {
	return r.q.DeleteSessionByID(ctx, id)
}

// DeleteUserSessionsExcept removes a user's sessions other than keepID.
func (r *UserRepository) DeleteUserSessionsExcept(ctx context.Context, username string, keepID int64) (int64, error) {
	n, err := r.q.DeleteUserSessionsExcept(ctx, sqlc.DeleteUserSessionsExceptParams{Username: username, KeepID: keepID})
	if err != nil {
		return 0, fmt.Errorf("delete user sessions: %w", err)
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
