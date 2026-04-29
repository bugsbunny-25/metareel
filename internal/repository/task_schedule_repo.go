package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/bugsbunny-25/metareel/internal/repository/sqlc"
)

const taskTypeFlixPatrolTop10 = "flixpatrol.top10.scrape"

type TaskScheduleRepository struct {
	q *sqlc.Queries
	db *sql.DB
}

func NewTaskScheduleRepository(db *sql.DB) *TaskScheduleRepository {
	return &TaskScheduleRepository{q: sqlc.New(db), db: db}
}

func (r *TaskScheduleRepository) ListFlixPatrolTop10TaskScheduleConfigs(ctx context.Context) ([]FlixPatrolTaskSchedule, error) {
	rows, err := r.q.ListEnabledTaskSchedules(ctx)
	if err != nil {
		return nil, fmt.Errorf("list enabled task schedules: %w", err)
	}

	out := make([]FlixPatrolTaskSchedule, 0, len(rows))
	for _, row := range rows {
		if row.TaskType != taskTypeFlixPatrolTop10 {
			continue
		}
		schedule, err := r.buildTaskSchedule(ctx, row.ID)
		if err != nil {
			return nil, err
		}
		out = append(out, *schedule)
	}
	return out, nil
}

type FlixPatrolTarget struct {
	CountrySlug  string
	ProviderSlug string
}

type FlixPatrolTaskSchedule struct {
	ID                  int64
	TaskType            string
	Name                string
	Enabled             bool
	MaxRetries          int64
	RequestDelaySeconds int64
	RespectRobots       bool
	UserAgent           string
	RunTimesUTC         []string
	Targets             []FlixPatrolTarget
}

type TaskScheduleSummary struct {
	ID          int64
	TaskType    string
	Name        string
	Enabled     bool
	RunTimesUTC []string
}

func (r *TaskScheduleRepository) ListTaskSchedules(ctx context.Context, enabled *bool, taskType string) ([]TaskScheduleSummary, error) {
	enableFilter := int64(0)
	enableValue := false
	if enabled != nil {
		enableFilter = 1
		enableValue = *enabled
	}
	typeFilter := int64(0)
	trimmedType := strings.TrimSpace(taskType)
	if trimmedType != "" {
		typeFilter = 1
	}

	rows, err := r.q.ListTaskSchedules(ctx, sqlc.ListTaskSchedulesParams{
		Column1:  enableFilter,
		Enabled:  enableValue,
		Column3:  typeFilter,
		TaskType: trimmedType,
	})
	if err != nil {
		return nil, fmt.Errorf("list task schedules: %w", err)
	}

	out := make([]TaskScheduleSummary, 0, len(rows))
	for _, row := range rows {
		timeRows, err := r.q.ListTaskScheduleRunTimesByScheduleID(ctx, row.ID)
		if err != nil {
			return nil, fmt.Errorf("list task schedule times %d: %w", row.ID, err)
		}
		runTimes := make([]string, 0, len(timeRows))
		for _, rt := range timeRows {
			runTimes = append(runTimes, rt.RunTimeUtc)
		}
		out = append(out, TaskScheduleSummary{
			ID:          row.ID,
			TaskType:    row.TaskType,
			Name:        row.Name,
			Enabled:     row.Enabled,
			RunTimesUTC: runTimes,
		})
	}
	return out, nil
}

func (r *TaskScheduleRepository) GetFlixPatrolTaskScheduleByID(ctx context.Context, id int64) (*FlixPatrolTaskSchedule, error) {
	schedule, err := r.GetTaskScheduleByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if schedule.TaskType != taskTypeFlixPatrolTop10 {
		return nil, fmt.Errorf("task schedule %d has unsupported type: %s", id, schedule.TaskType)
	}
	return schedule, nil
}

func (r *TaskScheduleRepository) GetTaskScheduleByID(ctx context.Context, id int64) (*FlixPatrolTaskSchedule, error) {
	return r.buildTaskSchedule(ctx, id)
}

type CreateTaskScheduleInput struct {
	TaskType            string
	Name                string
	Enabled             bool
	MaxRetries          int64
	RequestDelaySeconds int64
	RespectRobots       bool
	UserAgent           string
	UTCRunTimes         []string
}

func (r *TaskScheduleRepository) CreateTaskSchedule(ctx context.Context, in CreateTaskScheduleInput) (int64, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	q := sqlc.New(tx)
	created, err := q.CreateTaskSchedule(ctx, sqlc.CreateTaskScheduleParams{
		TaskType:            in.TaskType,
		Name:                in.Name,
		Enabled:             in.Enabled,
		MaxRetries:          in.MaxRetries,
		RequestDelaySeconds: in.RequestDelaySeconds,
		RespectRobots:       in.RespectRobots,
		UserAgent:           in.UserAgent,
	})
	if err != nil {
		return 0, fmt.Errorf("create task schedule: %w", err)
	}

	for _, runtime := range in.UTCRunTimes {
		if err := q.CreateTaskScheduleRunTime(ctx, sqlc.CreateTaskScheduleRunTimeParams{
			ScheduleID: created.ID,
			RunTimeUtc: runtime,
		}); err != nil {
			return 0, fmt.Errorf("create run time %s: %w", runtime, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit tx: %w", err)
	}
	return created.ID, nil
}

type UpdateTaskScheduleInput struct {
	Name                string
	Enabled             bool
	MaxRetries          int64
	RequestDelaySeconds int64
	RespectRobots       bool
	UserAgent           string
	UTCRunTimes         []string
}

func (r *TaskScheduleRepository) UpdateTaskSchedule(ctx context.Context, id int64, in UpdateTaskScheduleInput) (*FlixPatrolTaskSchedule, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	q := sqlc.New(tx)
	if _, err := q.UpdateTaskSchedule(ctx, sqlc.UpdateTaskScheduleParams{
		ID:                  id,
		Name:                in.Name,
		Enabled:             in.Enabled,
		MaxRetries:          in.MaxRetries,
		RequestDelaySeconds: in.RequestDelaySeconds,
		RespectRobots:       in.RespectRobots,
		UserAgent:           in.UserAgent,
	}); err != nil {
		return nil, fmt.Errorf("update task schedule: %w", err)
	}

	if err := q.DeleteTaskScheduleRunTimesByScheduleID(ctx, id); err != nil {
		return nil, fmt.Errorf("delete run times: %w", err)
	}
	for _, runtime := range in.UTCRunTimes {
		if err := q.CreateTaskScheduleRunTime(ctx, sqlc.CreateTaskScheduleRunTimeParams{
			ScheduleID: id,
			RunTimeUtc: runtime,
		}); err != nil {
			return nil, fmt.Errorf("create run time %s: %w", runtime, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit tx: %w", err)
	}
	return r.buildTaskSchedule(ctx, id)
}

func (r *TaskScheduleRepository) AddFlixPatrolTargets(ctx context.Context, scheduleID int64, targets []FlixPatrolTarget) error {
	for _, t := range targets {
		if err := r.q.CreateTaskScheduleTarget(ctx, sqlc.CreateTaskScheduleTargetParams{
			ScheduleID:   scheduleID,
			CountrySlug:  t.CountrySlug,
			ProviderSlug: t.ProviderSlug,
		}); err != nil {
			return fmt.Errorf("create target (%s,%s): %w", t.CountrySlug, t.ProviderSlug, err)
		}
	}
	return nil
}

func IsUniqueConstraintError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "UNIQUE constraint failed")
}

func NormalizeUTCRuntime(v string) (string, error) {
	parts := strings.Split(strings.TrimSpace(v), ":")
	if len(parts) != 2 {
		return "", fmt.Errorf("invalid UTC runtime format (want HH:MM)")
	}
	hour, err := strconv.Atoi(parts[0])
	if err != nil || hour < 0 || hour > 23 {
		return "", fmt.Errorf("invalid UTC runtime hour")
	}
	minute, err := strconv.Atoi(parts[1])
	if err != nil || minute < 0 || minute > 59 {
		return "", fmt.Errorf("invalid UTC runtime minute")
	}
	return time.Date(2000, 1, 1, hour, minute, 0, 0, time.UTC).Format("15:04"), nil
}

func (r *TaskScheduleRepository) buildTaskSchedule(ctx context.Context, id int64) (*FlixPatrolTaskSchedule, error) {
	row, err := r.q.GetTaskScheduleByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get task schedule %d: %w", id, err)
	}

	targetRows, err := r.q.ListTaskScheduleTargetsByScheduleID(ctx, row.ID)
	if err != nil {
		return nil, fmt.Errorf("list task schedule targets %d: %w", row.ID, err)
	}
	timeRows, err := r.q.ListTaskScheduleRunTimesByScheduleID(ctx, row.ID)
	if err != nil {
		return nil, fmt.Errorf("list task schedule times %d: %w", row.ID, err)
	}

	targets := make([]FlixPatrolTarget, 0, len(targetRows))
	for _, t := range targetRows {
		targets = append(targets, FlixPatrolTarget{
			CountrySlug:  t.CountrySlug,
			ProviderSlug: t.ProviderSlug,
		})
	}
	runTimes := make([]string, 0, len(timeRows))
	for _, rt := range timeRows {
		runTimes = append(runTimes, rt.RunTimeUtc)
	}

	return &FlixPatrolTaskSchedule{
		ID:                  row.ID,
		TaskType:            row.TaskType,
		Name:                row.Name,
		Enabled:             row.Enabled,
		MaxRetries:          row.MaxRetries,
		RequestDelaySeconds: row.RequestDelaySeconds,
		RespectRobots:       row.RespectRobots,
		UserAgent:           row.UserAgent,
		RunTimesUTC:         runTimes,
		Targets:             targets,
	}, nil
}

