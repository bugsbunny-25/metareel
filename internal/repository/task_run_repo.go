package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/bugsbunny-25/metareel/internal/repository/sqlc"
)

type CreateTaskRunInput struct {
	ScheduleID  int64
	TaskType    string
	AsynqTaskID string
	RetryCount  int64
	MaxRetry    int64
}

type TaskRunRecord struct {
	ID           int64
	ScheduleID   int64
	TaskType     string
	AsynqTaskID  string
	Status       string
	RetryCount   int64
	MaxRetry     int64
	StartedAt    time.Time
	FinishedAt   *time.Time
	ErrorMessage *string
}

type TaskRunLogRecord struct {
	ID        int64
	RunID     int64
	Level     string
	Message   string
	CreatedAt time.Time
}

func (r *TaskScheduleRepository) CreateTaskRun(ctx context.Context, in CreateTaskRunInput) (*TaskRunRecord, error) {
	row, err := r.q.CreateTaskRun(ctx, sqlc.CreateTaskRunParams{
		ScheduleID:  in.ScheduleID,
		TaskType:    in.TaskType,
		AsynqTaskID: sql.NullString{String: in.AsynqTaskID, Valid: in.AsynqTaskID != ""},
		Status:      "started",
		RetryCount:  in.RetryCount,
		MaxRetry:    in.MaxRetry,
	})
	if err != nil {
		return nil, fmt.Errorf("create task run: %w", err)
	}
	return toTaskRunRecord(row), nil
}

func (r *TaskScheduleRepository) CompleteTaskRun(ctx context.Context, runID int64, status string, errMsg string) error {
	if status != "succeeded" && status != "failed" {
		return fmt.Errorf("invalid task run status: %s", status)
	}
	return r.q.CompleteTaskRun(ctx, sqlc.CompleteTaskRunParams{
		Status:       status,
		ErrorMessage: sql.NullString{String: errMsg, Valid: errMsg != ""},
		ID:           runID,
	})
}

func (r *TaskScheduleRepository) CreateTaskRunLog(ctx context.Context, runID int64, level string, message string) error {
	return r.q.CreateTaskRunLog(ctx, sqlc.CreateTaskRunLogParams{
		RunID:   runID,
		Level:   level,
		Message: message,
	})
}

func (r *TaskScheduleRepository) ListTaskRunsByScheduleID(ctx context.Context, scheduleID int64, limit int64, offset int64) ([]TaskRunRecord, error) {
	rows, err := r.q.ListTaskRunsByScheduleID(ctx, sqlc.ListTaskRunsByScheduleIDParams{
		ScheduleID: scheduleID,
		Limit:      limit,
		Offset:     offset,
	})
	if err != nil {
		return nil, fmt.Errorf("list task runs: %w", err)
	}
	out := make([]TaskRunRecord, 0, len(rows))
	for _, row := range rows {
		out = append(out, *toTaskRunRecord(row))
	}
	return out, nil
}

// TaskRunFilter narrows ListTaskRuns / CountTaskRuns; zero values match all.
type TaskRunFilter struct {
	TaskType   string
	Status     string
	ScheduleID int64
}

func (r *TaskScheduleRepository) ListTaskRuns(ctx context.Context, f TaskRunFilter, limit int64, offset int64) ([]TaskRunRecord, error) {
	rows, err := r.q.ListTaskRuns(ctx, sqlc.ListTaskRunsParams{
		TaskType:   strings.TrimSpace(f.TaskType),
		Status:     strings.TrimSpace(f.Status),
		ScheduleID: f.ScheduleID,
		Limit:      limit,
		Offset:     offset,
	})
	if err != nil {
		return nil, fmt.Errorf("list task runs: %w", err)
	}
	out := make([]TaskRunRecord, 0, len(rows))
	for _, row := range rows {
		out = append(out, *toTaskRunRecord(row))
	}
	return out, nil
}

func (r *TaskScheduleRepository) CountTaskRuns(ctx context.Context, f TaskRunFilter) (int64, error) {
	n, err := r.q.CountTaskRuns(ctx, sqlc.CountTaskRunsParams{
		TaskType:   strings.TrimSpace(f.TaskType),
		Status:     strings.TrimSpace(f.Status),
		ScheduleID: f.ScheduleID,
	})
	if err != nil {
		return 0, fmt.Errorf("count task runs: %w", err)
	}
	return n, nil
}

// CountTaskRunsByStatusSince counts runs started within the window, keyed
// by status; since is a SQLite datetime modifier such as "-24 hours".
func (r *TaskScheduleRepository) CountTaskRunsByStatusSince(ctx context.Context, since string) (map[string]int64, error) {
	rows, err := r.q.CountTaskRunsByStatusSince(ctx, since)
	if err != nil {
		return nil, fmt.Errorf("count task runs by status: %w", err)
	}
	out := map[string]int64{}
	for _, row := range rows {
		out[row.Status] = row.Runs
	}
	return out, nil
}

// ListLatestTaskRunPerSchedule returns each schedule's most recent run.
func (r *TaskScheduleRepository) ListLatestTaskRunPerSchedule(ctx context.Context) ([]TaskRunRecord, error) {
	rows, err := r.q.ListLatestTaskRunPerSchedule(ctx)
	if err != nil {
		return nil, fmt.Errorf("list latest task runs: %w", err)
	}
	out := make([]TaskRunRecord, 0, len(rows))
	for _, row := range rows {
		out = append(out, *toTaskRunRecord(row))
	}
	return out, nil
}

// ListLastSuccessPerSchedule returns when each schedule last succeeded.
func (r *TaskScheduleRepository) ListLastSuccessPerSchedule(ctx context.Context) (map[int64]time.Time, error) {
	rows, err := r.q.ListLastSuccessPerSchedule(ctx)
	if err != nil {
		return nil, fmt.Errorf("list last successful runs: %w", err)
	}
	out := map[int64]time.Time{}
	for _, row := range rows {
		if t, err := time.Parse("2006-01-02 15:04:05", row.FinishedAt); err == nil {
			out[row.ScheduleID] = t.UTC()
		}
	}
	return out, nil
}

func (r *TaskScheduleRepository) GetTaskRunByID(ctx context.Context, runID int64) (*TaskRunRecord, error) {
	row, err := r.q.GetTaskRunByID(ctx, runID)
	if err != nil {
		return nil, err
	}
	return toTaskRunRecord(row), nil
}

func (r *TaskScheduleRepository) ListTaskRunLogsByRunID(ctx context.Context, runID int64) ([]TaskRunLogRecord, error) {
	rows, err := r.q.ListTaskRunLogsByRunID(ctx, runID)
	if err != nil {
		return nil, fmt.Errorf("list task run logs: %w", err)
	}
	out := make([]TaskRunLogRecord, 0, len(rows))
	for _, row := range rows {
		out = append(out, TaskRunLogRecord{
			ID:        row.ID,
			RunID:     row.RunID,
			Level:     row.Level,
			Message:   row.Message,
			CreatedAt: row.CreatedAt,
		})
	}
	return out, nil
}

func toTaskRunRecord(row sqlc.TaskRun) *TaskRunRecord {
	var asynqTaskID string
	if row.AsynqTaskID.Valid {
		asynqTaskID = row.AsynqTaskID.String
	}
	var finishedAt *time.Time
	if row.FinishedAt.Valid {
		t := row.FinishedAt.Time
		finishedAt = &t
	}
	var errMsg *string
	if row.ErrorMessage.Valid {
		msg := row.ErrorMessage.String
		errMsg = &msg
	}

	return &TaskRunRecord{
		ID:           row.ID,
		ScheduleID:   row.ScheduleID,
		TaskType:     row.TaskType,
		AsynqTaskID:  asynqTaskID,
		Status:       row.Status,
		RetryCount:   row.RetryCount,
		MaxRetry:     row.MaxRetry,
		StartedAt:    row.StartedAt,
		FinishedAt:   finishedAt,
		ErrorMessage: errMsg,
	}
}


// FailStaleTaskRuns marks runs still "started" after olderThan (a SQLite
// datetime modifier such as "-2 hours") as failed, returning how many.
func (r *TaskScheduleRepository) FailStaleTaskRuns(ctx context.Context, olderThan string) (int64, error) {
	n, err := r.q.FailStaleTaskRuns(ctx, olderThan)
	if err != nil {
		return 0, fmt.Errorf("fail stale task runs: %w", err)
	}
	return n, nil
}
