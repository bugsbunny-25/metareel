package service

import (
	"context"
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"

	"github.com/bugsbunny-25/metareel/internal/repository"
)

// Settings keys in app_settings.
const (
	settingRequireAPIKey = "public_api.require_key"
	settingRateLimit     = "public_api.rate_limit_per_minute"

	defaultRateLimitPerMinute = 300
	apiKeyPrefix              = "mr_"
	apiKeyDisplayLen          = 10 // characters of the key shown in listings
	apiKeyTouchInterval       = time.Minute
)

var (
	ErrAPIKeyMissing = errors.New("missing API key: send it in the X-API-Key header")
	ErrAPIKeyInvalid = errors.New("invalid API key")
	ErrRateLimited   = errors.New("rate limit exceeded")
)

// APIKeyService manages API keys for the public API and checks requests
// against them. Key hashes and settings are cached in memory and reloaded
// whenever they change, so authenticating a request does not hit the DB.
type APIKeyService struct {
	log  *slog.Logger
	repo *repository.APIKeyRepository
	now  func() time.Time

	mu         sync.RWMutex
	keysByHash map[string]int64 // hashAPIKey digest -> key id
	settings   PublicAPISettings
	limiters   map[string]*rate.Limiter // by key id or client IP
	lastTouch  map[int64]time.Time
}

type PublicAPISettings struct {
	// RequireAPIKey makes every non-admin /api/v1 endpoint need a valid key.
	RequireAPIKey bool `json:"require_api_key"`
	// RateLimitPerMinute caps requests per API key (or per client IP when
	// keys are not required); 0 disables the limit.
	RateLimitPerMinute int `json:"rate_limit_per_minute"`
}

type APIKeyResponse struct {
	ID         int64      `json:"id"`
	Name       string     `json:"name"`
	Prefix     string     `json:"prefix"` // e.g. "mr_Ab3dEf…"
	CreatedAt  time.Time  `json:"created_at"`
	RotatedAt  *time.Time `json:"rotated_at"`
	LastUsedAt *time.Time `json:"last_used_at"`
}

// APIKeySecretResponse carries a newly generated key. The key is not stored
// and cannot be shown again.
type APIKeySecretResponse struct {
	APIKeyResponse
	Key string `json:"key"`
}

func NewAPIKeyService(log *slog.Logger, repo *repository.APIKeyRepository) *APIKeyService {
	return &APIKeyService{
		log:        log,
		repo:       repo,
		now:        time.Now,
		keysByHash: map[string]int64{},
		settings:   PublicAPISettings{RequireAPIKey: true, RateLimitPerMinute: defaultRateLimitPerMinute},
		limiters:   map[string]*rate.Limiter{},
		lastTouch:  map[int64]time.Time{},
	}
}

// Load reads keys and settings into memory; call at startup.
func (s *APIKeyService) Load(ctx context.Context) error {
	if err := s.reloadKeys(ctx); err != nil {
		return err
	}
	return s.reloadSettings(ctx)
}

func (s *APIKeyService) reloadKeys(ctx context.Context) error {
	keys, err := s.repo.ListAPIKeys(ctx)
	if err != nil {
		return err
	}
	byHash := make(map[string]int64, len(keys))
	for _, k := range keys {
		byHash[k.KeyHash] = k.ID
	}
	s.mu.Lock()
	s.keysByHash = byHash
	s.limiters = map[string]*rate.Limiter{} // drop limiters of rotated / deleted keys
	s.mu.Unlock()
	return nil
}

func (s *APIKeyService) reloadSettings(ctx context.Context) error {
	raw, err := s.repo.ListAppSettings(ctx)
	if err != nil {
		return err
	}
	settings := PublicAPISettings{RequireAPIKey: true, RateLimitPerMinute: defaultRateLimitPerMinute}
	if v, ok := raw[settingRequireAPIKey]; ok {
		settings.RequireAPIKey = v != "false"
	}
	if v, ok := raw[settingRateLimit]; ok {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			settings.RateLimitPerMinute = n
		}
	}
	s.mu.Lock()
	s.settings = settings
	s.limiters = map[string]*rate.Limiter{}
	s.mu.Unlock()
	return nil
}

// Authorize checks a public API request. rawKey is the X-API-Key (or Bearer)
// value, clientIP is used for rate limiting when keys are not required.
func (s *APIKeyService) Authorize(rawKey, clientIP string) error {
	rawKey = strings.TrimSpace(rawKey)
	s.mu.RLock()
	settings := s.settings
	keyID, keyOK := s.keysByHash[hashAPIKey(rawKey)]
	s.mu.RUnlock()

	limiterKey := "ip:" + clientIP
	switch {
	case rawKey != "" && keyOK:
		limiterKey = "key:" + strconv.FormatInt(keyID, 10)
		s.touch(keyID)
	case settings.RequireAPIKey && rawKey == "":
		return ErrAPIKeyMissing
	case settings.RequireAPIKey || rawKey != "":
		// A wrong key is rejected even when keys are optional.
		return ErrAPIKeyInvalid
	}

	if settings.RateLimitPerMinute > 0 && !s.limiter(limiterKey, settings.RateLimitPerMinute).Allow() {
		return ErrRateLimited
	}
	return nil
}

func (s *APIKeyService) limiter(key string, perMinute int) *rate.Limiter {
	s.mu.Lock()
	defer s.mu.Unlock()
	l, ok := s.limiters[key]
	if !ok {
		if len(s.limiters) > 10000 {
			// Bound memory under many client IPs; limits restart from full.
			s.limiters = map[string]*rate.Limiter{}
		}
		burst := max(perMinute/4, 10)
		l = rate.NewLimiter(rate.Limit(float64(perMinute)/60), burst)
		s.limiters[key] = l
	}
	return l
}

// touch records last use at most once per apiKeyTouchInterval per key.
func (s *APIKeyService) touch(id int64) {
	now := s.now()
	s.mu.Lock()
	if now.Sub(s.lastTouch[id]) < apiKeyTouchInterval {
		s.mu.Unlock()
		return
	}
	s.lastTouch[id] = now
	s.mu.Unlock()
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := s.repo.TouchAPIKey(ctx, id); err != nil {
			s.log.Warn("recording api key use failed", slog.Int64("key_id", id), slog.Any("err", err))
		}
	}()
}

func (s *APIKeyService) List(ctx context.Context) ([]APIKeyResponse, error) {
	keys, err := s.repo.ListAPIKeys(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]APIKeyResponse, 0, len(keys))
	for _, k := range keys {
		out = append(out, toAPIKeyResponse(k))
	}
	return out, nil
}

func (s *APIKeyService) Create(ctx context.Context, name string) (*APIKeySecretResponse, error) {
	name, err := validateAPIKeyName(name)
	if err != nil {
		return nil, err
	}
	key, prefix, hash, err := generateAPIKey()
	if err != nil {
		return nil, err
	}
	rec, err := s.repo.CreateAPIKey(ctx, name, prefix, hash)
	if err != nil {
		return nil, err
	}
	if err := s.reloadKeys(ctx); err != nil {
		return nil, err
	}
	s.log.Info("api key created", slog.Int64("key_id", rec.ID), slog.String("name", name), slog.String("prefix", prefix))
	return &APIKeySecretResponse{APIKeyResponse: toAPIKeyResponse(rec), Key: key}, nil
}

// Rotate replaces a key's secret; the old secret stops working at once.
// Returns sql.ErrNoRows if the key does not exist.
func (s *APIKeyService) Rotate(ctx context.Context, id int64) (*APIKeySecretResponse, error) {
	key, prefix, hash, err := generateAPIKey()
	if err != nil {
		return nil, err
	}
	rec, err := s.repo.RotateAPIKey(ctx, id, prefix, hash)
	if err != nil {
		return nil, err
	}
	if err := s.reloadKeys(ctx); err != nil {
		return nil, err
	}
	s.log.Info("api key rotated", slog.Int64("key_id", id), slog.String("prefix", prefix))
	return &APIKeySecretResponse{APIKeyResponse: toAPIKeyResponse(rec), Key: key}, nil
}

// Rename returns sql.ErrNoRows if the key does not exist.
func (s *APIKeyService) Rename(ctx context.Context, id int64, name string) (*APIKeyResponse, error) {
	name, err := validateAPIKeyName(name)
	if err != nil {
		return nil, err
	}
	rec, err := s.repo.RenameAPIKey(ctx, id, name)
	if err != nil {
		return nil, err
	}
	resp := toAPIKeyResponse(rec)
	return &resp, nil
}

// Delete revokes a key; sql.ErrNoRows if it does not exist.
func (s *APIKeyService) Delete(ctx context.Context, id int64) error {
	ok, err := s.repo.DeleteAPIKey(ctx, id)
	if err != nil {
		return err
	}
	if !ok {
		return sql.ErrNoRows
	}
	s.log.Info("api key revoked", slog.Int64("key_id", id))
	return s.reloadKeys(ctx)
}

func (s *APIKeyService) Settings() PublicAPISettings {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.settings
}

// UpdateSettingsInput: nil fields are left unchanged.
type UpdateSettingsInput struct {
	RequireAPIKey      *bool `json:"require_api_key"`
	RateLimitPerMinute *int  `json:"rate_limit_per_minute"`
}

func (s *APIKeyService) UpdateSettings(ctx context.Context, in UpdateSettingsInput) (PublicAPISettings, error) {
	if in.RateLimitPerMinute != nil && (*in.RateLimitPerMinute < 0 || *in.RateLimitPerMinute > 100000) {
		return PublicAPISettings{}, &ValidationError{Message: "rate_limit_per_minute must be between 0 (off) and 100000"}
	}
	if in.RequireAPIKey != nil {
		if err := s.repo.UpsertAppSetting(ctx, settingRequireAPIKey, strconv.FormatBool(*in.RequireAPIKey)); err != nil {
			return PublicAPISettings{}, err
		}
	}
	if in.RateLimitPerMinute != nil {
		if err := s.repo.UpsertAppSetting(ctx, settingRateLimit, strconv.Itoa(*in.RateLimitPerMinute)); err != nil {
			return PublicAPISettings{}, err
		}
	}
	if err := s.reloadSettings(ctx); err != nil {
		return PublicAPISettings{}, err
	}
	settings := s.Settings()
	s.log.Info("public api settings updated", slog.Bool("require_api_key", settings.RequireAPIKey), slog.Int("rate_limit_per_minute", settings.RateLimitPerMinute))
	return settings, nil
}

func validateAPIKeyName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 100 {
		return "", &ValidationError{Message: "name is required (at most 100 characters)"}
	}
	return name, nil
}

// generateAPIKey returns a new key ("mr_" + 43 base64url chars, 256 bits),
// its display prefix and its hash.
func generateAPIKey() (key, prefix, hash string, err error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", "", "", fmt.Errorf("generate api key: %w", err)
	}
	key = apiKeyPrefix + base64.RawURLEncoding.EncodeToString(b)
	return key, key[:apiKeyDisplayLen], hashAPIKey(key), nil
}

// API keys are 256-bit random values, so a slow, salted-per-key hash isn't
// needed against brute force; a deterministic PBKDF2 (fixed salt, modest
// iterations, ~0.4ms) keeps lookups by digest possible while using a proper
// KDF rather than a bare hash.
var apiKeyHashSalt = []byte("metareel/api-key/v1")

const apiKeyHashIterations = 4096

// hashAPIKey returns the hex digest stored for a key ("" if hashing fails,
// which matches no stored key).
func hashAPIKey(key string) string {
	sum, err := pbkdf2.Key(sha256.New, key, apiKeyHashSalt, apiKeyHashIterations, 32)
	if err != nil {
		return ""
	}
	return hex.EncodeToString(sum)
}

func toAPIKeyResponse(k repository.APIKeyRecord) APIKeyResponse {
	return APIKeyResponse{ID: k.ID, Name: k.Name, Prefix: k.KeyPrefix + "…", CreatedAt: k.CreatedAt, RotatedAt: k.RotatedAt, LastUsedAt: k.LastUsedAt}
}
