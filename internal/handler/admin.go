package handler

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"

	"github.com/labstack/echo/v5"

	"github.com/bugsbunny-25/metareel/internal/service"
)

// GetAdminStats: GET /api/v1/admin/stats
func (h *Handler) GetAdminStats(c *echo.Context) error {
	out, err := h.Stats.GetStats(c.Request().Context())
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]any{"error": "internal server error"})
	}
	return c.JSON(http.StatusOK, out)
}

// ListAPIKeys: GET /api/v1/admin/api-keys
func (h *Handler) ListAPIKeys(c *echo.Context) error {
	out, err := h.APIKeys.List(c.Request().Context())
	return writeAPIKeyResponse(c, http.StatusOK, out, err)
}

type apiKeyNameRequest struct {
	Name string `json:"name"`
}

// CreateAPIKey: POST /api/v1/admin/api-keys {"name": "..."}; the response
// holds the key, which is not shown again.
func (h *Handler) CreateAPIKey(c *echo.Context) error {
	var req apiKeyNameRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{"error": "invalid JSON body"})
	}
	out, err := h.APIKeys.Create(c.Request().Context(), req.Name)
	return writeAPIKeyResponse(c, http.StatusCreated, out, err)
}

// RotateAPIKey: POST /api/v1/admin/api-keys/{id}/rotate
func (h *Handler) RotateAPIKey(c *echo.Context) error {
	id, err := parseID(c.Param("id"), "api key id")
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{"error": err.Error()})
	}
	out, err := h.APIKeys.Rotate(c.Request().Context(), id)
	return writeAPIKeyResponse(c, http.StatusOK, out, err)
}

// RenameAPIKey: PATCH /api/v1/admin/api-keys/{id} {"name": "..."}
func (h *Handler) RenameAPIKey(c *echo.Context) error {
	id, err := parseID(c.Param("id"), "api key id")
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{"error": err.Error()})
	}
	var req apiKeyNameRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{"error": "invalid JSON body"})
	}
	out, err := h.APIKeys.Rename(c.Request().Context(), id, req.Name)
	return writeAPIKeyResponse(c, http.StatusOK, out, err)
}

// DeleteAPIKey: DELETE /api/v1/admin/api-keys/{id}
func (h *Handler) DeleteAPIKey(c *echo.Context) error {
	id, err := parseID(c.Param("id"), "api key id")
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{"error": err.Error()})
	}
	if err := h.APIKeys.Delete(c.Request().Context(), id); err != nil {
		return writeAPIKeyResponse(c, 0, nil, err)
	}
	return c.NoContent(http.StatusNoContent)
}

// GetSettings: GET /api/v1/admin/settings
func (h *Handler) GetSettings(c *echo.Context) error {
	return c.JSON(http.StatusOK, h.APIKeys.Settings())
}

// PatchSettings: PATCH /api/v1/admin/settings
func (h *Handler) PatchSettings(c *echo.Context) error {
	var req service.UpdateSettingsInput
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{"error": "invalid JSON body"})
	}
	out, err := h.APIKeys.UpdateSettings(c.Request().Context(), req)
	return writeAPIKeyResponse(c, http.StatusOK, out, err)
}

func writeAPIKeyResponse(c *echo.Context, status int, out any, err error) error {
	var validationErr *service.ValidationError
	switch {
	case err == nil:
		return c.JSON(status, out)
	case errors.As(err, &validationErr):
		return c.JSON(http.StatusBadRequest, map[string]any{"error": validationErr.Error()})
	case errors.Is(err, sql.ErrNoRows):
		return c.JSON(http.StatusNotFound, map[string]any{"error": "api key not found"})
	default:
		return c.JSON(http.StatusInternalServerError, map[string]any{"error": "internal server error"})
	}
}

// RequireAPIKey guards the public API: it accepts the key from the
// X-API-Key header or an "Authorization: Bearer <key>" header, and applies
// the per-key (or per-IP) rate limit.
func (h *Handler) RequireAPIKey(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c *echo.Context) error {
		key := c.Request().Header.Get("X-API-Key")
		if key == "" {
			if auth := c.Request().Header.Get("Authorization"); len(auth) > 7 && strings.EqualFold(auth[:7], "bearer ") {
				key = auth[7:]
			}
		}
		switch err := h.APIKeys.Authorize(key, c.RealIP()); {
		case err == nil:
			return next(c)
		case errors.Is(err, service.ErrRateLimited):
			c.Response().Header().Set("Retry-After", "60")
			return c.JSON(http.StatusTooManyRequests, map[string]any{"error": err.Error()})
		default:
			c.Response().Header().Set("WWW-Authenticate", `ApiKey header="X-API-Key"`)
			return c.JSON(http.StatusUnauthorized, map[string]any{"error": err.Error()})
		}
	}
}
