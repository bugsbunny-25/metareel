package handler

import (
	"errors"
	"net/http"
	"strings"

	"github.com/labstack/echo/v5"

	"github.com/bugsbunny-25/metareel/internal/service"
)

// ListUsers: GET /api/v1/admin/users
func (h *Handler) ListUsers(c *echo.Context) error {
	out, err := h.Users.List(c.Request().Context())
	return h.writeUserResponse(c, http.StatusOK, out, err)
}

type createUserRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Role     string `json:"role"`
}

// CreateUser: POST /api/v1/admin/users {"username","password","role"}
func (h *Handler) CreateUser(c *echo.Context) error {
	var req createUserRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{"error": "invalid JSON body"})
	}
	if req.Role == "" {
		req.Role = service.RoleReader
	}
	out, err := h.Users.Create(c.Request().Context(), req.Username, req.Password, req.Role)
	if err == nil {
		h.auditUser(c, "user created", out.Username)
	}
	return h.writeUserResponse(c, http.StatusCreated, out, err)
}

type patchUserRequest struct {
	Role     *string `json:"role"`
	Disabled *bool   `json:"disabled"`
	Password *string `json:"password"`
}

// PatchUser: PATCH /api/v1/admin/users/{username} {"role"?, "disabled"?,
// "password"?}. A new password ends the user's other sessions (all of them
// for someone else). Admins can't demote or disable themselves.
func (h *Handler) PatchUser(c *echo.Context) error {
	username := c.Param("username")
	var req patchUserRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{"error": "invalid JSON body"})
	}
	ctx := c.Request().Context()
	self, me := h.isSelf(c, username)
	if self && ((req.Role != nil && *req.Role != service.RoleAdmin) || (req.Disabled != nil && *req.Disabled)) {
		return c.JSON(http.StatusBadRequest, map[string]any{"error": "you can't demote or disable your own account"})
	}
	if _, err := h.Users.Get(ctx, username); err != nil {
		return h.writeUserResponse(c, 0, nil, err)
	}
	if req.Password != nil {
		keep := int64(0)
		if self {
			keep = me.SessionID
		}
		if err := h.Users.SetPassword(ctx, username, *req.Password, keep); err != nil {
			return h.writeUserResponse(c, 0, nil, err)
		}
		h.auditUser(c, "user password changed", username)
	}
	if req.Role != nil {
		if err := h.Users.SetRole(ctx, username, *req.Role); err != nil {
			return h.writeUserResponse(c, 0, nil, err)
		}
		h.auditUser(c, "user role changed", username, "role", *req.Role)
	}
	if req.Disabled != nil {
		if err := h.Users.SetDisabled(ctx, username, *req.Disabled); err != nil {
			return h.writeUserResponse(c, 0, nil, err)
		}
		h.auditUser(c, "user disabled changed", username, "disabled", *req.Disabled)
	}
	out, err := h.Users.Get(ctx, username)
	return h.writeUserResponse(c, http.StatusOK, out, err)
}

// DeleteUser: DELETE /api/v1/admin/users/{username}
func (h *Handler) DeleteUser(c *echo.Context) error {
	username := c.Param("username")
	if self, _ := h.isSelf(c, username); self {
		return c.JSON(http.StatusBadRequest, map[string]any{"error": "you can't delete your own account"})
	}
	if err := h.Users.Delete(c.Request().Context(), username); err != nil {
		return h.writeUserResponse(c, 0, nil, err)
	}
	h.auditUser(c, "user deleted", username)
	return c.NoContent(http.StatusNoContent)
}

// LogoutUser: POST /api/v1/admin/users/{username}/logout ends the user's
// sessions (for yourself: every session but this one).
func (h *Handler) LogoutUser(c *echo.Context) error {
	username := c.Param("username")
	ctx := c.Request().Context()
	if _, err := h.Users.Get(ctx, username); err != nil {
		return h.writeUserResponse(c, 0, nil, err)
	}
	var n int64
	var err error
	if self, me := h.isSelf(c, username); self {
		n, err = h.Sessions.RevokeUserExcept(ctx, username, me.SessionID)
	} else {
		n, err = h.Sessions.RevokeUser(ctx, username)
	}
	if err != nil {
		return h.writeUserResponse(c, 0, nil, err)
	}
	h.auditUser(c, "user signed out", username, "sessions", n)
	return c.JSON(http.StatusOK, map[string]any{"sessions_ended": n})
}

// ListSessions: GET /api/v1/admin/sessions lists active sessions; the
// caller's own is marked current.
func (h *Handler) ListSessions(c *echo.Context) error {
	out, err := h.Sessions.List(c.Request().Context())
	if err != nil {
		return h.writeUserResponse(c, 0, nil, err)
	}
	if me, ok := PrincipalFrom(c); ok {
		for i := range out {
			out[i].Current = out[i].ID == me.SessionID
		}
	}
	return c.JSON(http.StatusOK, out)
}

// RevokeSession: DELETE /api/v1/admin/sessions/{id}
func (h *Handler) RevokeSession(c *echo.Context) error {
	id, err := parseID(c.Param("id"), "session id")
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{"error": err.Error()})
	}
	if err := h.Sessions.Revoke(c.Request().Context(), id); err != nil {
		return h.writeUserResponse(c, 0, nil, err)
	}
	h.auditUser(c, "session revoked", "", "session_id", id)
	return c.NoContent(http.StatusNoContent)
}

// isSelf reports whether username is the signed-in caller.
func (h *Handler) isSelf(c *echo.Context, username string) (bool, service.Principal) {
	me, ok := PrincipalFrom(c)
	return ok && me.Kind == service.PrincipalUser && strings.EqualFold(me.Name, username), me
}

// auditUser logs an admin's change to users or sessions with who made it.
func (h *Handler) auditUser(c *echo.Context, msg, username string, attrs ...any) {
	actor := "unauthenticated admin API"
	if me, ok := PrincipalFrom(c); ok {
		actor = me.Name
	}
	args := append([]any{"actor", actor, "ip", c.RealIP()}, attrs...)
	if username != "" {
		args = append(args, "username", username)
	}
	h.logger().Info(msg, args...)
}

func (h *Handler) writeUserResponse(c *echo.Context, status int, out any, err error) error {
	var vErr *service.ValidationError
	switch {
	case err == nil:
		return c.JSON(status, out)
	case errors.As(err, &vErr):
		return c.JSON(http.StatusBadRequest, map[string]any{"error": vErr.Message})
	case errors.Is(err, service.ErrUserNotFound), errors.Is(err, service.ErrSessionNotFound):
		return c.JSON(http.StatusNotFound, map[string]any{"error": err.Error()})
	case errors.Is(err, service.ErrUserExists):
		return c.JSON(http.StatusConflict, map[string]any{"error": err.Error()})
	default:
		h.logger().Error("user admin", "err", err)
		return c.JSON(http.StatusInternalServerError, map[string]any{"error": "internal server error"})
	}
}
