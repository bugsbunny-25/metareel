package service

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"

	"github.com/bugsbunny-25/metareel/internal/repository"
)

const (
	SessionTokenPrefix = "mrs_"
	sessionTokenLen    = len(SessionTokenPrefix) + 43 // 32 random bytes, base64url

	// sessionCacheTTL bounds how long a session is trusted from memory
	// before it is checked against the database again (revocations made in
	// this process take effect at once; ones made elsewhere, e.g. by the
	// CLI, within this time).
	sessionCacheTTL      = 30 * time.Second
	sessionTouchInterval = time.Minute
	maxUserAgentLen      = 256
)

var (
	ErrSessionInvalid   = errors.New("invalid or expired session")
	ErrSessionNotFound  = errors.New("session not found")
	ErrInvalidLogin     = errors.New("invalid username or password")
	ErrLoginRateLimited = errors.New("too many login attempts; try again later")
)

// SessionConfig sets how long sessions last.
type SessionConfig struct {
	// TTL is a session's absolute lifetime.
	TTL time.Duration
	// IdleTimeout ends a session that has not been used for this long.
	IdleTimeout time.Duration
}

// SessionService signs users in and checks session tokens. Only digests of
// tokens are stored; active sessions are cached briefly in memory so most
// requests don't hit the database.
type SessionService struct {
	log  *slog.Logger
	repo *repository.UserRepository
	cfg  SessionConfig
	now  func() time.Time

	mu        sync.Mutex
	cache     map[string]cachedSession // token digest -> session
	lastTouch map[int64]time.Time
	ipLimits  map[string]*rate.Limiter
	userLimit map[string]*rate.Limiter
}

type cachedSession struct {
	rec      repository.SessionRecord
	loadedAt time.Time
}

// SessionUser is the signed-in user as returned to clients.
type SessionUser struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
	Role     string `json:"role"`
}

// LoginResult carries a new session token; it is not stored and cannot be
// shown again.
type LoginResult struct {
	Token     string      `json:"token"`
	ExpiresAt time.Time   `json:"expires_at"`
	User      SessionUser `json:"user"`
}

func NewSessionService(log *slog.Logger, repo *repository.UserRepository, cfg SessionConfig) *SessionService {
	if cfg.TTL <= 0 {
		cfg.TTL = 30 * 24 * time.Hour
	}
	if cfg.IdleTimeout <= 0 || cfg.IdleTimeout > cfg.TTL {
		cfg.IdleTimeout = cfg.TTL
	}
	return &SessionService{
		log: log, repo: repo, cfg: cfg, now: time.Now,
		cache:     map[string]cachedSession{},
		lastTouch: map[int64]time.Time{},
		ipLimits:  map[string]*rate.Limiter{},
		userLimit: map[string]*rate.Limiter{},
	}
}

// Login checks a username and password and starts a session. Unknown users,
// disabled users and wrong passwords all fail the same way and take about
// the same time.
func (s *SessionService) Login(ctx context.Context, username, password, userAgent, ip string) (*LoginResult, error) {
	username = strings.TrimSpace(username)
	if username == "" || password == "" {
		return nil, &ValidationError{Message: "username and password are required"}
	}
	if len(username) > 64 || len(password) > maxPasswordLen {
		return nil, ErrInvalidLogin
	}
	if !s.allowLogin(ip, username) {
		s.log.Warn("login rate limited", slog.String("ip", ip), slog.String("username", username))
		return nil, ErrLoginRateLimited
	}

	user, err := s.repo.GetUserByUsername(ctx, username)
	known := err == nil && !user.Disabled
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	hash := user.PasswordHash
	if !known {
		hash = dummyPasswordHash()
	}
	release, err := acquirePasswordSlot(ctx)
	if err != nil {
		return nil, err
	}
	ok, err := verifyPassword(password, hash)
	release()
	if err != nil {
		s.log.Error("stored password hash unreadable", slog.String("username", user.Username), slog.Any("err", err))
	}
	if !known || !ok {
		s.loginFailed(username)
		s.log.Info("login failed", slog.String("ip", ip), slog.String("username", username))
		return nil, ErrInvalidLogin
	}

	token, digest, err := generateSessionToken()
	if err != nil {
		return nil, err
	}
	if len(userAgent) > maxUserAgentLen {
		userAgent = userAgent[:maxUserAgentLen]
	}
	expires, err := s.repo.CreateSession(ctx, user.ID, digest, s.cfg.TTL, userAgent, ip)
	if err != nil {
		return nil, err
	}
	if err := s.repo.RecordUserLogin(ctx, user.ID); err != nil {
		s.log.Warn("recording login failed", slog.Int64("user_id", user.ID), slog.Any("err", err))
	}
	s.log.Info("login", slog.String("username", user.Username), slog.String("ip", ip))
	return &LoginResult{
		Token: token, ExpiresAt: expires,
		User: SessionUser{ID: user.ID, Username: user.Username, Role: user.Role},
	}, nil
}

// Lookup returns the active session for a token, or ErrSessionInvalid.
func (s *SessionService) Lookup(ctx context.Context, token string) (repository.SessionRecord, error) {
	if !looksLikeSessionToken(token) {
		return repository.SessionRecord{}, ErrSessionInvalid
	}
	digest := hashSessionToken(token)
	now := s.now()

	s.mu.Lock()
	c, hit := s.cache[digest]
	s.mu.Unlock()
	if hit && now.Sub(c.loadedAt) < sessionCacheTTL && now.Before(c.rec.ExpiresAt) {
		s.touch(c.rec.ID)
		return c.rec, nil
	}

	rec, err := s.repo.GetActiveSession(ctx, digest, s.cfg.IdleTimeout)
	if err != nil {
		s.mu.Lock()
		delete(s.cache, digest)
		s.mu.Unlock()
		if errors.Is(err, sql.ErrNoRows) {
			return repository.SessionRecord{}, ErrSessionInvalid
		}
		return repository.SessionRecord{}, fmt.Errorf("look up session: %w", err)
	}
	s.mu.Lock()
	if len(s.cache) > 10000 {
		s.cache = map[string]cachedSession{}
	}
	s.cache[digest] = cachedSession{rec: rec, loadedAt: now}
	s.mu.Unlock()
	s.touch(rec.ID)
	return rec, nil
}

// Logout ends the session of a token (a no-op for unknown tokens).
func (s *SessionService) Logout(ctx context.Context, token string) error {
	if !looksLikeSessionToken(token) {
		return nil
	}
	digest := hashSessionToken(token)
	s.mu.Lock()
	delete(s.cache, digest)
	s.mu.Unlock()
	_, err := s.repo.DeleteSession(ctx, digest)
	return err
}

// RevokeUser ends every session of a user and returns how many there were.
func (s *SessionService) RevokeUser(ctx context.Context, username string) (int64, error) {
	s.forgetUser(username)
	n, err := s.repo.DeleteUserSessions(ctx, username)
	if err != nil {
		return 0, err
	}
	if n > 0 {
		s.log.Info("sessions revoked", slog.String("username", username), slog.Int64("sessions", n))
	}
	return n, nil
}

// RevokeUserExcept ends a user's sessions other than keepID (e.g. the one
// that changed the password).
func (s *SessionService) RevokeUserExcept(ctx context.Context, username string, keepID int64) (int64, error) {
	s.forgetUser(username)
	return s.repo.DeleteUserSessionsExcept(ctx, username, keepID)
}

// SessionResponse is an active session as listed in the admin API.
type SessionResponse struct {
	ID         int64     `json:"id"`
	UserID     int64     `json:"user_id"`
	Username   string    `json:"username"`
	CreatedAt  time.Time `json:"created_at"`
	LastSeenAt time.Time `json:"last_seen_at"`
	ExpiresAt  time.Time `json:"expires_at"`
	UserAgent  string    `json:"user_agent"`
	IP         string    `json:"ip"`
	Current    bool      `json:"current"` // the session making the request
}

// List returns every active session, most recently used first.
func (s *SessionService) List(ctx context.Context) ([]SessionResponse, error) {
	rows, err := s.repo.ListActiveSessions(ctx, s.cfg.IdleTimeout)
	if err != nil {
		return nil, err
	}
	out := make([]SessionResponse, 0, len(rows))
	for _, r := range rows {
		out = append(out, SessionResponse{ID: r.ID, UserID: r.UserID, Username: r.Username, CreatedAt: r.CreatedAt,
			LastSeenAt: r.LastSeenAt, ExpiresAt: r.ExpiresAt, UserAgent: r.UserAgent, IP: r.IP})
	}
	return out, nil
}

// Revoke ends one session by ID; ErrSessionNotFound if there is none.
func (s *SessionService) Revoke(ctx context.Context, id int64) error {
	digest, err := s.repo.DeleteSessionByID(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrSessionNotFound
	}
	if err != nil {
		return fmt.Errorf("revoke session: %w", err)
	}
	s.mu.Lock()
	delete(s.cache, digest)
	s.mu.Unlock()
	return nil
}

// forgetUser drops a user's cached sessions so the next request rereads them.
func (s *SessionService) forgetUser(username string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for k, c := range s.cache {
		if strings.EqualFold(c.rec.Username, username) {
			delete(s.cache, k)
		}
	}
}

// PruneExpired deletes expired and idle sessions (maintenance job).
func (s *SessionService) PruneExpired(ctx context.Context) (int64, error) {
	return s.repo.DeleteExpiredSessions(ctx, s.cfg.IdleTimeout)
}

// allowLogin rate limits login attempts: every attempt per client IP, and
// failed attempts per username (see loginFailed).
func (s *SessionService) allowLogin(ip, username string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.ipLimits) > 10000 {
		s.ipLimits = map[string]*rate.Limiter{}
	}
	ipOK := limiterFor(s.ipLimits, ip, 6*time.Second, 10).Allow()
	userOK := limiterFor(s.userLimit, strings.ToLower(username), 30*time.Second, 5).Tokens() >= 1
	return ipOK && userOK
}

// loginFailed spends one of the username's failed-attempt allowance.
func (s *SessionService) loginFailed(username string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.userLimit) > 10000 {
		s.userLimit = map[string]*rate.Limiter{}
	}
	limiterFor(s.userLimit, strings.ToLower(username), 30*time.Second, 5).Allow()
}

func limiterFor(m map[string]*rate.Limiter, key string, every time.Duration, burst int) *rate.Limiter {
	l, ok := m[key]
	if !ok {
		l = rate.NewLimiter(rate.Every(every), burst)
		m[key] = l
	}
	return l
}

// touch records session use at most once per sessionTouchInterval.
func (s *SessionService) touch(id int64) {
	now := s.now()
	s.mu.Lock()
	if now.Sub(s.lastTouch[id]) < sessionTouchInterval {
		s.mu.Unlock()
		return
	}
	if len(s.lastTouch) > 10000 {
		s.lastTouch = map[int64]time.Time{}
	}
	s.lastTouch[id] = now
	s.mu.Unlock()
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := s.repo.TouchSession(ctx, id); err != nil {
			s.log.Warn("recording session use failed", slog.Int64("session_id", id), slog.Any("err", err))
		}
	}()
}

func looksLikeSessionToken(token string) bool {
	return len(token) == sessionTokenLen && strings.HasPrefix(token, SessionTokenPrefix)
}

func generateSessionToken() (token, digest string, err error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", "", fmt.Errorf("generate session token: %w", err)
	}
	token = SessionTokenPrefix + base64.RawURLEncoding.EncodeToString(b)
	return token, hashSessionToken(token), nil
}

var sessionTokenSalt = []byte("metareel/session/v1")

// hashSessionToken: like API keys, session tokens are 256-bit random values,
// so a deterministic PBKDF2 digest is enough and keeps lookups by digest.
func hashSessionToken(token string) string {
	return hashToken(token, sessionTokenSalt)
}
