package handler

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/labstack/echo/v5"

	"github.com/bugsbunny-25/metareel/internal/service"
)

type Handler struct {
	Top10      *service.Top10ReadService
	AdminTasks *service.TaskScheduleAdminService
	Titles     *service.TitleAdminService
	Ratings    *service.RatingsService
	Releases   *service.ReleasesService
	Candidates *service.TitleCandidatesService
	Stats      *service.AdminStatsService
	APIKeys    *service.APIKeyService
	Health     *service.HealthService

	Charts      *service.ChartsService
	Overview    *service.TitleOverviewService
	Analytics   *service.AnalyticsService
	DataQuality *service.DataQualityService
}

// Live is a lightweight liveness endpoint.
func (h *Handler) Live(c *echo.Context) error {
	return c.JSON(http.StatusOK, map[string]any{"status": "ok"})
}

// Ready reports whether the database and Redis are reachable (503 if not),
// and whether FlixPatrol's fetch backend is (informational).
func (h *Handler) Ready(c *echo.Context) error {
	if h.Health == nil {
		return h.Live(c)
	}
	rep, ok := h.Health.Check(c.Request().Context())
	if !ok {
		return c.JSON(http.StatusServiceUnavailable, rep)
	}
	return c.JSON(http.StatusOK, rep)
}

func (h *Handler) Test(c *echo.Context) error {
	return c.String(http.StatusOK, "metareel: ok")
}

func (h *Handler) ListTitles(c *echo.Context) error {
	limit, offset, err := parsePagination(c.QueryParam("limit"), c.QueryParam("offset"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{"error": err.Error()})
	}
	out, err := h.Titles.ListTitlesPage(c.Request().Context(), service.TitlesQuery{
		Kind:        c.QueryParam("kind"),
		Search:      c.QueryParam("q"),
		Missing:     c.QueryParam("missing"),
		MatchStatus: c.QueryParam("match_status"),
		Sort:        c.QueryParam("sort"),
	}, limit, offset)
	if err != nil {
		return writeTitleError(c, err)
	}
	return c.JSON(http.StatusOK, out)
}

// GetTitle returns one scraped title: GET /api/v1/titles/{id}
func (h *Handler) GetTitle(c *echo.Context) error {
	id, err := parseID(c.Param("id"), "title id")
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{"error": err.Error()})
	}
	out, err := h.Titles.GetTitle(c.Request().Context(), id)
	if err != nil {
		return writeTitleError(c, err)
	}
	return c.JSON(http.StatusOK, out)
}

// GetTitleRankingsByID returns a scraped title's ranking history, mapped or
// not: GET /api/v1/titles/{id}/rankings?country=&from=&to=
func (h *Handler) GetTitleRankingsByID(c *echo.Context) error {
	id, err := parseID(c.Param("id"), "title id")
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{"error": err.Error()})
	}
	q, err := parseRankingHistoryQuery(c)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{"error": err.Error()})
	}
	out, err := h.Top10.GetTitleRankingsByID(c.Request().Context(), id, q)
	if err != nil {
		return writeTitleError(c, err)
	}
	return c.JSON(http.StatusOK, out)
}

// GetTitleCandidates suggests TMDB / IMDb / RT mappings for a title:
// GET /api/v1/admin/titles/{id}/candidates?name=&year=&kind=
func (h *Handler) GetTitleCandidates(c *echo.Context) error {
	id, err := parseID(c.Param("id"), "title id")
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{"error": err.Error()})
	}
	year := 0
	if raw := c.QueryParam("year"); raw != "" {
		if year, err = strconv.Atoi(raw); err != nil || year < 1800 || year > 2200 {
			return c.JSON(http.StatusBadRequest, map[string]any{"error": "invalid year"})
		}
	}
	out, err := h.Candidates.Search(c.Request().Context(), id, service.TitleCandidatesQuery{
		Name: c.QueryParam("name"), Year: year, Kind: c.QueryParam("kind"),
	})
	if err != nil {
		return writeTitleError(c, err)
	}
	return c.JSON(http.StatusOK, out)
}

func writeTitleError(c *echo.Context, err error) error {
	var validationErr *service.ValidationError
	switch {
	case errors.As(err, &validationErr):
		return c.JSON(http.StatusBadRequest, map[string]any{"error": validationErr.Error()})
	case errors.Is(err, sql.ErrNoRows):
		return c.JSON(http.StatusNotFound, map[string]any{"error": "title not found"})
	default:
		return c.JSON(http.StatusInternalServerError, map[string]any{"error": "internal server error"})
	}
}

func parseID(raw, name string) (int64, error) {
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("invalid %s", name)
	}
	return id, nil
}

func (h *Handler) PatchTitle(c *echo.Context) error {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		return c.JSON(http.StatusBadRequest, map[string]any{"error": "invalid title id"})
	}
	var req service.PatchTitleIDsInput
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{"error": "invalid JSON body"})
	}
	out, err := h.Titles.PatchTitleIDs(c.Request().Context(), id, req)
	if err == nil {
		return c.JSON(http.StatusOK, out)
	}
	var validationErr *service.ValidationError
	if errors.As(err, &validationErr) {
		return c.JSON(http.StatusBadRequest, map[string]any{"error": validationErr.Error()})
	}
	if errors.Is(err, sql.ErrNoRows) {
		return c.JSON(http.StatusNotFound, map[string]any{"error": "title not found"})
	}
	return c.JSON(http.StatusInternalServerError, map[string]any{"error": "internal server error"})
}

// GetTitleRankings returns the ranking history of one title by TMDB ID:
// GET /api/v1/titles/tmdb/{kind}/{id}/rankings?country=&from=&to=
func (h *Handler) GetTitleRankings(c *echo.Context) error {
	ref, err := service.ParseTmdbRef(c.Param("kind"), c.Param("id"))
	if err != nil {
		return writeTitleRankingsError(c, err)
	}
	q, err := parseRankingHistoryQuery(c)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{"error": err.Error()})
	}
	out, err := h.Top10.GetTitleRankings(c.Request().Context(), ref, q)
	if err != nil {
		return writeTitleRankingsError(c, err)
	}
	return c.JSON(http.StatusOK, out)
}

// GetTitleRankingsBatch returns ranking histories for several TMDB titles:
// GET /api/v1/titles/tmdb/rankings?ids=movie:425,tv:154385&country=&from=&to=
func (h *Handler) GetTitleRankingsBatch(c *echo.Context) error {
	refs, err := service.ParseTmdbRefList(c.QueryParam("ids"))
	if err != nil {
		return writeTitleRankingsError(c, err)
	}
	q, err := parseRankingHistoryQuery(c)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{"error": err.Error()})
	}
	out, err := h.Top10.GetTitleRankingsBatch(c.Request().Context(), refs, q)
	if err != nil {
		return writeTitleRankingsError(c, err)
	}
	return c.JSON(http.StatusOK, out)
}

// GetTitleRatings returns IMDb and Rotten Tomatoes ratings by TMDB ID, from
// the database if refreshed within RATINGS_TTL, else freshly fetched:
// GET /api/v1/titles/tmdb/{kind}/{id}/ratings[?refresh=true]
func (h *Handler) GetTitleRatings(c *echo.Context) error {
	ref, err := service.ParseTmdbRef(c.Param("kind"), c.Param("id"))
	if err != nil {
		return writeTitleRankingsError(c, err)
	}
	force, err := parseOptionalBool("refresh", c.QueryParam("refresh"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{"error": err.Error()})
	}
	out, err := h.Ratings.GetRatings(c.Request().Context(), ref, force)
	if errors.Is(err, service.ErrRatingsUnavailable) {
		return c.JSON(http.StatusBadGateway, map[string]any{"error": err.Error()})
	}
	if err != nil {
		return writeTitleRankingsError(c, err)
	}
	return c.JSON(http.StatusOK, out)
}

// GetNewTitles lists titles added to a streaming service, per day:
// GET /api/v1/titles/new/{country}/{service}?from=&to=&type=&language=
func (h *Handler) GetNewTitles(c *echo.Context) error {
	return h.handleReleases(c, h.Releases.GetNew)
}

// GetUpcomingTitles lists titles announced for a streaming service:
// GET /api/v1/titles/upcoming/{country}/{service}?from=&to=&type=&language=
func (h *Handler) GetUpcomingTitles(c *echo.Context) error {
	return h.handleReleases(c, h.Releases.GetUpcoming)
}

// GetServices lists the streaming services JustWatch knows in a country:
// GET /api/v1/services/{country}
func (h *Handler) GetServices(c *echo.Context) error {
	out, err := h.Releases.ListServices(c.Request().Context(), c.Param("country"))
	if err != nil {
		return writeReleasesError(c, err)
	}
	return c.JSON(http.StatusOK, out)
}

func (h *Handler) handleReleases(c *echo.Context, get func(context.Context, service.ReleasesQuery) (*service.ReleasesResponse, error)) error {
	from, err := parseOptionalDate("from", c.QueryParam("from"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{"error": err.Error()})
	}
	to, err := parseOptionalDate("to", c.QueryParam("to"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{"error": err.Error()})
	}
	out, err := get(c.Request().Context(), service.ReleasesQuery{
		CountryCode: c.Param("country"),
		Service:     c.Param("service"),
		From:        from,
		To:          to,
		Kind:        c.QueryParam("type"),
		Language:    c.QueryParam("language"),
	})
	if err != nil {
		return writeReleasesError(c, err)
	}
	return c.JSON(http.StatusOK, out)
}

func writeReleasesError(c *echo.Context, err error) error {
	var validationErr *service.ValidationError
	switch {
	case errors.As(err, &validationErr):
		return c.JSON(http.StatusBadRequest, map[string]any{"error": validationErr.Error()})
	case errors.Is(err, service.ErrUpstream):
		return c.JSON(http.StatusBadGateway, map[string]any{"error": "justwatch request failed"})
	default:
		return c.JSON(http.StatusInternalServerError, map[string]any{"error": "internal server error"})
	}
}

func parseOptionalBool(name, raw string) (bool, error) {
	if raw == "" {
		return false, nil
	}
	v, err := strconv.ParseBool(raw)
	if err != nil {
		return false, fmt.Errorf("invalid %s, expected true or false", name)
	}
	return v, nil
}

func parseRankingHistoryQuery(c *echo.Context) (service.RankingHistoryQuery, error) {
	from, err := parseOptionalDate("from", c.QueryParam("from"))
	if err != nil {
		return service.RankingHistoryQuery{}, err
	}
	to, err := parseOptionalDate("to", c.QueryParam("to"))
	if err != nil {
		return service.RankingHistoryQuery{}, err
	}
	return service.RankingHistoryQuery{CountryCode: c.QueryParam("country"), From: from, To: to}, nil
}

func writeTitleRankingsError(c *echo.Context, err error) error {
	var validationErr *service.ValidationError
	switch {
	case errors.As(err, &validationErr):
		return c.JSON(http.StatusBadRequest, map[string]any{"error": validationErr.Error()})
	case errors.Is(err, service.ErrTitleNotFound):
		return c.JSON(http.StatusNotFound, map[string]any{"error": err.Error()})
	default:
		return c.JSON(http.StatusInternalServerError, map[string]any{"error": "internal server error"})
	}
}

func (h *Handler) GetTop10MoviesByProvider(c *echo.Context) error {
	return h.handleTop10ByProvider(c, h.Top10.GetMoviesByProvider)
}

func (h *Handler) GetTop10MoviesAllProviders(c *echo.Context) error {
	return h.handleTop10AllProviders(c, h.Top10.GetMoviesAllProviders)
}

func (h *Handler) GetTop10TVShowsByProvider(c *echo.Context) error {
	return h.handleTop10ByProvider(c, h.Top10.GetTVShowsByProvider)
}

func (h *Handler) GetTop10TVShowsAllProviders(c *echo.Context) error {
	return h.handleTop10AllProviders(c, h.Top10.GetTVShowsAllProviders)
}

func (h *Handler) CreateTaskSchedule(c *echo.Context) error {
	var req service.CreateTaskScheduleInput
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{"error": "invalid JSON body"})
	}
	out, err := h.AdminTasks.CreateTaskSchedule(c.Request().Context(), req)
	return writeTaskScheduleResponse(c, out, err)
}

func (h *Handler) PatchTaskSchedule(c *echo.Context) error {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		return c.JSON(http.StatusBadRequest, map[string]any{"error": "invalid schedule id"})
	}
	var req service.UpdateTaskScheduleInput
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{"error": "invalid JSON body"})
	}
	out, err := h.AdminTasks.UpdateTaskSchedule(c.Request().Context(), id, req)
	return writeTaskScheduleResponse(c, out, err)
}

func (h *Handler) AddFlixPatrolTargets(c *echo.Context) error {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		return c.JSON(http.StatusBadRequest, map[string]any{"error": "invalid schedule id"})
	}
	var req service.AddFlixPatrolTargetsInput
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{"error": "invalid JSON body"})
	}
	out, err := h.AdminTasks.AddFlixPatrolTargets(c.Request().Context(), id, req)
	return writeTaskScheduleResponse(c, out, err)
}

func (h *Handler) GetTaskSchedules(c *echo.Context) error {
	var enabledPtr *bool
	if raw := c.QueryParam("enabled"); raw != "" {
		v, err := strconv.ParseBool(raw)
		if err != nil {
			return c.JSON(http.StatusBadRequest, map[string]any{"error": "invalid enabled filter, expected true|false"})
		}
		enabledPtr = &v
	}

	out, err := h.AdminTasks.ListTaskSchedules(c.Request().Context(), enabledPtr, c.QueryParam("task_type"))
	return writeTaskScheduleListResponse(c, out, err)
}

func (h *Handler) GetTaskScheduleByID(c *echo.Context) error {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		return c.JSON(http.StatusBadRequest, map[string]any{"error": "invalid schedule id"})
	}
	out, err := h.AdminTasks.GetTaskScheduleByID(c.Request().Context(), id)
	return writeTaskScheduleResponse(c, out, err)
}

func (h *Handler) RunTaskScheduleNow(c *echo.Context) error {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		return c.JSON(http.StatusBadRequest, map[string]any{"error": "invalid schedule id"})
	}
	out, err := h.AdminTasks.RunTaskScheduleNow(c.Request().Context(), id)
	return writeRunTaskNowResponse(c, out, err)
}

func (h *Handler) GetTaskRunsByScheduleID(c *echo.Context) error {
	scheduleID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || scheduleID <= 0 {
		return c.JSON(http.StatusBadRequest, map[string]any{"error": "invalid schedule id"})
	}
	limit, offset, err := parsePagination(c.QueryParam("limit"), c.QueryParam("offset"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{"error": err.Error()})
	}
	out, runErr := h.AdminTasks.ListTaskRunsByScheduleID(c.Request().Context(), scheduleID, limit, offset)
	return writeTaskRunListResponse(c, out, runErr)
}

func (h *Handler) GetTaskRuns(c *echo.Context) error {
	limit, offset, err := parsePagination(c.QueryParam("limit"), c.QueryParam("offset"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{"error": err.Error()})
	}
	q := service.TaskRunsQuery{TaskType: c.QueryParam("task_type"), Status: c.QueryParam("status")}
	if raw := c.QueryParam("schedule_id"); raw != "" {
		if q.ScheduleID, err = parseID(raw, "schedule_id"); err != nil {
			return c.JSON(http.StatusBadRequest, map[string]any{"error": err.Error()})
		}
	}
	out, total, runErr := h.AdminTasks.ListTaskRuns(c.Request().Context(), q, limit, offset)
	if runErr == nil {
		c.Response().Header().Set("X-Total-Count", strconv.FormatInt(total, 10))
	}
	return writeTaskRunListResponse(c, out, runErr)
}

func (h *Handler) GetTaskRunByID(c *echo.Context) error {
	runID, err := strconv.ParseInt(c.Param("run_id"), 10, 64)
	if err != nil || runID <= 0 {
		return c.JSON(http.StatusBadRequest, map[string]any{"error": "invalid run id"})
	}
	out, runErr := h.AdminTasks.GetTaskRunByID(c.Request().Context(), runID)
	return writeTaskRunResponse(c, out, runErr)
}

func (h *Handler) GetTaskRunLogsByRunID(c *echo.Context) error {
	runID, err := strconv.ParseInt(c.Param("run_id"), 10, 64)
	if err != nil || runID <= 0 {
		return c.JSON(http.StatusBadRequest, map[string]any{"error": "invalid run id"})
	}
	out, runErr := h.AdminTasks.ListTaskRunLogsByRunID(c.Request().Context(), runID)
	return writeTaskRunLogsResponse(c, out, runErr)
}

func (h *Handler) handleTop10ByProvider(c *echo.Context, getter func(ctx context.Context, q service.Top10Query) (*service.Top10Response, error)) error {
	date, err := parseDate(c.QueryParam("date"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{"error": err.Error()})
	}

	out, err := getter(c.Request().Context(), service.Top10Query{
		Date:        date,
		CountryCode: c.Param("country"),
		Provider:    c.Param("provider"),
	})
	return writeTop10Response(c, out, err)
}

func (h *Handler) handleTop10AllProviders(c *echo.Context, getter func(ctx context.Context, q service.Top10Query) (*service.Top10Response, error)) error {
	date, err := parseDate(c.QueryParam("date"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{"error": err.Error()})
	}

	out, err := getter(c.Request().Context(), service.Top10Query{
		Date:        date,
		CountryCode: c.Param("country"),
	})
	return writeTop10Response(c, out, err)
}

func writeTop10Response(c *echo.Context, out *service.Top10Response, err error) error {
	if err == nil {
		return c.JSON(http.StatusOK, out)
	}

	var validationErr *service.ValidationError
	switch {
	case errors.As(err, &validationErr):
		return c.JSON(http.StatusBadRequest, map[string]any{"error": validationErr.Error()})
	case errors.Is(err, service.ErrTop10NotFound):
		return c.JSON(http.StatusNotFound, map[string]any{"error": err.Error()})
	default:
		return c.JSON(http.StatusInternalServerError, map[string]any{"error": "internal server error"})
	}
}

func writeTaskScheduleResponse(c *echo.Context, out *service.TaskScheduleResponse, err error) error {
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
	case errors.Is(err, sql.ErrNoRows):
		return c.JSON(http.StatusNotFound, map[string]any{"error": "task schedule not found"})
	default:
		return c.JSON(http.StatusInternalServerError, map[string]any{"error": "internal server error"})
	}
}

func writeTaskScheduleListResponse(c *echo.Context, out []service.TaskScheduleListResponse, err error) error {
	if err == nil {
		return c.JSON(http.StatusOK, out)
	}
	var validationErr *service.ValidationError
	switch {
	case errors.As(err, &validationErr):
		return c.JSON(http.StatusBadRequest, map[string]any{"error": validationErr.Error()})
	default:
		return c.JSON(http.StatusInternalServerError, map[string]any{"error": "internal server error"})
	}
}

func writeRunTaskNowResponse(c *echo.Context, out *service.RunTaskNowResponse, err error) error {
	if err == nil {
		return c.JSON(http.StatusAccepted, out)
	}
	var validationErr *service.ValidationError
	var conflictErr *service.ConflictError
	switch {
	case errors.As(err, &validationErr):
		return c.JSON(http.StatusBadRequest, map[string]any{"error": validationErr.Error()})
	case errors.As(err, &conflictErr):
		return c.JSON(http.StatusConflict, map[string]any{"error": conflictErr.Error()})
	case errors.Is(err, sql.ErrNoRows):
		return c.JSON(http.StatusNotFound, map[string]any{"error": "task schedule not found"})
	default:
		return c.JSON(http.StatusInternalServerError, map[string]any{"error": "internal server error"})
	}
}

func writeTaskRunListResponse(c *echo.Context, out []service.TaskRunResponse, err error) error {
	if err == nil {
		return c.JSON(http.StatusOK, out)
	}
	var validationErr *service.ValidationError
	switch {
	case errors.As(err, &validationErr):
		return c.JSON(http.StatusBadRequest, map[string]any{"error": validationErr.Error()})
	default:
		return c.JSON(http.StatusInternalServerError, map[string]any{"error": "internal server error"})
	}
}

func writeTaskRunResponse(c *echo.Context, out *service.TaskRunResponse, err error) error {
	if err == nil {
		return c.JSON(http.StatusOK, out)
	}
	var validationErr *service.ValidationError
	switch {
	case errors.As(err, &validationErr):
		return c.JSON(http.StatusBadRequest, map[string]any{"error": validationErr.Error()})
	case errors.Is(err, sql.ErrNoRows):
		return c.JSON(http.StatusNotFound, map[string]any{"error": "task run not found"})
	default:
		return c.JSON(http.StatusInternalServerError, map[string]any{"error": "internal server error"})
	}
}

func writeTaskRunLogsResponse(c *echo.Context, out []service.TaskRunLogResponse, err error) error {
	if err == nil {
		return c.JSON(http.StatusOK, out)
	}
	var validationErr *service.ValidationError
	switch {
	case errors.As(err, &validationErr):
		return c.JSON(http.StatusBadRequest, map[string]any{"error": validationErr.Error()})
	default:
		return c.JSON(http.StatusInternalServerError, map[string]any{"error": "internal server error"})
	}
}

// parseDate parses an optional YYYY-MM-DD date; empty returns the zero time.
func parseDate(raw string) (time.Time, error) {
	if raw == "" {
		return time.Time{}, nil
	}
	d, err := time.Parse("2006-01-02", raw)
	if err != nil {
		return time.Time{}, errors.New("invalid date format, expected YYYY-MM-DD")
	}
	return d, nil
}

func parseOptionalDate(name, raw string) (*time.Time, error) {
	if raw == "" {
		return nil, nil
	}
	d, err := time.Parse("2006-01-02", raw)
	if err != nil {
		return nil, fmt.Errorf("invalid %s, expected YYYY-MM-DD", name)
	}
	return &d, nil
}

func parsePagination(rawLimit string, rawOffset string) (int64, int64, error) {
	limit := int64(50)
	offset := int64(0)
	if rawLimit != "" {
		v, err := strconv.ParseInt(rawLimit, 10, 64)
		if err != nil || v <= 0 {
			return 0, 0, errors.New("invalid limit, expected positive integer")
		}
		limit = v
	}
	if rawOffset != "" {
		v, err := strconv.ParseInt(rawOffset, 10, 64)
		if err != nil || v < 0 {
			return 0, 0, errors.New("invalid offset, expected non-negative integer")
		}
		offset = v
	}
	return limit, offset, nil
}
