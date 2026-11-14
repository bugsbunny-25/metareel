package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"time"

	"github.com/bugsbunny-25/metareel/internal/repository"
)

// User roles. Both can use the public API; admin is for the admin API.
const (
	RoleAdmin  = "admin"
	RoleReader = "reader"
)

var (
	ErrUserNotFound = errors.New("user not found")
	ErrUserExists   = errors.New("a user with that username already exists")
	// ErrLastAdmin keeps at least one enabled admin, so nobody is locked out
	// of the admin console.
	ErrLastAdmin = &ValidationError{Message: "this is the last enabled admin; make another user an admin first"}
)

var usernamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{1,63}$`)

// UserService manages user accounts. Changes that should end a user's access
// (new password, disabling) also revoke their sessions.
type UserService struct {
	log      *slog.Logger
	repo     *repository.UserRepository
	sessions *SessionService
}

type UserResponse struct {
	ID          int64      `json:"id"`
	Username    string     `json:"username"`
	Role        string     `json:"role"`
	Disabled    bool       `json:"disabled"`
	CreatedAt   time.Time  `json:"created_at"`
	LastLoginAt *time.Time `json:"last_login_at"`
}

func NewUserService(log *slog.Logger, repo *repository.UserRepository, sessions *SessionService) *UserService {
	return &UserService{log: log, repo: repo, sessions: sessions}
}

func (s *UserService) List(ctx context.Context) ([]UserResponse, error) {
	users, err := s.repo.ListUsers(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]UserResponse, 0, len(users))
	for _, u := range users {
		out = append(out, toUserResponse(u))
	}
	return out, nil
}

// Get returns one user; ErrUserNotFound if there is none.
func (s *UserService) Get(ctx context.Context, username string) (*UserResponse, error) {
	u, err := s.repo.GetUserByUsername(ctx, username)
	if err != nil {
		return nil, userErr(err)
	}
	out := toUserResponse(u)
	return &out, nil
}

func (s *UserService) Create(ctx context.Context, username, password, role string) (*UserResponse, error) {
	username, err := validateUsername(username)
	if err != nil {
		return nil, err
	}
	if err := validateRole(role); err != nil {
		return nil, err
	}
	if err := validatePassword(password); err != nil {
		return nil, err
	}
	if _, err := s.repo.GetUserByUsername(ctx, username); err == nil {
		return nil, ErrUserExists
	} else if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	hash, err := s.hash(ctx, password)
	if err != nil {
		return nil, err
	}
	u, err := s.repo.CreateUser(ctx, username, hash, role)
	if err != nil {
		return nil, err
	}
	s.log.Info("user created", slog.String("username", u.Username), slog.String("role", u.Role))
	out := toUserResponse(u)
	return &out, nil
}

// EnsureAdmin creates an admin account when no users exist yet (first start).
// It reports whether one was created.
func (s *UserService) EnsureAdmin(ctx context.Context, username, password string) (bool, error) {
	n, err := s.repo.CountUsers(ctx)
	if err != nil || n > 0 {
		return false, err
	}
	if _, err := s.Create(ctx, username, password, RoleAdmin); err != nil {
		return false, err
	}
	return true, nil
}

// SetPassword changes a password and ends the user's sessions, except
// keepSessionID (the caller's own session; 0 ends every one).
func (s *UserService) SetPassword(ctx context.Context, username, password string, keepSessionID int64) error {
	if err := validatePassword(password); err != nil {
		return err
	}
	hash, err := s.hash(ctx, password)
	if err != nil {
		return err
	}
	if err := userErr(s.repo.SetUserPassword(ctx, username, hash)); err != nil {
		return err
	}
	if keepSessionID > 0 {
		_, err = s.sessions.RevokeUserExcept(ctx, username, keepSessionID)
	} else {
		_, err = s.sessions.RevokeUser(ctx, username)
	}
	return err
}

func (s *UserService) SetRole(ctx context.Context, username, role string) error {
	if err := validateRole(role); err != nil {
		return err
	}
	if role != RoleAdmin {
		if err := s.keepAnAdmin(ctx, username); err != nil {
			return err
		}
	}
	if err := userErr(s.repo.SetUserRole(ctx, username, role)); err != nil {
		return err
	}
	// Cached sessions carry the role; make them reload it.
	s.sessions.forgetUser(username)
	return nil
}

// SetDisabled disables (and signs out) or re-enables a user.
func (s *UserService) SetDisabled(ctx context.Context, username string, disabled bool) error {
	if disabled {
		if err := s.keepAnAdmin(ctx, username); err != nil {
			return err
		}
	}
	if err := userErr(s.repo.SetUserDisabled(ctx, username, disabled)); err != nil {
		return err
	}
	if disabled {
		_, err := s.sessions.RevokeUser(ctx, username)
		return err
	}
	return nil
}

// Delete removes a user and their sessions.
func (s *UserService) Delete(ctx context.Context, username string) error {
	if err := s.keepAnAdmin(ctx, username); err != nil {
		return err
	}
	s.sessions.forgetUser(username)
	if err := userErr(s.repo.DeleteUser(ctx, username)); err != nil {
		return err
	}
	s.log.Info("user deleted", slog.String("username", username))
	return nil
}

// EnabledAdmins counts admins who can sign in.
func (s *UserService) EnabledAdmins(ctx context.Context) (int64, error) {
	return s.repo.CountEnabledAdmins(ctx)
}

// keepAnAdmin refuses to demote, disable or delete the last enabled admin.
func (s *UserService) keepAnAdmin(ctx context.Context, username string) error {
	u, err := s.repo.GetUserByUsername(ctx, username)
	if err != nil {
		return userErr(err)
	}
	if u.Role != RoleAdmin || u.Disabled {
		return nil
	}
	n, err := s.repo.CountEnabledAdmins(ctx)
	if err != nil {
		return err
	}
	if n <= 1 {
		return ErrLastAdmin
	}
	return nil
}

func (s *UserService) hash(ctx context.Context, password string) (string, error) {
	release, err := acquirePasswordSlot(ctx)
	if err != nil {
		return "", err
	}
	defer release()
	return hashPassword(password)
}

func validateUsername(username string) (string, error) {
	username = strings.TrimSpace(username)
	if !usernamePattern.MatchString(username) {
		return "", &ValidationError{Message: "username must be 2-64 characters: letters, digits, '.', '_' or '-', starting with a letter or digit"}
	}
	return username, nil
}

func validateRole(role string) error {
	switch role {
	case RoleAdmin, RoleReader:
		return nil
	}
	return &ValidationError{Message: fmt.Sprintf("role must be %q or %q", RoleAdmin, RoleReader)}
}

func userErr(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return ErrUserNotFound
	}
	return err
}

func toUserResponse(u repository.UserRecord) UserResponse {
	return UserResponse{ID: u.ID, Username: u.Username, Role: u.Role, Disabled: u.Disabled,
		CreatedAt: u.CreatedAt, LastLoginAt: u.LastLoginAt}
}

// ChangeOwnPassword is a signed-in user changing their own password: the
// current one must check out, and every other session of theirs is ended
// (the one making the change stays signed in). It returns how many ended.
func (s *UserService) ChangeOwnPassword(ctx context.Context, username, current, next, ip string, keepSessionID int64) (int64, error) {
	if err := s.sessions.CheckPassword(ctx, username, current, ip); err != nil {
		if errors.Is(err, ErrInvalidLogin) {
			return 0, &ValidationError{Message: "your current password isn't right"}
		}
		return 0, err
	}
	if current == next {
		return 0, &ValidationError{Message: "the new password is the same as the current one"}
	}
	if err := validatePassword(next); err != nil {
		return 0, err
	}
	hash, err := s.hash(ctx, next)
	if err != nil {
		return 0, err
	}
	if err := userErr(s.repo.SetUserPassword(ctx, username, hash)); err != nil {
		return 0, err
	}
	n, err := s.sessions.RevokeUserExcept(ctx, username, keepSessionID)
	if err != nil {
		return 0, err
	}
	s.log.Info("password changed by its user", "username", username, "sessions_ended", n)
	return n, nil
}
