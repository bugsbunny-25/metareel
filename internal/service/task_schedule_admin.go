package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/hibiken/asynq"

	"github.com/bugsbunny-25/metareel/internal/repository"
	"github.com/bugsbunny-25/metareel/internal/scraper/flixpatrol"
	"github.com/bugsbunny-25/metareel/internal/tasks"
)

const TaskTypeFlixPatrolTop10 = "flixpatrol.top10.scrape"

type ConflictError struct {
	Message string
}

func (e *ConflictError) Error() string { return e.Message }

type TaskScheduleAdminService struct {
	repo      *repository.TaskScheduleRepository
	asynq     *asynq.Client
	queueName string
}

func NewTaskScheduleAdminService(repo *repository.TaskScheduleRepository, asynqClient *asynq.Client, queueName string) *TaskScheduleAdminService {
	return &TaskScheduleAdminService{
		repo:      repo,
		asynq:     asynqClient,
		queueName: queueName,
	}
}

type CreateTaskScheduleInput struct {
	TaskType            string          `json:"task_type"`
	Name                string          `json:"name"`
	Enabled             *bool           `json:"enabled,omitempty"`
	MaxRetries          *int64          `json:"max_retries,omitempty"`
	RequestDelaySeconds *int64          `json:"request_delay_seconds,omitempty"`
	RespectRobots       *bool           `json:"respect_robots,omitempty"`
	UserAgent           *string         `json:"user_agent,omitempty"`
	UTCRuntimes         []string        `json:"utc_runtimes"`
	TaskDetails         json.RawMessage `json:"task_details,omitempty"`
}

type AddFlixPatrolTargetsInput struct {
	Targets []struct {
		CountryCode  string `json:"country_code"`
		ProviderSlug string `json:"provider_slug"`
	} `json:"targets"`
}

type TaskScheduleResponse struct {
	ID                  int64                     `json:"id"`
	TaskType            string                    `json:"task_type"`
	Name                string                    `json:"name"`
	Enabled             bool                      `json:"enabled"`
	MaxRetries          int64                     `json:"max_retries"`
	RequestDelaySeconds int64                     `json:"request_delay_seconds"`
	RespectRobots       bool                      `json:"respect_robots"`
	UserAgent           string                    `json:"user_agent"`
	UTCRuntimes         []string                  `json:"utc_runtimes"`
	FlixPatrolTargets   []FlixPatrolTargetPayload `json:"flixpatrol_targets,omitempty"`
}

type TaskScheduleListResponse struct {
	ID          int64    `json:"id"`
	TaskType    string   `json:"task_type"`
	Name        string   `json:"name"`
	Enabled     bool     `json:"enabled"`
	UTCRuntimes []string `json:"utc_runtimes"`
}

type FlixPatrolTargetPayload struct {
	CountryCode  string `json:"country_code"`
	ProviderSlug string `json:"provider_slug"`
}

type RunTaskNowResponse struct {
	ScheduleID int64  `json:"schedule_id"`
	TaskType   string `json:"task_type"`
	Queue      string `json:"queue"`
	Status     string `json:"status"`
}

type TaskRunResponse struct {
	ID           int64   `json:"id"`
	ScheduleID   int64   `json:"schedule_id"`
	TaskType     string  `json:"task_type"`
	AsynqTaskID  string  `json:"asynq_task_id,omitempty"`
	Status       string  `json:"status"`
	RetryCount   int64   `json:"retry_count"`
	MaxRetry     int64   `json:"max_retry"`
	StartedAt    string  `json:"started_at"`
	FinishedAt   *string `json:"finished_at,omitempty"`
	ErrorMessage *string `json:"error_message,omitempty"`
}

type TaskRunLogResponse struct {
	ID        int64  `json:"id"`
	RunID     int64  `json:"run_id"`
	Level     string `json:"level"`
	Message   string `json:"message"`
	CreatedAt string `json:"created_at"`
}

func (s *TaskScheduleAdminService) ListTaskSchedules(ctx context.Context, enabled *bool, taskType string) ([]TaskScheduleListResponse, error) {
	rows, err := s.repo.ListTaskSchedules(ctx, enabled, taskType)
	if err != nil {
		return nil, err
	}
	out := make([]TaskScheduleListResponse, 0, len(rows))
	for _, r := range rows {
		out = append(out, TaskScheduleListResponse{
			ID:          r.ID,
			TaskType:    r.TaskType,
			Name:        r.Name,
			Enabled:     r.Enabled,
			UTCRuntimes: r.RunTimesUTC,
		})
	}
	return out, nil
}

func (s *TaskScheduleAdminService) GetTaskScheduleByID(ctx context.Context, scheduleID int64) (*TaskScheduleResponse, error) {
	if scheduleID <= 0 {
		return nil, &ValidationError{Message: "invalid schedule id"}
	}
	return s.getFlixResponse(ctx, scheduleID)
}

func (s *TaskScheduleAdminService) RunTaskScheduleNow(ctx context.Context, scheduleID int64) (*RunTaskNowResponse, error) {
	if scheduleID <= 0 {
		return nil, &ValidationError{Message: "invalid schedule id"}
	}
	schedule, err := s.repo.GetTaskScheduleByID(ctx, scheduleID)
	if err != nil {
		return nil, err
	}
	if !schedule.Enabled {
		return nil, &ValidationError{Message: "cannot run disabled schedule"}
	}

	var task *asynq.Task
	switch schedule.TaskType {
	case TaskTypeFlixPatrolTop10:
		payload := tasks.FlixPatrolTop10Payload{
			ScheduleID:          schedule.ID,
			ScheduleName:        schedule.Name,
			RequestDelaySeconds: schedule.RequestDelaySeconds,
			UserAgent:           schedule.UserAgent,
			RespectRobots:       schedule.RespectRobots,
			Targets:             make([]tasks.FlixPatrolTop10TargetPayload, 0, len(schedule.Targets)),
		}
		for _, target := range schedule.Targets {
			payload.Targets = append(payload.Targets, tasks.FlixPatrolTop10TargetPayload{
				CountrySlug:  target.CountrySlug,
				ProviderSlug: target.ProviderSlug,
			})
		}
		task, err = tasks.NewFlixPatrolTop10Task(payload)
		if err != nil {
			return nil, &ValidationError{Message: err.Error()}
		}
	default:
		return nil, &ValidationError{Message: fmt.Sprintf("unsupported task_type: %s", schedule.TaskType)}
	}

	if s.asynq == nil {
		return nil, fmt.Errorf("asynq client not configured")
	}
	enqueueOpts := []asynq.Option{
		asynq.MaxRetry(int(schedule.MaxRetries) * len(schedule.Targets)),
	}
	if s.queueName != "" {
		enqueueOpts = append(enqueueOpts, asynq.Queue(s.queueName))
	}
	if _, err := s.asynq.EnqueueContext(ctx, task, enqueueOpts...); err != nil {
		return nil, fmt.Errorf("enqueue task: %w", err)
	}

	return &RunTaskNowResponse{
		ScheduleID: schedule.ID,
		TaskType:   schedule.TaskType,
		Queue:      s.queueName,
		Status:     "enqueued",
	}, nil
}

func (s *TaskScheduleAdminService) ListTaskRunsByScheduleID(ctx context.Context, scheduleID int64, limit int64, offset int64) ([]TaskRunResponse, error) {
	if scheduleID <= 0 {
		return nil, &ValidationError{Message: "invalid schedule id"}
	}
	if limit <= 0 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	rows, err := s.repo.ListTaskRunsByScheduleID(ctx, scheduleID, limit, offset)
	if err != nil {
		return nil, err
	}
	out := make([]TaskRunResponse, 0, len(rows))
	for _, row := range rows {
		out = append(out, toTaskRunResponse(row))
	}
	return out, nil
}

func (s *TaskScheduleAdminService) ListTaskRuns(ctx context.Context, taskType string, limit int64, offset int64) ([]TaskRunResponse, error) {
	if limit <= 0 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	rows, err := s.repo.ListTaskRuns(ctx, taskType, limit, offset)
	if err != nil {
		return nil, err
	}
	out := make([]TaskRunResponse, 0, len(rows))
	for _, row := range rows {
		out = append(out, toTaskRunResponse(row))
	}
	return out, nil
}

func (s *TaskScheduleAdminService) GetTaskRunByID(ctx context.Context, runID int64) (*TaskRunResponse, error) {
	if runID <= 0 {
		return nil, &ValidationError{Message: "invalid run id"}
	}
	row, err := s.repo.GetTaskRunByID(ctx, runID)
	if err != nil {
		return nil, err
	}
	resp := toTaskRunResponse(*row)
	return &resp, nil
}

func (s *TaskScheduleAdminService) ListTaskRunLogsByRunID(ctx context.Context, runID int64) ([]TaskRunLogResponse, error) {
	if runID <= 0 {
		return nil, &ValidationError{Message: "invalid run id"}
	}
	rows, err := s.repo.ListTaskRunLogsByRunID(ctx, runID)
	if err != nil {
		return nil, err
	}
	out := make([]TaskRunLogResponse, 0, len(rows))
	for _, row := range rows {
		out = append(out, TaskRunLogResponse{
			ID:        row.ID,
			RunID:     row.RunID,
			Level:     row.Level,
			Message:   row.Message,
			CreatedAt: row.CreatedAt.UTC().Format(time.RFC3339),
		})
	}
	return out, nil
}

func (s *TaskScheduleAdminService) CreateTaskSchedule(ctx context.Context, in CreateTaskScheduleInput) (*TaskScheduleResponse, error) {
	taskType := strings.TrimSpace(in.TaskType)
	if taskType == "" {
		return nil, &ValidationError{Message: "task_type is required"}
	}
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return nil, &ValidationError{Message: "name is required"}
	}
	if len(in.UTCRuntimes) == 0 {
		return nil, &ValidationError{Message: "utc_runtimes must have at least one value"}
	}

	runTimes := make([]string, 0, len(in.UTCRuntimes))
	for _, rt := range in.UTCRuntimes {
		normalized, err := repository.NormalizeUTCRuntime(rt)
		if err != nil {
			return nil, &ValidationError{Message: err.Error()}
		}
		runTimes = append(runTimes, normalized)
	}

	createIn := repository.CreateTaskScheduleInput{
		TaskType:            taskType,
		Name:                name,
		Enabled:             withDefaultBool(in.Enabled, true),
		MaxRetries:          withDefaultInt64(in.MaxRetries, 3),
		RequestDelaySeconds: withDefaultInt64(in.RequestDelaySeconds, 10),
		RespectRobots:       withDefaultBool(in.RespectRobots, true),
		UserAgent:           withDefaultString(in.UserAgent, "metareel-flixpatrol-bot/0.1"),
		UTCRunTimes:         runTimes,
	}

	scheduleID, err := s.repo.CreateTaskSchedule(ctx, createIn)
	if err != nil {
		if repository.IsUniqueConstraintError(err) {
			return nil, &ConflictError{Message: "task schedule with this name already exists"}
		}
		return nil, err
	}

	if taskType == TaskTypeFlixPatrolTop10 {
		targets, err := decodeFlixTargets(in.TaskDetails)
		if err != nil {
			return nil, err
		}
		if len(targets) == 0 {
			return nil, &ValidationError{Message: "task_details.targets is required for flixpatrol.top10.scrape"}
		}
		if err := s.repo.AddFlixPatrolTargets(ctx, scheduleID, targets); err != nil {
			return nil, err
		}
		return s.getFlixResponse(ctx, scheduleID)
	}

	return nil, &ValidationError{Message: fmt.Sprintf("unsupported task_type: %s", taskType)}
}

type UpdateTaskScheduleInput struct {
	Name                string   `json:"name"`
	Enabled             bool     `json:"enabled"`
	MaxRetries          *int64   `json:"max_retries,omitempty"`
	RequestDelaySeconds *int64   `json:"request_delay_seconds,omitempty"`
	RespectRobots       *bool    `json:"respect_robots,omitempty"`
	UserAgent           *string  `json:"user_agent,omitempty"`
	UTCRuntimes         []string `json:"utc_runtimes"`
}

func (s *TaskScheduleAdminService) UpdateTaskSchedule(ctx context.Context, id int64, in UpdateTaskScheduleInput) (*TaskScheduleResponse, error) {
	if id <= 0 {
		return nil, &ValidationError{Message: "invalid schedule id"}
	}
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return nil, &ValidationError{Message: "name is required"}
	}
	if len(in.UTCRuntimes) == 0 {
		return nil, &ValidationError{Message: "utc_runtimes must have at least one value"}
	}

	runTimes := make([]string, 0, len(in.UTCRuntimes))
	for _, rt := range in.UTCRuntimes {
		normalized, err := repository.NormalizeUTCRuntime(rt)
		if err != nil {
			return nil, &ValidationError{Message: err.Error()}
		}
		runTimes = append(runTimes, normalized)
	}

	existing, err := s.repo.GetTaskScheduleByID(ctx, id)
	if err != nil {
		return nil, err
	}

	updated, err := s.repo.UpdateTaskSchedule(ctx, id, repository.UpdateTaskScheduleInput{
		Name:                name,
		Enabled:             in.Enabled,
		MaxRetries:          withDefaultInt64(in.MaxRetries, existing.MaxRetries),
		RequestDelaySeconds: withDefaultInt64(in.RequestDelaySeconds, existing.RequestDelaySeconds),
		RespectRobots:       withDefaultBool(in.RespectRobots, existing.RespectRobots),
		UserAgent:           withDefaultString(in.UserAgent, existing.UserAgent),
		UTCRunTimes:         runTimes,
	})
	if err != nil {
		if repository.IsUniqueConstraintError(err) {
			return nil, &ConflictError{Message: "task schedule with this name already exists"}
		}
		return nil, err
	}

	resp := &TaskScheduleResponse{
		ID:                  updated.ID,
		TaskType:            updated.TaskType,
		Name:                updated.Name,
		Enabled:             updated.Enabled,
		MaxRetries:          updated.MaxRetries,
		RequestDelaySeconds: updated.RequestDelaySeconds,
		RespectRobots:       updated.RespectRobots,
		UserAgent:           updated.UserAgent,
		UTCRuntimes:         updated.RunTimesUTC,
		FlixPatrolTargets:   make([]FlixPatrolTargetPayload, 0, len(updated.Targets)),
	}
	for _, t := range updated.Targets {
		code, _ := flixpatrol.GetCountryCode(t.CountrySlug)
		resp.FlixPatrolTargets = append(resp.FlixPatrolTargets, FlixPatrolTargetPayload{
			CountryCode:  string(code),
			ProviderSlug: t.ProviderSlug,
		})
	}
	return resp, nil
}

func (s *TaskScheduleAdminService) AddFlixPatrolTargets(ctx context.Context, scheduleID int64, in AddFlixPatrolTargetsInput) (*TaskScheduleResponse, error) {
	if scheduleID <= 0 {
		return nil, &ValidationError{Message: "invalid schedule id"}
	}
	if len(in.Targets) == 0 {
		return nil, &ValidationError{Message: "targets must have at least one value"}
	}

	targets := make([]repository.FlixPatrolTarget, 0, len(in.Targets))
	for _, t := range in.Targets {
		slug, err := flixpatrol.GetCountrySlugFromISO(t.CountryCode)
		if err != nil {
			return nil, &ValidationError{Message: fmt.Sprintf("invalid country_code: %s", t.CountryCode)}
		}
		provider := strings.TrimSpace(t.ProviderSlug)
		if provider == "" {
			return nil, &ValidationError{Message: "provider_slug is required"}
		}
		targets = append(targets, repository.FlixPatrolTarget{
			CountrySlug:  slug,
			ProviderSlug: provider,
		})
	}
	if err := s.repo.AddFlixPatrolTargets(ctx, scheduleID, targets); err != nil {
		return nil, err
	}
	return s.getFlixResponse(ctx, scheduleID)
}

func (s *TaskScheduleAdminService) getFlixResponse(ctx context.Context, scheduleID int64) (*TaskScheduleResponse, error) {
	schedule, err := s.repo.GetTaskScheduleByID(ctx, scheduleID)
	if err != nil {
		return nil, err
	}

	resp := &TaskScheduleResponse{
		ID:                  schedule.ID,
		TaskType:            schedule.TaskType,
		Name:                schedule.Name,
		Enabled:             schedule.Enabled,
		MaxRetries:          schedule.MaxRetries,
		RequestDelaySeconds: schedule.RequestDelaySeconds,
		RespectRobots:       schedule.RespectRobots,
		UserAgent:           schedule.UserAgent,
		UTCRuntimes:         schedule.RunTimesUTC,
		FlixPatrolTargets:   make([]FlixPatrolTargetPayload, 0, len(schedule.Targets)),
	}
	for _, t := range schedule.Targets {
		code, _ := flixpatrol.GetCountryCode(t.CountrySlug)
		resp.FlixPatrolTargets = append(resp.FlixPatrolTargets, FlixPatrolTargetPayload{
			CountryCode:  string(code),
			ProviderSlug: t.ProviderSlug,
		})
	}
	return resp, nil
}

func decodeFlixTargets(raw json.RawMessage) ([]repository.FlixPatrolTarget, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var details struct {
		Targets []struct {
			CountryCode  string `json:"country_code"`
			ProviderSlug string `json:"provider_slug"`
		} `json:"targets"`
	}
	if err := json.Unmarshal(raw, &details); err != nil {
		return nil, &ValidationError{Message: "invalid task_details JSON"}
	}
	out := make([]repository.FlixPatrolTarget, 0, len(details.Targets))
	for _, t := range details.Targets {
		slug, err := flixpatrol.GetCountrySlugFromISO(t.CountryCode)
		if err != nil {
			return nil, &ValidationError{Message: fmt.Sprintf("invalid country_code: %s", t.CountryCode)}
		}
		provider := strings.TrimSpace(t.ProviderSlug)
		if provider == "" {
			return nil, &ValidationError{Message: "provider_slug is required in task_details.targets"}
		}
		out = append(out, repository.FlixPatrolTarget{
			CountrySlug:  slug,
			ProviderSlug: provider,
		})
	}
	return out, nil
}

func withDefaultBool(v *bool, def bool) bool {
	if v == nil {
		return def
	}
	return *v
}

func withDefaultInt64(v *int64, def int64) int64 {
	if v == nil {
		return def
	}
	return *v
}

func withDefaultString(v *string, def string) string {
	if v == nil || strings.TrimSpace(*v) == "" {
		return def
	}
	return strings.TrimSpace(*v)
}

func toTaskRunResponse(r repository.TaskRunRecord) TaskRunResponse {
	var finishedAt *string
	if r.FinishedAt != nil {
		v := r.FinishedAt.UTC().Format(time.RFC3339)
		finishedAt = &v
	}
	return TaskRunResponse{
		ID:           r.ID,
		ScheduleID:   r.ScheduleID,
		TaskType:     r.TaskType,
		AsynqTaskID:  r.AsynqTaskID,
		Status:       r.Status,
		RetryCount:   r.RetryCount,
		MaxRetry:     r.MaxRetry,
		StartedAt:    r.StartedAt.UTC().Format(time.RFC3339),
		FinishedAt:   finishedAt,
		ErrorMessage: r.ErrorMessage,
	}
}
