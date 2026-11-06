package handler

import (
	"net/http"

	"github.com/labstack/echo/v5"

	"github.com/bugsbunny-25/metareel/internal/service"
)

// Admin API for outbound webhooks (service/webhooks.go).

// ListWebhooks: GET /api/v1/admin/webhooks
func (h *Handler) ListWebhooks(c *echo.Context) error {
	items, err := h.Webhooks.List(c.Request().Context())
	if err != nil {
		return writeData(c, nil, err)
	}
	return c.JSON(http.StatusOK, map[string]any{"items": items, "event_types": h.Webhooks.EventTypes()})
}

// CreateWebhook: POST /api/v1/admin/webhooks {name, url, events, enabled?}
func (h *Handler) CreateWebhook(c *echo.Context) error {
	var in service.WebhookInput
	if err := c.Bind(&in); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{"error": "invalid JSON body"})
	}
	out, err := h.Webhooks.Create(c.Request().Context(), in)
	if err != nil {
		return writeData(c, nil, err)
	}
	return c.JSON(http.StatusCreated, out)
}

func (h *Handler) webhookID(c *echo.Context) (int64, error) {
	return parseID(c.Param("id"), "webhook id")
}

// UpdateWebhook: PATCH /api/v1/admin/webhooks/{id}
func (h *Handler) UpdateWebhook(c *echo.Context) error {
	id, err := h.webhookID(c)
	if err != nil {
		return badRequest(c, err)
	}
	var p service.WebhookPatch
	if err := c.Bind(&p); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{"error": "invalid JSON body"})
	}
	out, err := h.Webhooks.Update(c.Request().Context(), id, p)
	return writeData(c, out, err)
}

// DeleteWebhook: DELETE /api/v1/admin/webhooks/{id}
func (h *Handler) DeleteWebhook(c *echo.Context) error {
	id, err := h.webhookID(c)
	if err != nil {
		return badRequest(c, err)
	}
	if err := h.Webhooks.Delete(c.Request().Context(), id); err != nil {
		return writeData(c, nil, err)
	}
	return c.NoContent(http.StatusNoContent)
}

// RotateWebhookSecret: POST /api/v1/admin/webhooks/{id}/rotate-secret
func (h *Handler) RotateWebhookSecret(c *echo.Context) error {
	id, err := h.webhookID(c)
	if err != nil {
		return badRequest(c, err)
	}
	out, err := h.Webhooks.RotateSecret(c.Request().Context(), id)
	return writeData(c, out, err)
}

// TestWebhook: POST /api/v1/admin/webhooks/{id}/test
func (h *Handler) TestWebhook(c *echo.Context) error {
	id, err := h.webhookID(c)
	if err != nil {
		return badRequest(c, err)
	}
	out, err := h.Webhooks.Test(c.Request().Context(), id)
	return writeData(c, out, err)
}

// ListWebhookDeliveries: GET /api/v1/admin/webhooks/{id}/deliveries[?limit=]
func (h *Handler) ListWebhookDeliveries(c *echo.Context) error {
	id, err := h.webhookID(c)
	if err != nil {
		return badRequest(c, err)
	}
	limit, err := parseIntParam(c, "limit", 50)
	if err != nil {
		return badRequest(c, err)
	}
	items, err := h.Webhooks.Deliveries(c.Request().Context(), id, limit)
	if err != nil {
		return writeData(c, nil, err)
	}
	return c.JSON(http.StatusOK, map[string]any{"items": items})
}

// GetWebhookDelivery: GET /api/v1/admin/webhook-deliveries/{id} (the JSON body sent)
func (h *Handler) GetWebhookDelivery(c *echo.Context) error {
	id, err := parseID(c.Param("id"), "delivery id")
	if err != nil {
		return badRequest(c, err)
	}
	out, err := h.Webhooks.DeliveryPayload(c.Request().Context(), id)
	return writeData(c, out, err)
}

// RetryWebhookDelivery: POST /api/v1/admin/webhook-deliveries/{id}/retry
func (h *Handler) RetryWebhookDelivery(c *echo.Context) error {
	id, err := parseID(c.Param("id"), "delivery id")
	if err != nil {
		return badRequest(c, err)
	}
	if err := h.Webhooks.Retry(c.Request().Context(), id); err != nil {
		return writeData(c, nil, err)
	}
	return c.NoContent(http.StatusNoContent)
}
