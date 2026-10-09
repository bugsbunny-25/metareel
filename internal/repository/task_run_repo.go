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
	ScheduleID  int64 // 0 for ad-hoc runs (no schedule)
	TaskType    string
	AsynqTaskID string
	RetryCount  int64
	MaxRetry    int64
}

type TaskRunRecord struct {
	ID               int64
	ScheduleID       int64 // 0 for ad-hoc runs
	TaskType         string
	AsynqTaskID      string
	Status           string
	RetryCount       int64
	MaxRetry         int64
	StartedAt        time.Time
	FinishedAt       *time.Time
	ErrorMessage     *string
	TargetsTotal     int64
	TargetsSucceeded int64
	TargetsFailed    int64
	TargetsSkipped   int64
	Summary          *string // JSON
}

// Task run statuses. A partial run finished some of its targets and failed
// others.
const (
	RunStatusStarted   = "started"
	RunStatusSucceeded = "succeeded"
	RunStatusPartial   = "partial"
	RunStatusFailed    = "failed"
)

// TaskRunResult is how a run finished.
type TaskRunResult struct {
	Status           string
	ErrorMessage     string
	TargetsTotal     int64
	TargetsSucceeded int64
	TargetsFailed    int64
	TargetsSkipped   int64
	Summary          string // JSON, optional
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
		ScheduleID:  sql.NullInt64{Int64: in.ScheduleID, Valid: in.ScheduleID > 0},
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

func (r *TaskScheduleRepository) CompleteTaskRun(ctx context.Context, runID int64, res TaskRunResult) error {
	switch res.Status {
	case RunStatusSucceeded, RunStatusPartial, RunStatusFailed:
	default:
		return fmt.Errorf("invalid task run status: %s", res.Status)
	}
	return r.q.CompleteTaskRun(ctx, sqlc.CompleteTaskRunParams{
		Status:           res.Status,
		ErrorMessage:     sql.NullString{String: res.ErrorMessage, Valid: res.ErrorMessage != ""},
		TargetsTotal:     res.TargetsTotal,
		TargetsSucceeded: res.TargetsSucceeded,
		TargetsFailed:    res.TargetsFailed,
		TargetsSkipped:   res.TargetsSkipped,
		Summary:          sql.NullString{String: res.Summary, Valid: res.Summary != ""},
		ID:               runID,
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
		ScheduleID: sql.NullInt64{Int64: scheduleID, Valid: true},
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
	var summary *string
	if row.Summary.Valid {
		v := row.Summary.String
		summary = &v
	}

	return &TaskRunRecord{
		ID:               row.ID,
		ScheduleID:       row.ScheduleID.Int64,
		TaskType:         row.TaskType,
		AsynqTaskID:      asynqTaskID,
		Status:           row.Status,
		RetryCount:       row.RetryCount,
		MaxRetry:         row.MaxRetry,
		StartedAt:        row.StartedAt,
		FinishedAt:       finishedAt,
		ErrorMessage:     errMsg,
		TargetsTotal:     row.TargetsTotal,
		TargetsSucceeded: row.TargetsSucceeded,
		TargetsFailed:    row.TargetsFailed,
		TargetsSkipped:   row.TargetsSkipped,
		Summary:          summary,
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

// HasRunningTaskRun reports whether the schedule has a run started within
// since (a SQLite datetime modifier such as "-2 hours") that has not finished.
func (r *TaskScheduleRepository) HasRunningTaskRun(ctx context.Context, scheduleID int64, since string) (bool, error) {
	n, err := r.q.CountRunningTaskRuns(ctx, sqlc.CountRunningTaskRunsParams{
		ScheduleID: sql.NullInt64{Int64: scheduleID, Valid: scheduleID > 0},
		Since:      since,
	})
	if err != nil {
		return false, fmt.Errorf("count running task runs: %w", err)
	}
	return n > 0, nil
}

// PruneTaskRuns deletes finished runs (and their logs) started before
// olderThan, a SQLite datetime modifier such as "-90 days".
func (r *TaskScheduleRepository) PruneTaskRuns(ctx context.Context, olderThan string) (runs int64, logs int64, err error) {
	if logs, err = r.q.DeleteTaskRunLogsBefore(ctx, olderThan); err != nil {
		return 0, 0, fmt.Errorf("delete task run logs: %w", err)
	}
	if runs, err = r.q.DeleteTaskRunsBefore(ctx, olderThan); err != nil {
		return 0, logs, fmt.Errorf("delete task runs: %w", err)
	}
	return runs, logs, nil
}
