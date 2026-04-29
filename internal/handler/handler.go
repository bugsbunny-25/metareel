package handler

import (
	"context"
	"database/sql"
	"errors"
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
}

func New(top10 *service.Top10ReadService, adminTasks *service.TaskScheduleAdminService, titles *service.TitleAdminService) *Handler {
	return &Handler{Top10: top10, AdminTasks: adminTasks, Titles: titles}
}

// Health is a lightweight liveness endpoint.
func (h *Handler) Health(c *echo.Context) error {
	return c.JSON(http.StatusOK, map[string]any{"status": "ok"})
}

func (h *Handler) Test(c *echo.Context) error {
	return c.String(http.StatusOK, "metareel: ok")
}

func (h *Handler) ListTitles(c *echo.Context) error {
	limit, offset, err := parsePagination(c.QueryParam("limit"), c.QueryParam("offset"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{"error": err.Error()})
	}
	kind := c.QueryParam("kind")
	search := c.QueryParam("q")
	out, err := h.Titles.ListTitlesPage(c.Request().Context(), kind, search, limit, offset)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]any{"error": "internal server error"})
	}
	return c.JSON(http.StatusOK, out)
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
	if errors.Is(err, sql.ErrNoRows) {
		return c.JSON(http.StatusNotFound, map[string]any{"error": "title not found"})
	}
	return c.JSON(http.StatusInternalServerError, map[string]any{"error": "internal server error"})
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
	out, runErr := h.AdminTasks.ListTaskRuns(c.Request().Context(), c.QueryParam("task_type"), limit, offset)
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
	switch {
	case errors.As(err, &validationErr):
		return c.JSON(http.StatusBadRequest, map[string]any{"error": validationErr.Error()})
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

func parseDate(raw string) (time.Time, error) {
	if raw == "" {
		return time.Time{}, errors.New("date is required (YYYY-MM-DD)")
	}
	d, err := time.Parse("2006-01-02", raw)
	if err != nil {
		return time.Time{}, errors.New("invalid date format, expected YYYY-MM-DD")
	}
	return d, nil
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
