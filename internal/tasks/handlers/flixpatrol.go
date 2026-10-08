package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/hibiken/asynq"

	"github.com/bugsbunny-25/metareel/internal/repository"
	"github.com/bugsbunny-25/metareel/internal/scraper/flixpatrol"
	"github.com/bugsbunny-25/metareel/internal/service"
	"github.com/bugsbunny-25/metareel/internal/tasks"
)

type FlixPatrolHandler struct {
	log  *slog.Logger
	job  *service.FlixPatrolJob
	repo *repository.TaskScheduleRepository
}

func NewFlixPatrolHandler(log *slog.Logger, repo *repository.TaskScheduleRepository, job *service.FlixPatrolJob) *FlixPatrolHandler {
	return &FlixPatrolHandler{log: log, repo: repo, job: job}
}

func (h *FlixPatrolHandler) ProcessTask(ctx context.Context, t *asynq.Task) error {
	var payload tasks.FlixPatrolTop10Payload
	if err := json.Unmarshal(t.Payload(), &payload); err != nil {
		return fmt.Errorf("unmarshal flixpatrol task payload: %w", err)
	}
	if len(payload.Targets) == 0 {
		return fmt.Errorf("schedule %d has no targets in payload", payload.ScheduleID)
	}

	taskID, _ := asynq.GetTaskID(ctx)
	retryCount, _ := asynq.GetRetryCount(ctx)
	maxRetry, _ := asynq.GetMaxRetry(ctx)

	run, err := h.repo.CreateTaskRun(ctx, repository.CreateTaskRunInput{
		ScheduleID:  payload.ScheduleID,
		TaskType:    tasks.TypeFlixPatrolScheduleRun,
		AsynqTaskID: taskID,
		RetryCount:  int64(retryCount),
		MaxRetry:    int64(maxRetry),
	})
	if err != nil {
		return err
	}
	_ = h.repo.CreateTaskRunLog(ctx, run.ID, "info", fmt.Sprintf("task started (schedule=%d, task_id=%s)", payload.ScheduleID, taskID))
	h.log.Info("flixpatrol task started",
		slog.Int64("schedule_id", payload.ScheduleID),
		slog.String("schedule_name", payload.ScheduleName),
		slog.String("task_id", taskID),
		slog.Int64("run_id", run.ID),
		slog.Int("retry", retryCount),
		slog.Int("max_retry", maxRetry),
		slog.Int("targets", len(payload.Targets)))

	targets := make([]service.FlixPatrolScrapeTarget, 0, len(payload.Targets))
	for _, target := range payload.Targets {
		targets = append(targets, service.FlixPatrolScrapeTarget{
			CountrySlug: target.CountrySlug,
			Provider:    flixpatrol.Provider(target.ProviderSlug),
		})
	}

	opts := service.FlixPatrolRunOptions{
		RequestDelay:  time.Duration(payload.RequestDelaySeconds) * time.Second,
		UserAgent:     payload.UserAgent,
		RespectRobots: payload.RespectRobots,
		RunLog: func(level, message string) {
			// Use a fresh context so the log is kept even if the task's
			// context was cancelled (e.g. timeout).
			logCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
			defer cancel()
			if err := h.repo.CreateTaskRunLog(logCtx, run.ID, level, message); err != nil {
				h.log.Warn("writing task run log failed", slog.Int64("run_id", run.ID), slog.Any("err", err))
			}
		},
	}
	_ = h.repo.CreateTaskRunLog(ctx, run.ID, "info", fmt.Sprintf("running %d target(s)", len(targets)))

	if err := h.job.RunTargets(ctx, targets, time.Now().UTC(), opts); err != nil {
		_ = h.repo.CreateTaskRunLog(ctx, run.ID, "error", err.Error())
		_ = h.repo.CompleteTaskRun(ctx, run.ID, "failed", err.Error())
		h.log.Error("flixpatrol task failed",
			slog.Int64("schedule_id", payload.ScheduleID),
			slog.String("task_id", taskID),
			slog.Int64("run_id", run.ID),
			slog.String("schedule_name", payload.ScheduleName),
			slog.Int("retry", retryCount),
			slog.Int("max_retry", maxRetry),
			slog.Any("err", err))
		return err
	}
	_ = h.repo.CreateTaskRunLog(ctx, run.ID, "info", "task succeeded")
	_ = h.repo.CompleteTaskRun(ctx, run.ID, "succeeded", "")

	h.log.Info("flixpatrol task completed", slog.Int64("schedule_id", payload.ScheduleID), slog.String("schedule_name", payload.ScheduleName), slog.String("task_id", taskID), slog.Int64("run_id", run.ID))
	return nil
}

func (h *FlixPatrolHandler) Register(mux *asynq.ServeMux) {
	mux.HandleFunc(tasks.TypeFlixPatrolScheduleRun, h.ProcessTask)
}

