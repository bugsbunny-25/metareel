package handler

import (
	"errors"

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

// SyncMappings recomputes the links between Netflix and FlixPatrol titles
// and shares mappings over them now: POST /api/v1/admin/mappings/sync
func (h *Handler) SyncMappings(c *echo.Context) error {
	out, err := h.Mapping.Sync(c.Request().Context(), true, nil)
	return writeData(c, out, err)
}
