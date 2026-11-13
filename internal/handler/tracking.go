package handler

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/labstack/echo/v5"

	"github.com/bugsbunny-25/metareel/internal/service"
)

// RequireUser guards the tracking API: it needs a signed-in user, so an API
// key or an anonymous caller is refused. Tracking data belongs to a person,
// and an API key identifies an integration, not one.
//
// It runs after Authenticate, which has already put the principal on the
// context and applied the rate limit.
func (h *Handler) RequireUser(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c *echo.Context) error {
		p, ok := PrincipalFrom(c)
		if !ok || p.Kind != service.PrincipalUser || p.ID == 0 {
			c.Response().Header().Add("WWW-Authenticate", `Bearer realm="metareel"`)
			return c.JSON(http.StatusUnauthorized, map[string]any{
				"error": "sign in to use the tracking API; API keys cannot act for a user",
			})
		}
		return next(c)
	}
}

// NoStore keeps per-user responses out of every cache. The tracking routes
// are deliberately outside registerReadRoutes, which wraps the shared data
// routes in ETag: a cached or revalidated tracking response could otherwise
// be served to the wrong user.
func NoStore(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c *echo.Context) error {
		h := c.Response().Header()
		h.Set("Cache-Control", "private, no-store")
		h.Set("Vary", "Authorization, Cookie")
		return next(c)
	}
}

// userID is the signed-in caller. RequireUser has already refused anyone else.
func userID(c *echo.Context) int64 {
	p, _ := PrincipalFrom(c)
	return p.ID
}

// writeTracking writes out as JSON with status, or maps err like writeData
// and adds the tracking-specific not-found cases.
func (h *Handler) writeTracking(c *echo.Context, status int, out any, err error) error {
	if err == nil {
		return c.JSON(status, out)
	}
	if errors.Is(err, service.ErrTrackedTitleUnknown) {
		return c.JSON(http.StatusNotFound, map[string]any{"error": err.Error()})
	}
	return writeData(c, out, err)
}

// SearchTrackedTitles: GET /api/v1/me/titles/search?q=&kind=&limit=
func (h *Handler) SearchTrackedTitles(c *echo.Context) error {
	limit, err := optionalInt(c.QueryParam("limit"), "limit")
	if err != nil {
		return badRequest(c, err)
	}
	out, err := h.Tracking.SearchTitles(c.Request().Context(), c.QueryParam("q"), c.QueryParam("kind"), int(limit))
	return h.writeTracking(c, http.StatusOK, out, err)
}

// ListTrackedTitles: GET /api/v1/me/titles?status=&kind=&favorite=&sort=&limit=&offset=
func (h *Handler) ListTrackedTitles(c *echo.Context) error {
	in, err := listTitlesInput(c)
	if err != nil {
		return badRequest(c, err)
	}
	out, err := h.Tracking.ListTitles(c.Request().Context(), userID(c), in)
	return h.writeTracking(c, http.StatusOK, out, err)
}

// GetWatchlist: GET /api/v1/me/watchlist?kind=&sort=&limit=&offset=
func (h *Handler) GetWatchlist(c *echo.Context) error {
	in, err := listTitlesInput(c)
	if err != nil {
		return badRequest(c, err)
	}
	in.Status = service.StatusWatchlist
	out, err := h.Tracking.ListTitles(c.Request().Context(), userID(c), in)
	return h.writeTracking(c, http.StatusOK, out, err)
}

func listTitlesInput(c *echo.Context) (service.ListTitlesInput, error) {
	limit, err := optionalInt(c.QueryParam("limit"), "limit")
	if err != nil {
		return service.ListTitlesInput{}, err
	}
	offset, err := optionalInt(c.QueryParam("offset"), "offset")
	if err != nil {
		return service.ListTitlesInput{}, err
	}
	return service.ListTitlesInput{
		Status:       c.QueryParam("status"),
		Kind:         c.QueryParam("kind"),
		FavoriteOnly: isTrue(c.QueryParam("favorite")),
		Sort:         c.QueryParam("sort"),
		Limit:        limit,
		Offset:       offset,
	}, nil
}

// GetTrackedTitle: GET /api/v1/me/titles/{kind}/{id}
func (h *Handler) GetTrackedTitle(c *echo.Context) error {
	out, err := h.Tracking.GetTitleState(c.Request().Context(), userID(c), c.Param("kind"), c.Param("id"))
	return h.writeTracking(c, http.StatusOK, out, err)
}

// SetTrackedTitle: PUT /api/v1/me/titles/{kind}/{id}
// {"status"?, "favorite"?, "watchlist_note"?}
func (h *Handler) SetTrackedTitle(c *echo.Context) error {
	var req service.TitleStateInput
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{"error": "invalid JSON body"})
	}
	out, err := h.Tracking.SetTitleState(c.Request().Context(), userID(c), c.Param("kind"), c.Param("id"), req)
	return h.writeTracking(c, http.StatusOK, out, err)
}

// DeleteTrackedTitle: DELETE /api/v1/me/titles/{kind}/{id}. Watches are kept.
func (h *Handler) DeleteTrackedTitle(c *echo.Context) error {
	err := h.Tracking.DeleteTitleState(c.Request().Context(), userID(c), c.Param("kind"), c.Param("id"))
	if err != nil {
		return h.writeTracking(c, http.StatusOK, nil, err)
	}
	return c.NoContent(http.StatusNoContent)
}

// AddToWatchlist: PUT /api/v1/me/watchlist/{kind}/{id} {"watchlist_note"?}
func (h *Handler) AddToWatchlist(c *echo.Context) error {
	var req service.TitleStateInput
	if c.Request().ContentLength > 0 {
		if err := c.Bind(&req); err != nil {
			return c.JSON(http.StatusBadRequest, map[string]any{"error": "invalid JSON body"})
		}
	}
	status := service.StatusWatchlist
	req.Status = &status
	out, err := h.Tracking.SetTitleState(c.Request().Context(), userID(c), c.Param("kind"), c.Param("id"), req)
	return h.writeTracking(c, http.StatusOK, out, err)
}

// RemoveFromWatchlist: DELETE /api/v1/me/watchlist/{kind}/{id}. It clears the
// watchlist status and keeps any rating, favourite flag and watches.
func (h *Handler) RemoveFromWatchlist(c *echo.Context) error {
	none := service.StatusNone
	_, err := h.Tracking.SetTitleState(c.Request().Context(), userID(c), c.Param("kind"), c.Param("id"),
		service.TitleStateInput{Status: &none})
	if err != nil {
		return h.writeTracking(c, http.StatusOK, nil, err)
	}
	return c.NoContent(http.StatusNoContent)
}

// ListWatches: GET /api/v1/me/watches?tmdb_kind=&tmdb_id=&from=&to=&rated=&cursor=&limit=
func (h *Handler) ListWatches(c *echo.Context) error {
	limit, err := optionalInt(c.QueryParam("limit"), "limit")
	if err != nil {
		return badRequest(c, err)
	}
	out, err := h.Tracking.ListWatches(c.Request().Context(), userID(c), service.DiaryInput{
		Kind:      c.QueryParam("tmdb_kind"),
		TmdbID:    c.QueryParam("tmdb_id"),
		From:      c.QueryParam("from"),
		To:        c.QueryParam("to"),
		RatedOnly: isTrue(c.QueryParam("rated")),
		Cursor:    c.QueryParam("cursor"),
		Limit:     limit,
	})
	return h.writeTracking(c, http.StatusOK, out, err)
}

// LogWatch: POST /api/v1/me/watches. Replaying the same client_key returns
// the watch stored the first time, with 200 instead of 201.
func (h *Handler) LogWatch(c *echo.Context) error {
	var req service.WatchInput
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{"error": "invalid JSON body"})
	}
	entry, created, err := h.Tracking.LogWatch(c.Request().Context(), userID(c), req)
	if err != nil {
		return h.writeTracking(c, http.StatusOK, nil, err)
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	return c.JSON(status, entry)
}

// GetWatch: GET /api/v1/me/watches/{id}
func (h *Handler) GetWatch(c *echo.Context) error {
	id, err := parseID(c.Param("id"), "watch id")
	if err != nil {
		return badRequest(c, err)
	}
	entry, err := h.Tracking.GetWatch(c.Request().Context(), userID(c), id)
	return h.writeTracking(c, http.StatusOK, entry, err)
}

// UpdateWatch: PUT /api/v1/me/watches/{id}. The title of a watch cannot be
// changed; delete it and log another.
func (h *Handler) UpdateWatch(c *echo.Context) error {
	id, err := parseID(c.Param("id"), "watch id")
	if err != nil {
		return badRequest(c, err)
	}
	var req service.WatchInput
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{"error": "invalid JSON body"})
	}
	entry, err := h.Tracking.UpdateWatch(c.Request().Context(), userID(c), id, req)
	return h.writeTracking(c, http.StatusOK, entry, err)
}

// DeleteWatch: DELETE /api/v1/me/watches/{id}. Permanent, there is no trash.
func (h *Handler) DeleteWatch(c *echo.Context) error {
	id, err := parseID(c.Param("id"), "watch id")
	if err != nil {
		return badRequest(c, err)
	}
	if err := h.Tracking.DeleteWatch(c.Request().Context(), userID(c), id); err != nil {
		return h.writeTracking(c, http.StatusOK, nil, err)
	}
	return c.NoContent(http.StatusNoContent)
}

// ListWatchTags: GET /api/v1/me/tags
func (h *Handler) ListWatchTags(c *echo.Context) error {
	out, err := h.Tracking.ListTags(c.Request().Context(), userID(c))
	return h.writeTracking(c, http.StatusOK, out, err)
}

// DeleteWatchTag: DELETE /api/v1/me/tags/{id}
func (h *Handler) DeleteWatchTag(c *echo.Context) error {
	id, err := parseID(c.Param("id"), "tag id")
	if err != nil {
		return badRequest(c, err)
	}
	if err := h.Tracking.DeleteTag(c.Request().Context(), userID(c), id); err != nil {
		return h.writeTracking(c, http.StatusOK, nil, err)
	}
	return c.NoContent(http.StatusNoContent)
}

// optionalInt reads an optional non-negative integer query param.
func optionalInt(v, name string) (int64, error) {
	if strings.TrimSpace(v) == "" {
		return 0, nil
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil || n < 0 {
		return 0, errors.New(name + " must be a non-negative integer")
	}
	return n, nil
}

func isTrue(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes":
		return true
	}
	return false
}

// GetShow: GET /api/v1/me/shows/{id}?include_specials=
func (h *Handler) GetShow(c *echo.Context) error {
	out, err := h.Tracking.GetShow(c.Request().Context(), userID(c), c.Param("id"), isTrue(c.QueryParam("include_specials")))
	return h.writeTracking(c, http.StatusOK, out, err)
}

// MarkEpisodes: POST /api/v1/me/shows/{id}/watched
// {"scope":"episode|season|up_to|all","season_number":1,"episode_number":3,...}
func (h *Handler) MarkEpisodes(c *echo.Context) error {
	var req service.MarkEpisodesInput
	if c.Request().ContentLength > 0 {
		if err := c.Bind(&req); err != nil {
			return c.JSON(http.StatusBadRequest, map[string]any{"error": "invalid JSON body"})
		}
	}
	out, err := h.Tracking.MarkEpisodes(c.Request().Context(), userID(c), c.Param("id"), req)
	return h.writeTracking(c, http.StatusOK, out, err)
}

// UnmarkEpisodes: DELETE /api/v1/me/shows/{id}/watched?scope=&season=&episode=
// Deletes every watch of the episodes the scope selects, permanently.
func (h *Handler) UnmarkEpisodes(c *echo.Context) error {
	season, err := optionalInt64Ptr(c.QueryParam("season"), "season")
	if err != nil {
		return badRequest(c, err)
	}
	episode, err := optionalInt64Ptr(c.QueryParam("episode"), "episode")
	if err != nil {
		return badRequest(c, err)
	}
	out, err := h.Tracking.UnmarkEpisodes(c.Request().Context(), userID(c), c.Param("id"),
		c.QueryParam("scope"), season, episode, isTrue(c.QueryParam("include_specials")))
	return h.writeTracking(c, http.StatusOK, out, err)
}

// UpNext: GET /api/v1/me/up-next?limit=
func (h *Handler) UpNext(c *echo.Context) error {
	limit, err := optionalInt(c.QueryParam("limit"), "limit")
	if err != nil {
		return badRequest(c, err)
	}
	out, err := h.Tracking.UpNext(c.Request().Context(), userID(c), limit)
	return h.writeTracking(c, http.StatusOK, out, err)
}

// optionalInt64Ptr reads an optional integer query param, nil when absent.
func optionalInt64Ptr(v, name string) (*int64, error) {
	if strings.TrimSpace(v) == "" {
		return nil, nil
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return nil, errors.New(name + " must be an integer")
	}
	return &n, nil
}
