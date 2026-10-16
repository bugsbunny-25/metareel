package handler

import (
	"errors"
	"strconv"

	"github.com/labstack/echo/v5"

	"github.com/bugsbunny-25/metareel/internal/service"
)

// ListMappings lists the TMDB titles FlixPatrol and Netflix titles are mapped
// to, with every external ID and the titles of each source:
// GET /api/v1/admin/mappings?kind=movie|tv&q=&flag=&limit=&offset=
func (h *Handler) ListMappings(c *echo.Context) error {
	limit, offset, err := parsePagination(c.QueryParam("limit"), c.QueryParam("offset"))
	if err != nil {
		return badRequest(c, err)
	}
	out, err := h.Mapping.List(c.Request().Context(), service.MappingQuery{
		Kind: c.QueryParam("kind"), Search: c.QueryParam("q"), Flag: c.QueryParam("flag"),
	}, limit, offset)
	return writeData(c, out, err)
}

// GetMapping: GET /api/v1/admin/mappings/{kind}/{id}
func (h *Handler) GetMapping(c *echo.Context) error {
	ref, err := h.parseRef(c)
	if err != nil {
		return writeData(c, nil, err)
	}
	out, err := h.Mapping.Get(c.Request().Context(), ref)
	return writeData(c, out, err)
}

// RemapMapping moves every title mapped to a TMDB title to another one:
// POST /api/v1/admin/mappings/{kind}/{id}/remap {"tmdb_kind": "movie|tv", "tmdb_id": "..."}
func (h *Handler) RemapMapping(c *echo.Context) error {
	ref, err := h.parseRef(c)
	if err != nil {
		return writeData(c, nil, err)
	}
	var in service.RemapInput
	if err := c.Bind(&in); err != nil {
		return badRequest(c, errors.New("invalid JSON body"))
	}
	out, err := h.Mapping.Remap(c.Request().Context(), ref, in)
	return writeData(c, out, err)
}

// ListMappingEvents lists the history of mapping changes, newest first:
// GET /api/v1/admin/mapping-events?subject=flixpatrol:12|netflix:34&tmdb=movie:100&limit=50
func (h *Handler) ListMappingEvents(c *echo.Context) error {
	var limit int64
	if v := c.QueryParam("limit"); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil || n < 1 {
			return badRequest(c, errors.New("limit must be a positive integer"))
		}
		limit = n
	}
	out, err := h.Mapping.Events(c.Request().Context(), service.MappingEventsQuery{
		Subject: c.QueryParam("subject"), Tmdb: c.QueryParam("tmdb"), Limit: limit,
	})
	return writeData(c, out, err)
}

// ListMappingRefs returns the mapping reference: one row per chart title
// with its TMDB title and every external ID:
// GET /api/v1/admin/mapping-refs?tmdb_kind=movie|tv&mapped=true
func (h *Handler) ListMappingRefs(c *echo.Context) error {
	mapped := false
	if v := c.QueryParam("mapped"); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return badRequest(c, errors.New("mapped must be true or false"))
		}
		mapped = b
	}
	out, err := h.Mapping.Refs(c.Request().Context(), c.QueryParam("tmdb_kind"), mapped)
	return writeData(c, out, err)
}

// SyncMappings recomputes the links between Netflix and FlixPatrol titles
// and shares mappings over them now: POST /api/v1/admin/mappings/sync
func (h *Handler) SyncMappings(c *echo.Context) error {
	out, err := h.Mapping.Sync(c.Request().Context(), true, nil)
	return writeData(c, out, err)
}
