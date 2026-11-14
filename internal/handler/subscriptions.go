package handler

import (
	"net/http"

	"github.com/labstack/echo/v5"

	"github.com/bugsbunny-25/metareel/internal/service"
)

// ListSubscriptions: GET /api/v1/me/subscriptions
func (h *Handler) ListSubscriptions(c *echo.Context) error {
	out, err := h.Tracking.ListSubscriptions(c.Request().Context(), userID(c))
	return h.writeTracking(c, http.StatusOK, out, err)
}

// CreateSubscription: POST /api/v1/me/subscriptions
func (h *Handler) CreateSubscription(c *echo.Context) error {
	var req service.SubscriptionInput
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{"error": "invalid JSON body"})
	}
	out, err := h.Tracking.CreateSubscription(c.Request().Context(), userID(c), req)
	return h.writeTracking(c, http.StatusCreated, out, err)
}

// UpdateSubscription: PUT /api/v1/me/subscriptions/{id}
func (h *Handler) UpdateSubscription(c *echo.Context) error {
	id, err := parseID(c.Param("id"), "subscription id")
	if err != nil {
		return badRequest(c, err)
	}
	var req service.SubscriptionInput
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{"error": "invalid JSON body"})
	}
	out, err := h.Tracking.UpdateSubscription(c.Request().Context(), userID(c), id, req)
	return h.writeTracking(c, http.StatusOK, out, err)
}

// DeleteSubscription: DELETE /api/v1/me/subscriptions/{id}
func (h *Handler) DeleteSubscription(c *echo.Context) error {
	id, err := parseID(c.Param("id"), "subscription id")
	if err != nil {
		return badRequest(c, err)
	}
	if err := h.Tracking.DeleteSubscription(c.Request().Context(), userID(c), id); err != nil {
		return h.writeTracking(c, http.StatusOK, nil, err)
	}
	return c.NoContent(http.StatusNoContent)
}

// GetSubscriptionSavings: GET /api/v1/me/subscriptions/savings
func (h *Handler) GetSubscriptionSavings(c *echo.Context) error {
	out, err := h.Tracking.Savings(c.Request().Context(), userID(c))
	return h.writeTracking(c, http.StatusOK, out, err)
}
