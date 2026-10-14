package handler

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v5"

	"github.com/bugsbunny-25/metareel/internal/service"
	"github.com/bugsbunny-25/metareel/internal/tasks"
)

// ListNetflixTitles lists Netflix official Top 10 titles and their mapping:
// GET /api/v1/admin/netflix/titles?q=&kind=&match_status=&missing=&sort=&limit=&offset=
func (h *Handler) ListNetflixTitles(c *echo.Context) error {
	limit, offset, err := parsePagination(c.QueryParam("limit"), c.QueryParam("offset"))
	if err != nil {
		return badRequest(c, err)
	}
	out, err := h.Netflix.List(c.Request().Context(), service.NetflixTitlesQuery{
		Kind: c.QueryParam("kind"), Search: c.QueryParam("q"), MatchStatus: c.QueryParam("match_status"),
		Missing: c.QueryParam("missing"), Sort: c.QueryParam("sort"),
	}, limit, offset)
	return writeData(c, out, err)
}

// GetNetflixTitle: GET /api/v1/admin/netflix/titles/{id}
func (h *Handler) GetNetflixTitle(c *echo.Context) error {
	id, err := parseID(c.Param("id"), "netflix title id")
	if err != nil {
		return badRequest(c, err)
	}
	out, err := h.Netflix.Get(c.Request().Context(), id)
	return writeData(c, out, err)
}

// PatchNetflixTitle fixes a Netflix title's TMDB / IMDb IDs by hand:
// PATCH /api/v1/admin/netflix/titles/{id} {"tmdb_id": "...", "imdb_id": "..."}
func (h *Handler) PatchNetflixTitle(c *echo.Context) error {
	id, err := parseID(c.Param("id"), "netflix title id")
	if err != nil {
		return badRequest(c, err)
	}
	var in service.PatchNetflixTitleInput
	if err := c.Bind(&in); err != nil {
		return badRequest(c, errors.New("invalid JSON body"))
	}
	out, err := h.Netflix.Patch(c.Request().Context(), id, in)
	return writeData(c, out, err)
}

// RematchNetflixTitle queues a Netflix title for matching now:
// POST /api/v1/admin/netflix/titles/{id}/rematch {"clear": false}
func (h *Handler) RematchNetflixTitle(c *echo.Context) error {
	id, err := parseID(c.Param("id"), "netflix title id")
	if err != nil {
		return badRequest(c, err)
	}
	var in struct {
		Clear bool `json:"clear"`
	}
	if c.Request().ContentLength != 0 {
		if err := c.Bind(&in); err != nil {
			return badRequest(c, errors.New("invalid JSON body"))
		}
	}
	title, err := h.Netflix.Rematch(c.Request().Context(), id, in.Clear)
	if err != nil {
		return writeData(c, nil, err)
	}
	run, err := h.AdminTasks.RunJob(c.Request().Context(), tasks.TypeNetflixTitlesMatch, service.RunJobInput{TitleIDs: []int64{id}})
	var conflictErr *service.ConflictError
	if err != nil && !errors.As(err, &conflictErr) {
		return writeData(c, nil, err)
	}
	return c.JSON(http.StatusAccepted, map[string]any{"title": title, "job": run})
}

// GetNetflixTitleCandidates suggests TMDB / IMDb mappings for a Netflix title:
// GET /api/v1/admin/netflix/titles/{id}/candidates?name=&year=&kind=
func (h *Handler) GetNetflixTitleCandidates(c *echo.Context) error {
	id, err := parseID(c.Param("id"), "netflix title id")
	if err != nil {
		return badRequest(c, err)
	}
	year := 0
	if raw := c.QueryParam("year"); raw != "" {
		if year, err = strconv.Atoi(raw); err != nil || year < 1800 || year > 2200 {
			return badRequest(c, errors.New("invalid year"))
		}
	}
	out, err := h.Netflix.Candidates(c.Request().Context(), id, service.TitleCandidatesQuery{
		Name: c.QueryParam("name"), Year: year, Kind: c.QueryParam("kind"),
	})
	return writeData(c, out, err)
}

// PatchNetflixSeason fixes a season title's season number by hand, or hands
// it back to automatic mapping:
// PATCH /api/v1/admin/netflix/seasons/{id} {"season_number": 2} | {"auto": true}
func (h *Handler) PatchNetflixSeason(c *echo.Context) error {
	id, err := parseID(c.Param("id"), "season id")
	if err != nil {
		return badRequest(c, err)
	}
	var in service.PatchNetflixSeasonInput
	if err := c.Bind(&in); err != nil {
		return badRequest(c, errors.New("invalid JSON body"))
	}
	out, err := h.Netflix.PatchSeason(c.Request().Context(), id, in)
	return writeData(c, out, err)
}
