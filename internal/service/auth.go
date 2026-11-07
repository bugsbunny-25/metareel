package service

import (
	"context"
	"errors"
	"strconv"
	"strings"
)

// Principal kinds.
const (
	PrincipalAPIKey    = "api_key"
	PrincipalUser      = "user"
	PrincipalAnonymous = "anonymous" // no credentials, allowed when keys are optional
)

// Principal is who a public API request is made by.
type Principal struct {
	Kind string `json:"kind"`
	ID   int64  `json:"id,omitempty"`
	Name string `json:"name,omitempty"` // API key name or username
	Role string `json:"role,omitempty"` // users only
	// SessionID is the user's session making the request (not returned).
	SessionID int64 `json:"-"`
}

// Credentials are what a request presented. At most one of APIKey and
// SessionToken is used: an API key wins.
type Credentials struct {
	APIKey       string
	SessionToken string
	ClientIP     string
}

// AuthService authenticates public API requests by API key or by a user's
// session, and applies the rate limit per caller.
type AuthService struct {
	keys     *APIKeyService
	sessions *SessionService
}

func NewAuthService(keys *APIKeyService, sessions *SessionService) *AuthService {
	return &AuthService{keys: keys, sessions: sessions}
}

// Authenticate returns the request's principal. A credential that is
// presented must be valid, even when the API is open to anonymous callers.
// Errors: ErrAPIKeyMissing, ErrAPIKeyInvalid, ErrSessionInvalid,
// ErrRateLimited, or a database error.
func (s *AuthService) Authenticate(ctx context.Context, cred Credentials) (Principal, error) {
	apiKey := strings.TrimSpace(cred.APIKey)
	token := strings.TrimSpace(cred.SessionToken)

	var p Principal
	var limiterKey string
	switch {
	case apiKey != "":
		k, ok := s.keys.lookupKey(apiKey)
		if !ok {
			return Principal{}, ErrAPIKeyInvalid
		}
		p = Principal{Kind: PrincipalAPIKey, ID: k.id, Name: k.name}
		limiterKey = "key:" + strconv.FormatInt(k.id, 10)
	case token != "":
		if s.sessions == nil {
			return Principal{}, ErrSessionInvalid
		}
		sess, err := s.sessions.Lookup(ctx, token)
		if err != nil {
			return Principal{}, err
		}
		p = Principal{Kind: PrincipalUser, ID: sess.UserID, Name: sess.Username, Role: sess.Role, SessionID: sess.ID}
		limiterKey = "user:" + strconv.FormatInt(sess.UserID, 10)
	case s.keys.Settings().RequireAPIKey:
		return Principal{}, ErrAPIKeyMissing
	default:
		p = Principal{Kind: PrincipalAnonymous}
		limiterKey = "ip:" + cred.ClientIP
	}
	if err := s.keys.allow(limiterKey); err != nil {
		return Principal{}, err
	}
	return p, nil
}

// AuthenticateSession returns the user of a session token, without the
// public rate limit (used by the admin API). Errors: ErrSessionInvalid or a
// database error.
func (s *AuthService) AuthenticateSession(ctx context.Context, token string) (Principal, error) {
	if s.sessions == nil {
		return Principal{}, ErrSessionInvalid
	}
	sess, err := s.sessions.Lookup(ctx, strings.TrimSpace(token))
	if err != nil {
		return Principal{}, err
	}
	return Principal{Kind: PrincipalUser, ID: sess.UserID, Name: sess.Username, Role: sess.Role, SessionID: sess.ID}, nil
}

// IsUnauthorized reports whether err from Authenticate means 401.
func IsUnauthorized(err error) bool {
	return errors.Is(err, ErrAPIKeyMissing) || errors.Is(err, ErrAPIKeyInvalid) || errors.Is(err, ErrSessionInvalid)
}
