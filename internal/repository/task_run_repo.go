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

func (r *TaskScheduleRepository) ListTaskRuns(ctx context.Context, taskType string, limit int64, offset int64) ([]TaskRunRecord, error) {
	typeFilter := int64(0)
	trimmedType := strings.TrimSpace(taskType)
	if trimmedType != "" {
		typeFilter = 1
	}

	rows, err := r.q.ListTaskRuns(ctx, sqlc.ListTaskRunsParams{
		Column1:  typeFilter,
		TaskType: trimmedType,
		Limit:    limit,
		Offset:   offset,
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

