package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/hibiken/asynq"

	"github.com/bugsbunny-25/metareel/internal/repository"
	"github.com/bugsbunny-25/metareel/internal/scraper/flixpatrol"
	"github.com/bugsbunny-25/metareel/internal/service"
	"github.com/bugsbunny-25/metareel/internal/tasks"
)

// recheckInterval is how often a target whose chart FlixPatrol has not
// published yet is checked again (within the schedule's recheck_hours).
const recheckInterval = time.Hour

type FlixPatrolHandler struct {
	log    *slog.Logger
	runner *Runner
	job    *service.FlixPatrolJob
	asynq  *asynq.Client // enqueues re-checks and enrichment; optional
	queue  string
}

func NewFlixPatrolHandler(log *slog.Logger, runner *Runner, job *service.FlixPatrolJob, asynqClient *asynq.Client, queue string) *FlixPatrolHandler {
	return &FlixPatrolHandler{log: log, runner: runner, job: job, asynq: asynqClient, queue: queue}
}

func (h *FlixPatrolHandler) ProcessTask(ctx context.Context, t *asynq.Task) error {
	var payload tasks.FlixPatrolTop10Payload
	if err := json.Unmarshal(t.Payload(), &payload); err != nil {
		return fmt.Errorf("unmarshal flixpatrol task payload: %w: %w", err, asynq.SkipRetry)
	}
	if len(payload.Targets) == 0 {
		return fmt.Errorf("schedule %d has no targets in payload: %w", payload.ScheduleID, asynq.SkipRetry)
	}

	run, err := h.runner.Start(ctx, tasks.TypeFlixPatrolScheduleRun, payload.ScheduleID, payload.ScheduleName)
	if err != nil {
		return err
	}

	targets := make([]service.FlixPatrolScrapeTarget, 0, len(payload.Targets))
	for _, target := range payload.Targets {
		targets = append(targets, service.FlixPatrolScrapeTarget{
			CountrySlug: target.CountrySlug,
			Provider:    flixpatrol.Provider(target.ProviderSlug),
		})
	}

	now := time.Now().UTC()
	recheckUntil, err := recheckDeadline(payload, now)
	if err != nil {
		run.Finish(repository.TaskRunResult{Status: repository.RunStatusFailed, ErrorMessage: err.Error()}, nil)
		return fmt.Errorf("%w: %w", err, asynq.SkipRetry)
	}
	opts := service.FlixPatrolRunOptions{
		RequestDelay:  time.Duration(payload.RequestDelaySeconds) * time.Second,
		UserAgent:     payload.UserAgent,
		RespectRobots: payload.RespectRobots,
		RunID:         run.ID,
		Force:         payload.Force,
		BackfillDays:  int(payload.BackfillDays),
		// Store an unchanged chart once re-checking is off or over.
		AcceptUnchanged: recheckUntil.IsZero() || !now.Add(recheckInterval).Before(recheckUntil),
		RunLog:          run.Log,
	}
	for _, d := range payload.Dates {
		date, err := time.Parse(repository.DateLayout, d)
		if err != nil {
			run.Finish(repository.TaskRunResult{Status: repository.RunStatusFailed, ErrorMessage: fmt.Sprintf("invalid date %q", d)}, nil)
			return fmt.Errorf("invalid date %q: %w", d, asynq.SkipRetry)
		}
		opts.Dates = append(opts.Dates, date)
	}
	run.Log("info", fmt.Sprintf("running %d target(s)%s", len(targets), describeRun(payload)))

	report, runErr := h.job.Run(ctx, targets, opts)
	succeeded := report.Count(service.TargetSucceeded)
	failed := report.Count(service.TargetFailed)
	skipped := report.Count(service.TargetSkipped) + report.Count(service.TargetNotFresh)
	res := repository.TaskRunResult{
		Status:           statusFor(succeeded+skipped, failed),
		TargetsTotal:     int64(len(report.Results)),
		TargetsSucceeded: int64(succeeded),
		TargetsFailed:    int64(failed),
		TargetsSkipped:   int64(skipped),
	}
	if runErr != nil {
		res.Status, res.ErrorMessage = repository.RunStatusFailed, runErr.Error()
	} else if failed > 0 {
		res.ErrorMessage = fmt.Sprintf("%d of %d chart pages failed", failed, len(report.Results))
	}

	// Match new titles now rather than waiting for the nightly backlog.
	if len(report.NewTitleIDs) > 0 {
		h.enqueueEnrich(ctx, run, payload, report.NewTitleIDs)
	}

	retrying := res.Status != repository.RunStatusSucceeded && !run.LastAttempt()
	if notFresh := report.NotFresh(); len(notFresh) > 0 && !retrying && runErr == nil {
		h.enqueueRecheck(ctx, run, payload, notFresh, recheckUntil, now)
	}
	run.Finish(res, report)

	if runErr != nil {
		return runErr
	}
	if failed > 0 {
		// asynq retries the task; stored charts are skipped, so only the
		// failed pages are fetched again.
		return errors.New(res.ErrorMessage)
	}
	return nil
}

// recheckDeadline is when to stop re-checking a chart FlixPatrol has not
// updated: the payload's deadline on follow-ups, else now + RecheckHours
// (zero = re-checking off).
func recheckDeadline(p tasks.FlixPatrolTop10Payload, now time.Time) (time.Time, error) {
	if p.RecheckUntil != "" {
		t, err := time.Parse(time.RFC3339, p.RecheckUntil)
		if err != nil {
			return time.Time{}, fmt.Errorf("invalid recheck_until %q", p.RecheckUntil)
		}
		return t, nil
	}
	if p.RecheckHours <= 0 || len(p.Dates) > 0 {
		return time.Time{}, nil
	}
	return now.Add(time.Duration(p.RecheckHours) * time.Hour), nil
}

func describeRun(p tasks.FlixPatrolTop10Payload) string {
	switch {
	case len(p.Dates) > 0:
		return fmt.Sprintf(" for %d date(s) %s…%s", len(p.Dates), p.Dates[0], p.Dates[len(p.Dates)-1])
	case p.RecheckUntil != "":
		return " (re-check for an unpublished chart, until " + p.RecheckUntil + ")"
	case p.BackfillDays > 0:
		return fmt.Sprintf(", filling gaps from the last %d days", p.BackfillDays)
	}
	return ""
}

func (h *FlixPatrolHandler) enqueueEnrich(ctx context.Context, run *Run, p tasks.FlixPatrolTop10Payload, ids []int64) {
	if h.asynq == nil {
		return
	}
	task, err := tasks.NewJobTask(tasks.TypeTitlesEnrich, tasks.JobPayload{
		TitleIDs: ids, RequestDelaySeconds: p.RequestDelaySeconds, Reason: fmt.Sprintf("new titles from run %d", run.ID),
	})
	if err == nil {
		_, err = h.asynq.EnqueueContext(ctx, task, tasks.Options(tasks.TypeTitlesEnrich, 1, h.queue)...)
	}
	if err != nil {
		run.Log("warn", fmt.Sprintf("could not queue matching of %d new titles (the nightly matching job will pick them up): %v", len(ids), err))
		return
	}
	run.Log("info", fmt.Sprintf("queued matching of %d new title(s)", len(ids)))
}

func (h *FlixPatrolHandler) enqueueRecheck(ctx context.Context, run *Run, p tasks.FlixPatrolTop10Payload, targets []service.FlixPatrolScrapeTarget, until, now time.Time) {
	if h.asynq == nil || until.IsZero() || !now.Before(until) {
		return
	}
	next := tasks.FlixPatrolTop10Payload{
		ScheduleID: p.ScheduleID, ScheduleName: p.ScheduleName, RequestDelaySeconds: p.RequestDelaySeconds,
		UserAgent: p.UserAgent, RespectRobots: p.RespectRobots, RecheckUntil: until.UTC().Format(time.RFC3339),
	}
	for _, t := range targets {
		next.Targets = append(next.Targets, tasks.FlixPatrolTop10TargetPayload{CountrySlug: t.CountrySlug, ProviderSlug: string(t.Provider)})
	}
	at := now.Add(recheckInterval)
	if at.After(until) {
		at = until
	}
	task, err := tasks.NewFlixPatrolTop10Task(next)
	if err == nil {
		opts := append(tasks.Options(tasks.TypeFlixPatrolScheduleRun, 1, h.queue), asynq.ProcessAt(at))
		_, err = h.asynq.EnqueueContext(ctx, task, opts...)
	}
	if err != nil && !errors.Is(err, asynq.ErrDuplicateTask) {
		run.Log("warn", fmt.Sprintf("could not queue a re-check: %v", err))
		return
	}
	run.Log("info", fmt.Sprintf("%d target(s) not published yet; checking again at %s UTC", len(targets), at.Format("15:04")))
}

func (h *FlixPatrolHandler) Register(mux *asynq.ServeMux) {
	mux.HandleFunc(tasks.TypeFlixPatrolScheduleRun, h.ProcessTask)
}
