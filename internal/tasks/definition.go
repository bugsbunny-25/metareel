package tasks

import (
	"encoding/json"
	"fmt"

	"github.com/hibiken/asynq"
)

const TypeFlixPatrolScheduleRun = "flixpatrol.top10.scrape"

type ScheduleRunPayload struct {
	ScheduleID int64 `json:"schedule_id"`
}

type FlixPatrolTop10TargetPayload struct {
	CountrySlug  string `json:"country_slug"`
	ProviderSlug string `json:"provider_slug"`
}

type FlixPatrolTop10Payload struct {
	ScheduleID          int64                          `json:"schedule_id"`
	ScheduleName        string                         `json:"schedule_name"`
	RequestDelaySeconds int64                          `json:"request_delay_seconds"`
	UserAgent           string                         `json:"user_agent"`
	RespectRobots       bool                           `json:"respect_robots"`
	Targets             []FlixPatrolTop10TargetPayload `json:"targets"`
}

func NewScheduleRunTask(taskType string, scheduleID int64) (*asynq.Task, error) {
	if taskType == "" {
		return nil, fmt.Errorf("task type is required")
	}
	if scheduleID <= 0 {
		return nil, fmt.Errorf("schedule id must be > 0")
	}
	payload, err := json.Marshal(ScheduleRunPayload{ScheduleID: scheduleID})
	if err != nil {
		return nil, fmt.Errorf("marshal schedule payload: %w", err)
	}
	return asynq.NewTask(taskType, payload), nil
}

func NewFlixPatrolTop10Task(payload FlixPatrolTop10Payload) (*asynq.Task, error) {
	if payload.ScheduleID <= 0 {
		return nil, fmt.Errorf("schedule id must be > 0")
	}
	if len(payload.Targets) == 0 {
		return nil, fmt.Errorf("at least one target is required")
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal flixpatrol payload: %w", err)
	}
	return asynq.NewTask(TypeFlixPatrolScheduleRun, body), nil
}

