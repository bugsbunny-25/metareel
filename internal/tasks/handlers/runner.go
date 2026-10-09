package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/hibiken/asynq"

	"github.com/bugsbunny-25/metareel/internal/repository"
	"github.com/bugsbunny-25/metareel/internal/service"
)

// Runner records task runs (task_runs + task_run_logs) for every job type and
// alerts when a run's last attempt fails.
type Runner struct {
	log    *slog.Logger
	repo   *repository.TaskScheduleRepository
	alerts *service.AlertService // optional
}

func NewRunner(log *slog.Logger, repo *repository.TaskScheduleRepository, alerts *service.AlertService) *Runner {
	return &Runner{log: log, repo: repo, alerts: alerts}
}

// Run is one task run in progress.
type Run struct {
	ID         int64
	ScheduleID int64
	TaskType   string
	Name       string
	TaskID     string
	Retry      int
	MaxRetry   int
	r          *Runner
	ctx        context.Context
}

// Start creates the run record.
func (r *Runner) Start(ctx context.Context, taskType string, scheduleID int64, name string) (*Run, error) {
	taskID, _ := asynq.GetTaskID(ctx)
	retry, _ := asynq.GetRetryCount(ctx)
	maxRetry, _ := asynq.GetMaxRetry(ctx)
	rec, err := r.repo.CreateTaskRun(ctx, repository.CreateTaskRunInput{
		ScheduleID: scheduleID, TaskType: taskType, AsynqTaskID: taskID,
		RetryCount: int64(retry), MaxRetry: int64(maxRetry),
	})
	if err != nil {
		return nil, err
	}
	run := &Run{ID: rec.ID, ScheduleID: scheduleID, TaskType: taskType, Name: name, TaskID: taskID, Retry: retry, MaxRetry: maxRetry, r: r, ctx: ctx}
	r.log.Info("task started", slog.String("task_type", taskType), slog.Int64("schedule_id", scheduleID),
		slog.String("schedule_name", name), slog.String("task_id", taskID), slog.Int64("run_id", rec.ID),
		slog.Int("retry", retry), slog.Int("max_retry", maxRetry))
	run.Log("info", fmt.Sprintf("task started (%s, attempt %d of %d)", taskType, retry+1, maxRetry+1))
	return run, nil
}

// Log writes a run log line. It uses a fresh context so the log is kept even
// if the task's context was cancelled (e.g. timeout).
func (run *Run) Log(level, message string) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(run.ctx), 5*time.Second)
	defer cancel()
	if err := run.r.repo.CreateTaskRunLog(ctx, run.ID, level, message); err != nil {
		run.r.log.Warn("writing task run log failed", slog.Int64("run_id", run.ID), slog.Any("err", err))
	}
}

// LastAttempt reports whether asynq will not retry this task after a failure.
func (run *Run) LastAttempt() bool { return run.Retry >= run.MaxRetry }

// Finish records the outcome. summary (optional) is stored as JSON. A
// failed or partial last attempt raises an alert; a success clears it.
func (run *Run) Finish(res repository.TaskRunResult, summary any) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(run.ctx), 10*time.Second)
	defer cancel()
	if summary != nil {
		if b, err := json.Marshal(summary); err == nil {
			res.Summary = string(b)
		}
	}
	if res.ErrorMessage != "" {
		run.Log("error", res.ErrorMessage)
	}
	run.Log("info", "task "+res.Status)
	if err := run.r.repo.CompleteTaskRun(ctx, run.ID, res); err != nil {
		run.r.log.Warn("completing task run failed", slog.Int64("run_id", run.ID), slog.Any("err", err))
	}
	attrs := []any{slog.String("task_type", run.TaskType), slog.Int64("schedule_id", run.ScheduleID),
		slog.String("schedule_name", run.Name), slog.Int64("run_id", run.ID), slog.String("status", res.Status)}
	if res.Status == repository.RunStatusSucceeded {
		run.r.log.Info("task finished", attrs...)
	} else {
		run.r.log.Error("task finished", append(attrs, slog.String("err", res.ErrorMessage))...)
	}

	key := fmt.Sprintf("run:%s:%d", run.TaskType, run.ScheduleID)
	switch {
	case res.Status == repository.RunStatusSucceeded:
		run.r.alerts.Resolve(ctx, key)
	case run.LastAttempt():
		name := run.Name
		if name == "" {
			name = run.TaskType
		}
		msg := res.ErrorMessage
		if res.TargetsTotal > 0 {
			msg = fmt.Sprintf("%d of %d targets failed. %s", res.TargetsFailed, res.TargetsTotal, msg)
		}
		_, _ = run.r.alerts.Send(ctx, service.Alert{
			Key: key, Level: "error", Title: fmt.Sprintf("%s run %s (run %d)", name, res.Status, run.ID), Message: msg,
		})
	}
}

// statusFor maps succeeded / failed counts to a run status.
func statusFor(succeeded, failed int) string {
	switch {
	case failed == 0:
		return repository.RunStatusSucceeded
	case succeeded > 0:
		return repository.RunStatusPartial
	default:
		return repository.RunStatusFailed
	}
}
