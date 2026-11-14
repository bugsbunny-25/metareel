package handler

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/labstack/echo/v5"
)

// GetStatsSummary: GET /api/v1/me/stats/summary?from=&to=
func (h *Handler) GetStatsSummary(c *echo.Context) error {
	out, err := h.Tracking.Summary(c.Request().Context(), userID(c),
		c.QueryParam("from"), c.QueryParam("to"))
	return h.writeTracking(c, http.StatusOK, out, err)
}

// GetStatsCalendar: GET /api/v1/me/stats/calendar?from=&to=
// The day-by-day heatmap and the streaks that come out of it.
func (h *Handler) GetStatsCalendar(c *echo.Context) error {
	out, err := h.Tracking.Calendar(c.Request().Context(), userID(c),
		c.QueryParam("from"), c.QueryParam("to"))
	return h.writeTracking(c, http.StatusOK, out, err)
}

// GetStatsSpend: GET /api/v1/me/stats/spend?from=&to=
func (h *Handler) GetStatsSpend(c *echo.Context) error {
	out, err := h.Tracking.Spend(c.Request().Context(), userID(c),
		c.QueryParam("from"), c.QueryParam("to"))
	return h.writeTracking(c, http.StatusOK, out, err)
}

// GetStatsRatings: GET /api/v1/me/stats/ratings?from=&to=
func (h *Handler) GetStatsRatings(c *echo.Context) error {
	out, err := h.Tracking.Ratings(c.Request().Context(), userID(c),
		c.QueryParam("from"), c.QueryParam("to"))
	return h.writeTracking(c, http.StatusOK, out, err)
}

// GetStatsPeople: GET /api/v1/me/stats/people?job=&limit=&from=&to=
// job is empty for cast, or a crew job such as Director.
func (h *Handler) GetStatsPeople(c *echo.Context) error {
	limit, err := optionalInt(c.QueryParam("limit"), "limit")
	if err != nil {
		return badRequest(c, err)
	}
	out, err := h.Tracking.TopPeople(c.Request().Context(), userID(c),
		c.QueryParam("from"), c.QueryParam("to"), strings.TrimSpace(c.QueryParam("job")), limit)
	return h.writeTracking(c, http.StatusOK, out, err)
}

// GetStatsYear: GET /api/v1/me/stats/year?year=
// Defaults to the current year.
func (h *Handler) GetStatsYear(c *echo.Context) error {
	year := h.Tracking.CurrentYear()
	if raw := strings.TrimSpace(c.QueryParam("year")); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil {
			return c.JSON(http.StatusBadRequest, map[string]any{"error": "year must be a four-digit year"})
		}
		year = n
	}
	out, err := h.Tracking.YearInReview(c.Request().Context(), userID(c), year)
	return h.writeTracking(c, http.StatusOK, out, err)
}

// GetWatchlistInsights: GET /api/v1/me/watchlist/insights?country=&days=
// What the watchlist is doing in metareel's own chart and availability data.
func (h *Handler) GetWatchlistInsights(c *echo.Context) error {
	days, err := optionalInt(c.QueryParam("days"), "days")
	if err != nil {
		return badRequest(c, err)
	}
	out, err := h.Tracking.WatchlistInsights(c.Request().Context(), userID(c),
		strings.TrimSpace(c.QueryParam("country")), days)
	return h.writeTracking(c, http.StatusOK, out, err)
}
