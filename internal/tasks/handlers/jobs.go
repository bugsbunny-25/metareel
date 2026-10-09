package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/hibiken/asynq"

	"github.com/bugsbunny-25/metareel/internal/repository"
	"github.com/bugsbunny-25/metareel/internal/service"
	"github.com/bugsbunny-25/metareel/internal/tasks"
)

// JobFunc runs one job and returns a summary (stored as JSON on the run).
type JobFunc func(ctx context.Context, p tasks.JobPayload, run *Run) (any, error)

// JobHandler runs every non-FlixPatrol task type through the Runner.
type JobHandler struct {
	runner *Runner
	jobs   map[string]JobFunc
}

func NewJobHandler(runner *Runner) *JobHandler {
	return &JobHandler{runner: runner, jobs: map[string]JobFunc{}}
}

// Handle registers fn for a task type (nil fn = not configured; skipped).
func (h *JobHandler) Handle(taskType string, fn JobFunc) *JobHandler {
	if fn != nil {
		h.jobs[taskType] = fn
	}
	return h
}

func (h *JobHandler) Register(mux *asynq.ServeMux) {
	for taskType := range h.jobs {
		mux.HandleFunc(taskType, h.process)
	}
}

func (h *JobHandler) process(ctx context.Context, t *asynq.Task) error {
	fn := h.jobs[t.Type()]
	var p tasks.JobPayload
	if err := json.Unmarshal(t.Payload(), &p); err != nil {
		return fmt.Errorf("unmarshal %s payload: %w: %w", t.Type(), err, asynq.SkipRetry)
	}
	run, err := h.runner.Start(ctx, t.Type(), p.ScheduleID, p.ScheduleName)
	if err != nil {
		return err
	}
	if p.Reason != "" {
		run.Log("info", p.Reason)
	}
	summary, jobErr := fn(ctx, p, run)
	res := repository.TaskRunResult{Status: repository.RunStatusSucceeded}
	if jobErr != nil {
		res.Status, res.ErrorMessage = repository.RunStatusFailed, jobErr.Error()
	}
	run.Finish(res, summary)
	return jobErr
}

// Job functions, adapting each service to JobFunc.

func EnrichJob(e *service.TitleEnricher, userAgent string) JobFunc {
	return func(ctx context.Context, p tasks.JobPayload, run *Run) (any, error) {
		return e.Run(ctx, service.EnrichOptions{
			TitleIDs:      p.TitleIDs,
			RequestDelay:  time.Duration(p.RequestDelaySeconds) * time.Second,
			UserAgent:     userAgent,
			RespectRobots: true,
			RunLog:        run.Log,
		})
	}
}

func MetadataJob(m *service.TitleMetadataService) JobFunc {
	return func(ctx context.Context, _ tasks.JobPayload, run *Run) (any, error) {
		return m.Run(ctx, run.Log)
	}
}

func RatingsPrewarmJob(r *service.RatingsService, titles *repository.FlixPatrolRepository) JobFunc {
	return func(ctx context.Context, _ tasks.JobPayload, run *Run) (any, error) {
		return r.Prewarm(ctx, titles, run.Log)
	}
}

func NetflixImportJob(n *service.NetflixImporter) JobFunc {
	return func(ctx context.Context, p tasks.JobPayload, run *Run) (any, error) {
		return n.Run(ctx, p.Force, run.Log)
	}
}

func IMDbImportJob(i *service.IMDbImporter) JobFunc {
	return func(ctx context.Context, p tasks.JobPayload, run *Run) (any, error) {
		return i.Run(ctx, p.Force, run.Log)
	}
}

func MaintenanceJob(m *service.MaintenanceService) JobFunc {
	return func(ctx context.Context, _ tasks.JobPayload, run *Run) (any, error) {
		return m.Run(ctx, run.Log)
	}
}
