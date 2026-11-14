package handler

import (
	"net/http"

	"github.com/labstack/echo/v5"

	"github.com/bugsbunny-25/metareel/internal/service"
)

// Devices registered for push notifications (the mobile app). Tokens go in
// and are never returned or logged.

// ListDevices: GET /api/v1/me/devices
func (h *Handler) ListDevices(c *echo.Context) error {
	devices, err := h.Push.Devices(c.Request().Context(), userID(c))
	return h.writeTracking(c, http.StatusOK, map[string]any{"devices": devices, "push_enabled": h.Push.Enabled()}, err)
}

// RegisterDevice: PUT /api/v1/me/devices
// Registers (or refreshes) this device's push token for the signed-in
// session; signing out of that session removes it.
func (h *Handler) RegisterDevice(c *echo.Context) error {
	var req service.DeviceInput
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{"error": "invalid JSON body"})
	}
	me, _ := PrincipalFrom(c)
	var session *int64
	if me.SessionID != 0 {
		session = &me.SessionID
	}
	device, err := h.Push.RegisterDevice(c.Request().Context(), me.ID, session, req)
	return h.writeTracking(c, http.StatusOK, map[string]any{"device": device, "push_enabled": h.Push.Enabled()}, err)
}

// DeleteDevice: DELETE /api/v1/me/devices/{id}
func (h *Handler) DeleteDevice(c *echo.Context) error {
	id, err := parseID(c.Param("id"), "device id")
	if err != nil {
		return badRequest(c, err)
	}
	if err := h.Push.DeleteDevice(c.Request().Context(), userID(c), id); err != nil {
		return h.writeTracking(c, 0, nil, err)
	}
	return c.NoContent(http.StatusNoContent)
}
