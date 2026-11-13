package handler

import (
	"net/http"

	"github.com/labstack/echo/v5"

	"github.com/bugsbunny-25/metareel/internal/service"
)

// ListTrackingFormats: GET /api/v1/me/formats
// The premium formats a watch can carry. Retired ones are left out here;
// the admin list shows them.
func (h *Handler) ListTrackingFormats(c *echo.Context) error {
	out, err := h.Tracking.ListFormats(c.Request().Context(), true)
	return h.writeTracking(c, http.StatusOK, out, err)
}

// ListVenues: GET /api/v1/me/venues?q=&country=&include_closed=&limit=&offset=
func (h *Handler) ListVenues(c *echo.Context) error {
	limit, err := optionalInt(c.QueryParam("limit"), "limit")
	if err != nil {
		return badRequest(c, err)
	}
	offset, err := optionalInt(c.QueryParam("offset"), "offset")
	if err != nil {
		return badRequest(c, err)
	}
	out, err := h.Tracking.ListVenues(c.Request().Context(), c.QueryParam("q"), c.QueryParam("country"),
		isTrue(c.QueryParam("include_closed")), limit, offset)
	return h.writeTracking(c, http.StatusOK, out, err)
}

// GetVenue: GET /api/v1/me/venues/{id}
func (h *Handler) GetVenue(c *echo.Context) error {
	id, err := parseID(c.Param("id"), "venue id")
	if err != nil {
		return badRequest(c, err)
	}
	out, err := h.Tracking.GetVenue(c.Request().Context(), id)
	return h.writeTracking(c, http.StatusOK, out, err)
}

// CreateVenue: POST /api/v1/me/venues
func (h *Handler) CreateVenue(c *echo.Context) error {
	var req service.VenueInput
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{"error": "invalid JSON body"})
	}
	out, err := h.Tracking.CreateVenue(c.Request().Context(), userID(c), req)
	return h.writeTracking(c, http.StatusCreated, out, err)
}

// UpdateVenue: PUT /api/v1/me/venues/{id}
func (h *Handler) UpdateVenue(c *echo.Context) error {
	id, err := parseID(c.Param("id"), "venue id")
	if err != nil {
		return badRequest(c, err)
	}
	var req service.VenueInput
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{"error": "invalid JSON body"})
	}
	out, err := h.Tracking.UpdateVenue(c.Request().Context(), id, req)
	return h.writeTracking(c, http.StatusOK, out, err)
}

// DeleteVenue: DELETE /api/v1/me/venues/{id}. A venue watches point at is
// refused with 409; close it instead.
func (h *Handler) DeleteVenue(c *echo.Context) error {
	id, err := parseID(c.Param("id"), "venue id")
	if err != nil {
		return badRequest(c, err)
	}
	if err := h.Tracking.DeleteVenue(c.Request().Context(), id); err != nil {
		return h.writeTracking(c, http.StatusOK, nil, err)
	}
	return c.NoContent(http.StatusNoContent)
}

type venueAliasRequest struct {
	Name     string  `json:"name"`
	FromDate *string `json:"from_date"`
	ToDate   *string `json:"to_date"`
}

// AddVenueAlias: POST /api/v1/me/venues/{id}/aliases
func (h *Handler) AddVenueAlias(c *echo.Context) error {
	id, err := parseID(c.Param("id"), "venue id")
	if err != nil {
		return badRequest(c, err)
	}
	var req venueAliasRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{"error": "invalid JSON body"})
	}
	out, err := h.Tracking.AddVenueAlias(c.Request().Context(), id, req.Name, req.FromDate, req.ToDate)
	return h.writeTracking(c, http.StatusCreated, out, err)
}

// DeleteVenueAlias: DELETE /api/v1/me/venues/{id}/aliases/{alias_id}
func (h *Handler) DeleteVenueAlias(c *echo.Context) error {
	venueID, err := parseID(c.Param("id"), "venue id")
	if err != nil {
		return badRequest(c, err)
	}
	aliasID, err := parseID(c.Param("alias_id"), "alias id")
	if err != nil {
		return badRequest(c, err)
	}
	if err := h.Tracking.DeleteVenueAlias(c.Request().Context(), venueID, aliasID); err != nil {
		return h.writeTracking(c, http.StatusOK, nil, err)
	}
	return c.NoContent(http.StatusNoContent)
}

// CreateScreen: POST /api/v1/me/venues/{id}/screens
func (h *Handler) CreateScreen(c *echo.Context) error {
	id, err := parseID(c.Param("id"), "venue id")
	if err != nil {
		return badRequest(c, err)
	}
	var req service.ScreenInput
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{"error": "invalid JSON body"})
	}
	out, err := h.Tracking.CreateScreen(c.Request().Context(), id, req)
	return h.writeTracking(c, http.StatusCreated, out, err)
}

// UpdateScreen: PUT /api/v1/me/venues/{id}/screens/{screen_id}
func (h *Handler) UpdateScreen(c *echo.Context) error {
	venueID, screenID, err := venueAndScreen(c)
	if err != nil {
		return badRequest(c, err)
	}
	var req service.ScreenInput
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{"error": "invalid JSON body"})
	}
	out, err := h.Tracking.UpdateScreen(c.Request().Context(), venueID, screenID, req)
	return h.writeTracking(c, http.StatusOK, out, err)
}

// DeleteScreen: DELETE /api/v1/me/venues/{id}/screens/{screen_id}
func (h *Handler) DeleteScreen(c *echo.Context) error {
	venueID, screenID, err := venueAndScreen(c)
	if err != nil {
		return badRequest(c, err)
	}
	if err := h.Tracking.DeleteScreen(c.Request().Context(), venueID, screenID); err != nil {
		return h.writeTracking(c, http.StatusOK, nil, err)
	}
	return c.NoContent(http.StatusNoContent)
}

func venueAndScreen(c *echo.Context) (int64, int64, error) {
	venueID, err := parseID(c.Param("id"), "venue id")
	if err != nil {
		return 0, 0, err
	}
	screenID, err := parseID(c.Param("screen_id"), "screen id")
	if err != nil {
		return 0, 0, err
	}
	return venueID, screenID, nil
}

// ListWatchFormatsAdmin: GET /api/v1/admin/watch-formats
// Shows retired formats too, which the user-facing list leaves out.
func (h *Handler) ListWatchFormatsAdmin(c *echo.Context) error {
	out, err := h.Tracking.ListFormats(c.Request().Context(), false)
	return writeData(c, out, err)
}

// CreateWatchFormat: POST /api/v1/admin/watch-formats
func (h *Handler) CreateWatchFormat(c *echo.Context) error {
	var req service.FormatInput
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{"error": "invalid JSON body"})
	}
	out, err := h.Tracking.CreateFormat(c.Request().Context(), req)
	if err != nil {
		return writeData(c, nil, err)
	}
	return c.JSON(http.StatusCreated, out)
}

// UpdateWatchFormat: PATCH /api/v1/admin/watch-formats/{id}
// A format is retired by setting active to false, never deleted: watches
// reference it for good.
func (h *Handler) UpdateWatchFormat(c *echo.Context) error {
	id, err := parseID(c.Param("id"), "format id")
	if err != nil {
		return badRequest(c, err)
	}
	var req service.FormatInput
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{"error": "invalid JSON body"})
	}
	out, err := h.Tracking.UpdateFormat(c.Request().Context(), id, req)
	return writeData(c, out, err)
}
