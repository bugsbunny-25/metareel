package handler

import (
	"database/sql"
	"errors"
	"net/http"
	"slices"
	"strconv"
	"strings"
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
		errors.Is(err, service.ErrNoMetadata), errors.Is(err, service.ErrNetflixTitleNotFound),
		errors.Is(err, service.ErrNetflixSeasonNotFound), errors.Is(err, service.ErrWebhookNotFound),
		errors.Is(err, service.ErrReviewSubjectNotFound), errors.Is(err, service.ErrPersonNotFound):
		return c.JSON(http.StatusNotFound, map[string]any{"error": err.Error()})
	case errors.Is(err, sql.ErrNoRows):
		return c.JSON(http.StatusNotFound, map[string]any{"error": "not found"})
	case errors.Is(err, service.ErrRatingsUnavailable), errors.Is(err, service.ErrUpstream), errors.Is(err, service.ErrPersonUnavailable):
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

// SearchTitles: GET /api/v1/search?q=&kind=movie|tv&limit=
func (h *Handler) SearchTitles(c *echo.Context) error {
	limit, err := parseIntParam(c, "limit", 0)
	if err != nil {
		return badRequest(c, err)
	}
	out, err := h.Search.Search(c.Request().Context(), service.SearchQuery{Q: c.QueryParam("q"), Kind: c.QueryParam("kind"), Limit: limit})
	return writeData(c, out, err)
}

// GetTitleHistory: GET /api/v1/titles/tmdb/{kind}/{id}/history[?from=&to=]
func (h *Handler) GetTitleHistory(c *echo.Context) error {
	ref, err := h.parseRef(c)
	if err != nil {
		return writeData(c, nil, err)
	}
	from, to, err := parseRange(c)
	if err != nil {
		return badRequest(c, err)
	}
	out, err := h.History.TitleHistory(c.Request().Context(), ref, service.TitleHistoryQuery{From: from, To: to})
	return writeData(c, out, err)
}

// GetTitleTimeline: GET /api/v1/titles/tmdb/{kind}/{id}/timeline[?types=&country=&from=&to=&limit=]
func (h *Handler) GetTitleTimeline(c *echo.Context) error {
	ref, err := h.parseRef(c)
	if err != nil {
		return writeData(c, nil, err)
	}
	from, to, err := parseRange(c)
	if err != nil {
		return badRequest(c, err)
	}
	limit, err := parseIntParam(c, "limit", 0)
	if err != nil {
		return badRequest(c, err)
	}
	out, err := h.Timeline.Timeline(c.Request().Context(), ref, service.TimelineQuery{
		Types: c.QueryParam("types"), Country: c.QueryParam("country"), From: from, To: to, Limit: limit,
	})
	return writeData(c, out, err)
}

// GetTitleReleases returns a movie's release dates and windows per country:
// GET /api/v1/titles/tmdb/movie/{id}/releases[?country=]
func (h *Handler) GetTitleReleases(c *echo.Context) error {
	ref, err := h.parseRef(c)
	if err != nil {
		return writeData(c, nil, err)
	}
	out, err := h.ReleaseWindows.TitleReleases(c.Request().Context(), ref, c.QueryParam("country"))
	return writeData(c, out, err)
}

// GetReleaseWindows: GET /api/v1/analytics/release-windows[?from=&to=&country=&limit=]
func (h *Handler) GetReleaseWindows(c *echo.Context) error {
	from, to, err := parseRange(c)
	if err != nil {
		return badRequest(c, err)
	}
	limit, err := parseIntParam(c, "limit", 0)
	if err != nil {
		return badRequest(c, err)
	}
	if c.QueryParam("limit") != "" && limit == 0 {
		return badRequest(c, errors.New("invalid limit, expected a positive integer"))
	}
	out, err := h.ReleaseWindows.Windows(c.Request().Context(), service.ReleaseWindowsQuery{From: from, To: to, Country: c.QueryParam("country"), Limit: limit})
	return writeData(c, out, err)
}

// GetTitleAvailabilityHistory: GET /api/v1/titles/tmdb/{kind}/{id}/availability/history[?country=]
func (h *Handler) GetTitleAvailabilityHistory(c *echo.Context) error {
	ref, err := h.parseRef(c)
	if err != nil {
		return writeData(c, nil, err)
	}
	out, err := h.History.TitleAvailabilityHistory(c.Request().Context(), ref, c.QueryParam("country"))
	return writeData(c, out, err)
}

// GetAvailabilityChanges: GET /api/v1/availability/changes[?since=&country=&event=&monetization=&provider_id=&limit=]
func (h *Handler) GetAvailabilityChanges(c *echo.Context) error {
	q := service.AvailabilityChangesQuery{
		Country: c.QueryParam("country"), Event: c.QueryParam("event"),
		Monetization: c.QueryParam("monetization"), ProviderID: c.QueryParam("provider_id"),
	}
	if raw := c.QueryParam("since"); raw != "" {
		t, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			return badRequest(c, errors.New("invalid since, expected RFC 3339 (e.g. 2026-10-09T12:00:00Z)"))
		}
		q.Since = &t
	}
	limit, err := parseIntParam(c, "limit", 200)
	if err != nil {
		return badRequest(c, err)
	}
	q.Limit = limit
	out, err := h.History.AvailabilityChanges(c.Request().Context(), q)
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

// LookupTitle: GET /api/v1/titles/lookup?imdb=tt…|tmdb=movie:425|slug=…|netflix=81231974
func (h *Handler) LookupTitle(c *echo.Context) error {
	out, err := h.Overview.Lookup(c.Request().Context(), c.QueryParam("imdb"), c.QueryParam("tmdb"), c.QueryParam("slug"), c.QueryParam("netflix"))
	return writeData(c, out, err)
}

// GetCalendar: GET /api/v1/calendar/{country}[?from=&to=&provider_id=]
func (h *Handler) GetCalendar(c *echo.Context) error {
	from, to, err := parseRange(c)
	if err != nil {
		return badRequest(c, err)
	}
	out, err := h.Calendar.Calendar(c.Request().Context(), service.CalendarQuery{
		Country: c.Param("country"), From: from, To: to, ProviderID: c.QueryParam("provider_id"),
	})
	return writeData(c, out, err)
}

// GetBreakouts lists titles rising in several countries at once:
// GET /api/v1/breakouts[?date=&window=&min_countries=&min_climb=&category=&provider=&kind=&limit=]
func (h *Handler) GetBreakouts(c *echo.Context) error {
	date, err := parseOptionalDate("date", c.QueryParam("date"))
	if err != nil {
		return badRequest(c, err)
	}
	q := service.BreakoutQuery{Date: date, Category: c.QueryParam("category"), Provider: c.QueryParam("provider"), Kind: c.QueryParam("kind")}
	for _, p := range []struct {
		name string
		dst  *int
	}{{"window", &q.Window}, {"min_countries", &q.MinCountries}, {"min_climb", &q.MinClimb}, {"limit", &q.Limit}} {
		v, err := parseIntParam(c, p.name, 0)
		if err != nil {
			return badRequest(c, err)
		}
		if c.QueryParam(p.name) != "" && v == 0 {
			return badRequest(c, errors.New("invalid "+p.name+", expected a positive integer"))
		}
		*p.dst = v
	}
	out, err := h.Breakouts.Breakouts(c.Request().Context(), q)
	return writeData(c, out, err)
}

// --- discovery --------------------------------------------------------------------

// intParams reads optional positive integer query params (0 = default).
func intParams(c *echo.Context, dst map[string]*int) error {
	for _, name := range []string{"days", "limit", "slots", "min_titles"} {
		p, ok := dst[name]
		if !ok {
			continue
		}
		v, err := parseIntParam(c, name, 0)
		if err != nil {
			return err
		}
		if c.QueryParam(name) != "" && v == 0 {
			return errors.New("invalid " + name + ", expected a positive integer")
		}
		*p = v
	}
	return nil
}

// GetHiddenGems: GET /api/v1/discover/{country}/hidden-gems[?kind=&provider_id=&min_rating=&min_votes=&days=&limit=]
func (h *Handler) GetHiddenGems(c *echo.Context) error {
	q := service.HiddenGemsQuery{Country: c.Param("country"), Kind: c.QueryParam("kind")}
	if err := intParams(c, map[string]*int{"days": &q.Days, "limit": &q.Limit}); err != nil {
		return badRequest(c, err)
	}
	if raw := c.QueryParam("provider_id"); raw != "" {
		v, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || v <= 0 {
			return badRequest(c, errors.New("invalid provider_id"))
		}
		q.ProviderID = v
	}
	if raw := c.QueryParam("min_rating"); raw != "" {
		v, err := strconv.ParseFloat(raw, 64)
		if err != nil || !(v >= 1 && v <= 10) { // also rejects NaN
			return badRequest(c, errors.New("invalid min_rating, expected 1-10"))
		}
		q.MinRating = v
	}
	if raw := c.QueryParam("min_votes"); raw != "" {
		v, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || v <= 0 {
			return badRequest(c, errors.New("invalid min_votes, expected a positive integer"))
		}
		q.MinVotes = v
	}
	out, err := h.Discover.HiddenGems(c.Request().Context(), q)
	return writeData(c, out, err)
}

// GetElsewhere: GET /api/v1/discover/{country}/elsewhere[?kind=&days=&limit=]
func (h *Handler) GetElsewhere(c *echo.Context) error {
	q := service.ElsewhereQuery{Country: c.Param("country"), Kind: c.QueryParam("kind")}
	if err := intParams(c, map[string]*int{"days": &q.Days, "limit": &q.Limit}); err != nil {
		return badRequest(c, err)
	}
	out, err := h.Discover.Elsewhere(c.Request().Context(), q)
	return writeData(c, out, err)
}

// GetRotation: GET /api/v1/discover/{country}/rotation[?subscribed=8,9&slots=&days=&min_titles=]
func (h *Handler) GetRotation(c *echo.Context) error {
	q := service.RotationQuery{Country: c.Param("country"), IncludeOther: c.QueryParam("include_other") == "true"}
	if err := intParams(c, map[string]*int{"days": &q.Days, "slots": &q.Slots, "min_titles": &q.MinTitles}); err != nil {
		return badRequest(c, err)
	}
	if raw := strings.TrimSpace(c.QueryParam("subscribed")); raw != "" {
		parts := strings.Split(raw, ",")
		if len(parts) > 20 {
			return badRequest(c, errors.New("at most 20 subscribed services"))
		}
		for _, part := range parts {
			v, err := strconv.ParseInt(strings.TrimSpace(part), 10, 64)
			if err != nil || v <= 0 {
				return badRequest(c, errors.New("invalid subscribed, expected comma-separated TMDB provider IDs"))
			}
			if !slices.Contains(q.Subscribed, v) {
				q.Subscribed = append(q.Subscribed, v)
			}
		}
	}
	out, err := h.Discover.Rotation(c.Request().Context(), q)
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
			Category: c.QueryParam("category"), Kind: c.QueryParam("kind"), Interval: c.QueryParam("interval"), Window: c.QueryParam("window"),
			Role: c.QueryParam("role")}
		if err := intParams(c, map[string]*int{"limit": &q.Limit}); err != nil {
			return badRequest(c, err)
		}
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

func (h *Handler) GetReleasePatterns() echo.HandlerFunc {
	return h.analytics(func(c *echo.Context, q service.AnalyticsQuery) (any, error) {
		return h.Analytics.ReleasePatterns(c.Request().Context(), q)
	})
}

func (h *Handler) GetSpreadLag() echo.HandlerFunc {
	return h.analytics(func(c *echo.Context, q service.AnalyticsQuery) (any, error) {
		return h.Analytics.SpreadLag(c.Request().Context(), q)
	})
}

func (h *Handler) GetPeopleAnalytics() echo.HandlerFunc {
	return h.analytics(func(c *echo.Context, q service.AnalyticsQuery) (any, error) {
		return h.Analytics.People(c.Request().Context(), q)
	})
}

// GetTitleCredits: GET /api/v1/titles/tmdb/{kind}/{id}/credits
func (h *Handler) GetTitleCredits(c *echo.Context) error {
	ref, err := h.parseRef(c)
	if err != nil {
		return writeData(c, nil, err)
	}
	out, err := h.Credits.TitleCredits(c.Request().Context(), ref)
	return writeData(c, out, err)
}

// GetPerson: GET /api/v1/people/{id}
func (h *Handler) GetPerson(c *echo.Context) error {
	id, err := parseID(c.Param("id"), "person id")
	if err != nil {
		return badRequest(c, err)
	}
	out, err := h.Credits.Person(c.Request().Context(), id)
	return writeData(c, out, err)
}

func (h *Handler) GetOrigins() echo.HandlerFunc {
	return h.analytics(func(c *echo.Context, q service.AnalyticsQuery) (any, error) {
		return h.Analytics.Origins(c.Request().Context(), q)
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
