package handler

import (
	"errors"
	"net/http"

	"github.com/labstack/echo/v5"

	"github.com/bugsbunny-25/metareel/internal/service"
)

// The signed-in user's own account: who they are, where they're signed in,
// their password and their preferences. All on the /api/v1/me group, so a
// session is required and API keys are refused.

// GetAccount: GET /api/v1/me/account
func (h *Handler) GetAccount(c *echo.Context) error {
	me, _ := PrincipalFrom(c)
	ctx := c.Request().Context()
	user, err := h.Users.Get(ctx, me.Name)
	if err != nil {
		return h.writeAccount(c, http.StatusOK, nil, err)
	}
	sessions, err := h.Sessions.ListForUser(ctx, me.ID, me.SessionID)
	return h.writeAccount(c, http.StatusOK, map[string]any{"user": user, "sessions": sessions}, err)
}

type changePasswordRequest struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

// ChangePassword: POST /api/v1/me/password {"current_password","new_password"}
// Ends every other session of the user; this one stays signed in.
func (h *Handler) ChangePassword(c *echo.Context) error {
	var req changePasswordRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{"error": "invalid JSON body"})
	}
	me, _ := PrincipalFrom(c)
	n, err := h.Users.ChangeOwnPassword(c.Request().Context(), me.Name, req.CurrentPassword, req.NewPassword, c.RealIP(), me.SessionID)
	return h.writeAccount(c, http.StatusOK, map[string]any{"other_sessions_ended": n}, err)
}

// RevokeMySession: DELETE /api/v1/me/sessions/{id}
// Signs one of the user's other devices out. The current session signs out
// through POST /api/v1/auth/logout, which also clears the cookie.
func (h *Handler) RevokeMySession(c *echo.Context) error {
	id, err := parseID(c.Param("id"), "session id")
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{"error": err.Error()})
	}
	me, _ := PrincipalFrom(c)
	if id == me.SessionID {
		return c.JSON(http.StatusBadRequest, map[string]any{"error": "that's this session; sign out instead"})
	}
	if err := h.Sessions.RevokeForUser(c.Request().Context(), me.ID, id); err != nil {
		return h.writeAccount(c, 0, nil, err)
	}
	return c.NoContent(http.StatusNoContent)
}

// RevokeMyOtherSessions: DELETE /api/v1/me/sessions
// Signs the user out everywhere except here.
func (h *Handler) RevokeMyOtherSessions(c *echo.Context) error {
	me, _ := PrincipalFrom(c)
	n, err := h.Sessions.RevokeUserExcept(c.Request().Context(), me.Name, me.SessionID)
	return h.writeAccount(c, http.StatusOK, map[string]any{"ended": n}, err)
}

// GetPreferences: GET /api/v1/me/preferences
func (h *Handler) GetPreferences(c *echo.Context) error {
	out, err := h.Tracking.Preferences(c.Request().Context(), userID(c))
	return h.writeTracking(c, http.StatusOK, out, err)
}

// SetPreferences: PUT /api/v1/me/preferences — replaces them; omitted fields are cleared.
func (h *Handler) SetPreferences(c *echo.Context) error {
	var req service.Preferences
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{"error": "invalid JSON body"})
	}
	out, err := h.Tracking.SetPreferences(c.Request().Context(), userID(c), req)
	return h.writeTracking(c, http.StatusOK, out, err)
}

func (h *Handler) writeAccount(c *echo.Context, status int, out any, err error) error {
	if errors.Is(err, service.ErrLoginRateLimited) {
		return c.JSON(http.StatusTooManyRequests, map[string]any{"error": err.Error()})
	}
	return h.writeUserResponse(c, status, out, err)
}
