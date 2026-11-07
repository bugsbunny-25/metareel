package handler

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v5"

	"github.com/bugsbunny-25/metareel/internal/service"
)

const (
	// SessionCookie holds a session token for browsers.
	SessionCookie = "metareel_session"
	principalKey  = "metareel.principal"
)

// Authenticate guards the public API. A request authenticates with one of:
//   - an API key in X-API-Key, or as "Authorization: Bearer mr_…";
//   - a session token from POST /api/v1/auth/login as
//     "Authorization: Bearer mrs_…", or the metareel_session cookie.
//
// Header credentials win over the cookie, and a credential that is sent must
// be valid. Without one the request passes only when API keys are optional.
// The rate limit applies per key, per user, or per client IP.
func (h *Handler) Authenticate(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c *echo.Context) error {
		p, err := h.Auth.Authenticate(c.Request().Context(), requestCredentials(c))
		switch {
		case err == nil:
			c.Set(principalKey, p)
			return next(c)
		case errors.Is(err, service.ErrRateLimited):
			c.Response().Header().Set("Retry-After", "60")
			return c.JSON(http.StatusTooManyRequests, map[string]any{"error": err.Error()})
		case service.IsUnauthorized(err):
			c.Response().Header().Add("WWW-Authenticate", `ApiKey header="X-API-Key"`)
			c.Response().Header().Add("WWW-Authenticate", `Bearer realm="metareel"`)
			return c.JSON(http.StatusUnauthorized, map[string]any{"error": err.Error()})
		default:
			h.logger().Error("authenticate request", "err", err)
			return c.JSON(http.StatusInternalServerError, map[string]any{"error": "internal server error"})
		}
	}
}

// AdminAuth guards /api/v1/admin. With AdminLoginRequired only signed-in
// users with the admin role pass (session cookie or "Bearer mrs_…"; API keys
// never open the admin API): no session → 401, other roles → 403. Without it
// the admin API stays open, but a valid session still identifies the caller.
func (h *Handler) AdminAuth(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c *echo.Context) error {
		r := c.Request()
		signedIn := false
		if token := sessionToken(r); token != "" && h.Auth != nil {
			p, err := h.Auth.AuthenticateSession(r.Context(), token)
			switch {
			case err == nil:
				c.Set(principalKey, p)
				signedIn = true
			case errors.Is(err, service.ErrSessionInvalid):
			default:
				h.logger().Error("authenticate admin request", "err", err)
				return c.JSON(http.StatusInternalServerError, map[string]any{"error": "internal server error"})
			}
		}
		if !h.AdminLoginRequired {
			return next(c)
		}
		if !signedIn {
			c.Response().Header().Set("WWW-Authenticate", `Bearer realm="metareel admin"`)
			return c.JSON(http.StatusUnauthorized, map[string]any{"error": "sign in to use the admin console"})
		}
		if p, _ := PrincipalFrom(c); p.Role != service.RoleAdmin {
			return c.JSON(http.StatusForbidden, map[string]any{"error": "the admin console needs the admin role"})
		}
		return next(c)
	}
}

// AuthSession: GET /api/v1/auth/session tells the admin console whether it
// needs a sign-in and who is signed in (user is null otherwise). Never 401.
func (h *Handler) AuthSession(c *echo.Context) error {
	out := map[string]any{"admin_login_required": h.AdminLoginRequired, "user": nil}
	r := c.Request()
	if token := sessionToken(r); token != "" && h.Auth != nil {
		p, err := h.Auth.AuthenticateSession(r.Context(), token)
		switch {
		case err == nil:
			out["user"] = service.SessionUser{ID: p.ID, Username: p.Name, Role: p.Role}
		case !errors.Is(err, service.ErrSessionInvalid):
			h.logger().Error("auth session", "err", err)
			return c.JSON(http.StatusInternalServerError, map[string]any{"error": "internal server error"})
		}
	}
	c.Response().Header().Set("Cache-Control", "no-store")
	return c.JSON(http.StatusOK, out)
}

// PrincipalFrom returns who made a request that passed Authenticate.
func PrincipalFrom(c *echo.Context) (service.Principal, bool) {
	p, ok := c.Get(principalKey).(service.Principal)
	return p, ok
}

func requestCredentials(c *echo.Context) service.Credentials {
	r := c.Request()
	cred := service.Credentials{ClientIP: c.RealIP()}
	if key := r.Header.Get("X-API-Key"); key != "" {
		cred.APIKey = key
		return cred
	}
	if token := bearerToken(r); token != "" {
		if strings.HasPrefix(token, service.SessionTokenPrefix) {
			cred.SessionToken = token
		} else {
			cred.APIKey = token
		}
		return cred
	}
	if ck, err := r.Cookie(SessionCookie); err == nil {
		cred.SessionToken = ck.Value
	}
	return cred
}

func bearerToken(r *http.Request) string {
	if auth := r.Header.Get("Authorization"); len(auth) > 7 && strings.EqualFold(auth[:7], "bearer ") {
		return strings.TrimSpace(auth[7:])
	}
	return ""
}

// sessionToken is the session token a request carries (Bearer or cookie).
func sessionToken(r *http.Request) string {
	if t := bearerToken(r); strings.HasPrefix(t, service.SessionTokenPrefix) {
		return t
	}
	if ck, err := r.Cookie(SessionCookie); err == nil {
		return ck.Value
	}
	return ""
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// Login: POST /api/v1/auth/login {"username","password"}. Sets the session
// cookie and returns the token once, for non-browser clients.
func (h *Handler) Login(c *echo.Context) error {
	var req loginRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{"error": "invalid JSON body"})
	}
	r := c.Request()
	out, err := h.Sessions.Login(r.Context(), req.Username, req.Password, r.UserAgent(), c.RealIP())
	var vErr *service.ValidationError
	switch {
	case err == nil:
	case errors.As(err, &vErr):
		return c.JSON(http.StatusBadRequest, map[string]any{"error": vErr.Error()})
	case errors.Is(err, service.ErrInvalidLogin):
		return c.JSON(http.StatusUnauthorized, map[string]any{"error": err.Error()})
	case errors.Is(err, service.ErrLoginRateLimited):
		c.Response().Header().Set("Retry-After", "60")
		return c.JSON(http.StatusTooManyRequests, map[string]any{"error": err.Error()})
	default:
		h.logger().Error("login", "err", err)
		return c.JSON(http.StatusInternalServerError, map[string]any{"error": "internal server error"})
	}
	c.SetCookie(sessionCookie(r, out.Token, out.ExpiresAt))
	c.Response().Header().Set("Cache-Control", "no-store")
	return c.JSON(http.StatusOK, out)
}

// Logout: POST /api/v1/auth/logout ends the request's session (Bearer token
// or cookie) and clears the cookie. Always 204.
func (h *Handler) Logout(c *echo.Context) error {
	r := c.Request()
	if token := sessionToken(r); token != "" {
		if err := h.Sessions.Logout(r.Context(), token); err != nil {
			h.logger().Error("logout", "err", err)
			return c.JSON(http.StatusInternalServerError, map[string]any{"error": "internal server error"})
		}
	}
	c.SetCookie(sessionCookie(r, "", time.Unix(0, 0)))
	return c.NoContent(http.StatusNoContent)
}

// Me: GET /api/v1/auth/me returns the caller (user or API key); 401 when
// there are no credentials.
func (h *Handler) Me(c *echo.Context) error {
	p, ok := PrincipalFrom(c)
	if !ok || p.Kind == service.PrincipalAnonymous {
		c.Response().Header().Add("WWW-Authenticate", `Bearer realm="metareel"`)
		return c.JSON(http.StatusUnauthorized, map[string]any{"error": "not signed in"})
	}
	return c.JSON(http.StatusOK, p)
}

// sessionCookie is HttpOnly and SameSite=Strict (never sent by other sites);
// Secure when the request came over HTTPS (directly or via a proxy), so it
// still works on a plain-HTTP LAN. A zero-time expiry deletes it.
func sessionCookie(r *http.Request, value string, expires time.Time) *http.Cookie {
	ck := &http.Cookie{
		Name:     SessionCookie,
		Value:    value,
		Path:     "/",
		Expires:  expires,
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		Secure:   r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https"),
	}
	if value == "" {
		ck.MaxAge = -1
	}
	return ck
}
