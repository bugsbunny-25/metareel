package handler

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/labstack/echo/v5"

	"github.com/bugsbunny-25/metareel/internal/service"
)

// writeData writes out as JSON, or maps err to a status: validation 400,
// conflict 409, not found 404, upstream 502, anything else 500 (with a
// generic message).
func writeData(c *echo.Context, out any, err error) error {
	if err == nil {
		return c.JSON(http.StatusOK, out)
	}
	var validationErr *service.ValidationError
	var conflictErr *service.ConflictError
	switch {
	case errors.As(err, &validationErr):
		return c.JSON(http.StatusBadRequest, map[string]any{"error": validationErr.Error()})
	case errors.As(err, &conflictErr):
		return c.JSON(http.StatusConflict, map[string]any{"error": conflictErr.Error()})
	case errors.Is(err, service.ErrTitleNotFound), errors.Is(err, service.ErrTop10NotFound),
		errors.Is(err, service.ErrNoMetadata):
		return c.JSON(http.StatusNotFound, map[string]any{"error": err.Error()})
	case errors.Is(err, sql.ErrNoRows):
		return c.JSON(http.StatusNotFound, map[string]any{"error": "not found"})
	case errors.Is(err, service.ErrRatingsUnavailable), errors.Is(err, service.ErrUpstream):
		return c.JSON(http.StatusBadGateway, map[string]any{"error": err.Error()})
	default:
		return c.JSON(http.StatusInternalServerError, map[string]any{"error": "internal server error"})
	}
}

func badRequest(c *echo.Context, err error) error {
	return c.JSON(http.StatusBadRequest, map[string]any{"error": err.Error()})
}

// parseRange reads optional from / to query params.
func parseRange(c *echo.Context) (*time.Time, *time.Time, error) {
	from, err := parseOptionalDate("from", c.QueryParam("from"))
	if err != nil {
		return nil, nil, err
	}
	to, err := parseOptionalDate("to", c.QueryParam("to"))
	return from, to, err
}

func parseIntParam(c *echo.Context, name string, def int) (int, error) {
	raw := c.QueryParam(name)
	if raw == "" {
		return def, nil
	}
	v, err := strconv.Atoi(raw)
	if err != nil || v < 0 {
		return 0, errors.New("invalid " + name + ", expected a non-negative integer")
	}
	return v, nil
}

// --- charts ---------------------------------------------------------------------

// GetCountryTop10 returns every provider's movie and TV charts of a country:
// GET /api/v1/top10/{country}[?date=]
func (h *Handler) GetCountryTop10(c *echo.Context) error {
	date, err := parseDate(c.QueryParam("date"))
	if err != nil {
		return badRequest(c, err)
	}
	out, err := h.Top10.GetCountryTop10(c.Request().Context(), service.Top10Query{Date: date, CountryCode: c.Param("country")})
	return writeData(c, out, err)
}

// GetChartHistory returns one chart over a date range:
// GET /api/v1/top10/{movies|tv-shows}/{country}/{provider}/history?from=&to=
func (h *Handler) GetChartHistory(category string) echo.HandlerFunc {
	return func(c *echo.Context) error {
		from, to, err := parseRange(c)
		if err != nil {
			return badRequest(c, err)
		}
		out, err := h.Charts.History(c.Request().Context(), service.HistoryQuery{
			Country: c.Param("country"), Provider: c.Param("provider"), Category: category, From: from, To: to,
		})
		return writeData(c, out, err)
	}
}

// GetGlobalTop10 ranks titles on a provider's charts across every country by
// points: GET /api/v1/top10/global/{provider}[?date=&category=&kind=&limit=]
func (h *Handler) GetGlobalTop10(c *echo.Context) error {
	date, err := parseOptionalDate("date", c.QueryParam("date"))
	if err != nil {
		return badRequest(c, err)
	}
	limit, err := parseIntParam(c, "limit", 10)
	if err != nil {
		return badRequest(c, err)
	}
	if c.Param("provider") == "" {
		return badRequest(c, errors.New("provider is required"))
	}
	out, err := h.Charts.Leaderboard(c.Request().Context(), service.LeaderboardQuery{
		Period: "day", Date: date, Provider: c.Param("provider"), Category: c.QueryParam("category"), Kind: c.QueryParam("kind"), Limit: limit,
	})
	return writeData(c, out, err)
}

// GetCharts lists stored charts: GET /api/v1/charts[?country=&provider=]
func (h *Handler) GetCharts(c *echo.Context) error {
	out, err := h.Charts.Catalog(c.Request().Context(), c.QueryParam("country"), c.QueryParam("provider"))
	return writeData(c, out, err)
}

// GetChartDates lists a chart's stored dates:
// GET /api/v1/charts/{country}/{provider}/dates[?category=&limit=]
func (h *Handler) GetChartDates(c *echo.Context) error {
	limit, err := parseIntParam(c, "limit", 366)
	if err != nil {
		return badRequest(c, err)
	}
	out, err := h.Charts.Dates(c.Request().Context(), c.Param("country"), c.Param("provider"), c.QueryParam("category"), int64(limit))
	return writeData(c, out, err)
}

// GetChanges lists charts stored or re-scraped after since:
// GET /api/v1/changes?since=RFC3339[&limit=]
func (h *Handler) GetChanges(c *echo.Context) error {
	since := time.Now().UTC().Add(-24 * time.Hour)
	if raw := c.QueryParam("since"); raw != "" {
		t, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			return badRequest(c, errors.New("invalid since, expected RFC 3339 (e.g. 2026-10-09T12:00:00Z)"))
		}
		since = t
	}
	limit, err := parseIntParam(c, "limit", 200)
	if err != nil {
		return badRequest(c, err)
	}
	out, err := h.Charts.Changes(c.Request().Context(), since, int64(limit))
	return writeData(c, out, err)
}

// GetMovers returns climbers, fallers, debuts, re-entries and exits:
// GET /api/v1/movers?country=&provider=[&category=&date=]
func (h *Handler) GetMovers(c *echo.Context) error {
	date, err := parseOptionalDate("date", c.QueryParam("date"))
	if err != nil {
		return badRequest(c, err)
	}
	out, err := h.Charts.Movers(c.Request().Context(), service.MoversQuery{
		Country: c.QueryParam("country"), Provider: c.QueryParam("provider"), Category: c.QueryParam("category"), Date: date,
	})
	return writeData(c, out, err)
}

// GetLeaderboard ranks titles by chart points:
// GET /api/v1/leaderboards?period=day|week|month|custom[&date=&from=&to=&country=&provider=&category=&kind=&limit=]
func (h *Handler) GetLeaderboard(c *echo.Context) error {
	date, err := parseOptionalDate("date", c.QueryParam("date"))
	if err != nil {
		return badRequest(c, err)
	}
	from, to, err := parseRange(c)
	if err != nil {
		return badRequest(c, err)
	}
	limit, err := parseIntParam(c, "limit", 50)
	if err != nil {
		return badRequest(c, err)
	}
	out, err := h.Charts.Leaderboard(c.Request().Context(), service.LeaderboardQuery{
		Period: c.QueryParam("period"), Date: date, From: from, To: to, Country: c.QueryParam("country"),
		Provider: c.QueryParam("provider"), Category: c.QueryParam("category"), Kind: c.QueryParam("kind"), Limit: limit,
	})
	return writeData(c, out, err)
}

// ExportRankings streams chart entries as CSV or NDJSON:
// GET /api/v1/export/rankings?from=&to=[&country=&provider=&category=&format=csv|ndjson]
func (h *Handler) ExportRankings(c *echo.Context) error {
	from, to, err := parseRange(c)
	if err != nil {
		return badRequest(c, err)
	}
	format := c.QueryParam("format")
	contentType, err := service.ExportFormat(format)
	if err != nil {
		return writeData(c, nil, err)
	}
	q := service.ExportQuery{From: from, To: to, Country: c.QueryParam("country"), Provider: c.QueryParam("provider"), Category: c.QueryParam("category"), Format: format}
	// Validate before streaming so errors still get a JSON body.
	if err := h.Charts.ValidateExport(q); err != nil {
		return writeData(c, nil, err)
	}
	ext := "csv"
	if contentType != "text/csv; charset=utf-8" {
		ext = "ndjson"
	}
	w := c.Response()
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", `attachment; filename="metareel-rankings.`+ext+`"`)
	w.WriteHeader(http.StatusOK)
	return h.Charts.Export(c.Request().Context(), q, w)
}

// GetCountries lists the countries FlixPatrol charts can be scraped for:
// GET /api/v1/countries
func (h *Handler) GetCountries(c *echo.Context) error {
	return c.JSON(http.StatusOK, h.Charts.Countries())
}

// GetProviders lists known chart providers: GET /api/v1/providers
func (h *Handler) GetProviders(c *echo.Context) error {
	out, err := h.Charts.Providers(c.Request().Context())
	return writeData(c, out, err)
}

// --- titles ---------------------------------------------------------------------

func (h *Handler) parseRef(c *echo.Context) (service.TmdbRef, error) {
	return service.ParseTmdbRef(c.Param("kind"), c.Param("id"))
}

// GetTitleOverview combines metadata, chart stats, current positions,
// ratings, availability and Netflix numbers:
// GET /api/v1/titles/tmdb/{kind}/{id}[?include=&country=]
func (h *Handler) GetTitleOverview(c *echo.Context) error {
	ref, err := h.parseRef(c)
	if err != nil {
		return writeData(c, nil, err)
	}
	include, err := service.ParseIncludes(c.QueryParam("include"))
	if err != nil {
		return writeData(c, nil, err)
	}
	out, err := h.Overview.Overview(c.Request().Context(), ref, service.OverviewQuery{Country: c.QueryParam("country"), Include: include})
	return writeData(c, out, err)
}

// GetTitleStats: GET /api/v1/titles/tmdb/{kind}/{id}/stats[?country=&from=&to=]
func (h *Handler) GetTitleStats(c *echo.Context) error {
	ref, err := h.parseRef(c)
	if err != nil {
		return writeData(c, nil, err)
	}
	q, err := parseRankingHistoryQuery(c)
	if err != nil {
		return badRequest(c, err)
	}
	out, err := h.Overview.Stats(c.Request().Context(), ref, q)
	return writeData(c, out, err)
}

// GetTitleAvailability: GET /api/v1/titles/tmdb/{kind}/{id}/availability[?country=]
func (h *Handler) GetTitleAvailability(c *echo.Context) error {
	ref, err := h.parseRef(c)
	if err != nil {
		return writeData(c, nil, err)
	}
	out, err := h.Overview.Availability(c.Request().Context(), ref, c.QueryParam("country"))
	return writeData(c, out, err)
}

// GetTitleNetflix: GET /api/v1/titles/tmdb/{kind}/{id}/netflix
func (h *Handler) GetTitleNetflix(c *echo.Context) error {
	ref, err := h.parseRef(c)
	if err != nil {
		return writeData(c, nil, err)
	}
	out, err := h.Overview.Netflix(c.Request().Context(), ref)
	return writeData(c, out, err)
}

// GetTitleRatingsBatch: GET /api/v1/titles/tmdb/ratings?ids=movie:425,tv:1
func (h *Handler) GetTitleRatingsBatch(c *echo.Context) error {
	refs, err := service.ParseTmdbRefList(c.QueryParam("ids"))
	if err != nil {
		return writeData(c, nil, err)
	}
	out, err := h.Ratings.GetRatingsBatch(c.Request().Context(), refs)
	return writeData(c, out, err)
}

// LookupTitle: GET /api/v1/titles/lookup?imdb=tt…|tmdb=movie:425|slug=…
func (h *Handler) LookupTitle(c *echo.Context) error {
	out, err := h.Overview.Lookup(c.Request().Context(), c.QueryParam("imdb"), c.QueryParam("tmdb"), c.QueryParam("slug"))
	return writeData(c, out, err)
}

// --- netflix ----------------------------------------------------------------------

// GetNetflixTop10 returns Netflix's official weekly Top 10, globally or for a
// country: GET /api/v1/netflix/top10[/{country}][?week=&category=]
func (h *Handler) GetNetflixTop10(c *echo.Context) error {
	out, err := h.Analytics.NetflixWeek(c.Request().Context(), c.Param("country"), c.QueryParam("week"), c.QueryParam("category"))
	return writeData(c, out, err)
}

// GetNetflixMostPopular: GET /api/v1/netflix/most-popular[?category=]
func (h *Handler) GetNetflixMostPopular(c *echo.Context) error {
	out, err := h.Analytics.NetflixMostPopular(c.Request().Context(), c.QueryParam("category"))
	return writeData(c, out, err)
}

// --- analytics ----------------------------------------------------------------

func (h *Handler) analytics(run func(c *echo.Context, q service.AnalyticsQuery) (any, error)) echo.HandlerFunc {
	return func(c *echo.Context) error {
		from, to, err := parseRange(c)
		if err != nil {
			return badRequest(c, err)
		}
		q := service.AnalyticsQuery{From: from, To: to, Country: c.QueryParam("country"), Provider: c.QueryParam("provider"),
			Category: c.QueryParam("category"), Kind: c.QueryParam("kind")}
		out, err := run(c, q)
		return writeData(c, out, err)
	}
}

func (h *Handler) GetDecay() echo.HandlerFunc {
	return h.analytics(func(c *echo.Context, q service.AnalyticsQuery) (any, error) {
		return h.Analytics.Decay(c.Request().Context(), q)
	})
}

func (h *Handler) GetCountrySimilarity() echo.HandlerFunc {
	return h.analytics(func(c *echo.Context, q service.AnalyticsQuery) (any, error) {
		return h.Analytics.CountrySimilarity(c.Request().Context(), q)
	})
}

func (h *Handler) GetReleaseLag() echo.HandlerFunc {
	return h.analytics(func(c *echo.Context, q service.AnalyticsQuery) (any, error) {
		return h.Analytics.ReleaseLag(c.Request().Context(), q)
	})
}

func (h *Handler) GetRatingsVsPopularity() echo.HandlerFunc {
	return h.analytics(func(c *echo.Context, q service.AnalyticsQuery) (any, error) {
		return h.Analytics.RatingsVsPopularity(c.Request().Context(), q)
	})
}

func (h *Handler) GetGenres() echo.HandlerFunc {
	return h.analytics(func(c *echo.Context, q service.AnalyticsQuery) (any, error) {
		return h.Analytics.Genres(c.Request().Context(), q)
	})
}

func (h *Handler) GetNetflixCalibration() echo.HandlerFunc {
	return h.analytics(func(c *echo.Context, q service.AnalyticsQuery) (any, error) {
		return h.Analytics.NetflixCalibration(c.Request().Context(), q)
	})
}

// --- admin ------------------------------------------------------------------------

// GetTaskTypes: GET /api/v1/admin/task-types
func (h *Handler) GetTaskTypes(c *echo.Context) error {
	return c.JSON(http.StatusOK, h.AdminTasks.TaskTypes())
}

// RunJob starts a job outside its schedule: POST /api/v1/admin/jobs/{type}/run
func (h *Handler) RunJob(c *echo.Context) error {
	var in service.RunJobInput
	if c.Request().ContentLength != 0 {
		if err := c.Bind(&in); err != nil {
			return badRequest(c, errors.New("invalid JSON body"))
		}
	}
	out, err := h.AdminTasks.RunJob(c.Request().Context(), c.Param("type"), in)
	return writeRunTaskNowResponse(c, out, err)
}

// Backfill scrapes FlixPatrol charts for past dates: POST /api/v1/admin/backfill
func (h *Handler) Backfill(c *echo.Context) error {
	var in service.BackfillInput
	if err := c.Bind(&in); err != nil {
		return badRequest(c, errors.New("invalid JSON body"))
	}
	out, err := h.AdminTasks.Backfill(c.Request().Context(), in)
	return writeRunTaskNowResponse(c, out, err)
}

// RematchTitle queues a title for matching now:
// POST /api/v1/admin/titles/{id}/rematch {"clear": false}
func (h *Handler) RematchTitle(c *echo.Context) error {
	id, err := parseID(c.Param("id"), "title id")
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
	title, err := h.Titles.Rematch(c.Request().Context(), id, in.Clear)
	if err != nil {
		return writeTitleError(c, err)
	}
	run, err := h.AdminTasks.RunJob(c.Request().Context(), "titles.enrich", service.RunJobInput{TitleIDs: []int64{id}})
	var conflictErr *service.ConflictError
	if err != nil && !errors.As(err, &conflictErr) {
		return writeData(c, nil, err)
	}
	return c.JSON(http.StatusAccepted, map[string]any{"title": title, "job": run})
}

// GetDataQuality: GET /api/v1/admin/data-quality
func (h *Handler) GetDataQuality(c *echo.Context) error {
	out, err := h.DataQuality.Report(c.Request().Context())
	return writeData(c, out, err)
}
