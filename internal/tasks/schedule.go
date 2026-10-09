package tasks

import (
	"fmt"

	"github.com/hibiken/asynq"

	"github.com/bugsbunny-25/metareel/internal/repository"
)

// ForSchedule builds the task a schedule runs (on its cron times or "run
// now") and its enqueue options.
func ForSchedule(s repository.TaskSchedule, queue string) (*asynq.Task, []asynq.Option, error) {
	var (
		task *asynq.Task
		err  error
	)
	switch s.TaskType {
	case TypeFlixPatrolScheduleRun:
		payload := FlixPatrolTop10Payload{
			ScheduleID:          s.ID,
			ScheduleName:        s.Name,
			RequestDelaySeconds: s.RequestDelaySeconds,
			UserAgent:           s.UserAgent,
			RespectRobots:       s.RespectRobots,
			BackfillDays:        s.BackfillDays,
			RecheckHours:        s.RecheckHours,
			Targets:             make([]FlixPatrolTop10TargetPayload, 0, len(s.Targets)),
		}
		for _, t := range s.Targets {
			payload.Targets = append(payload.Targets, FlixPatrolTop10TargetPayload{
				CountrySlug:  t.CountrySlug,
				ProviderSlug: t.ProviderSlug,
			})
		}
		task, err = NewFlixPatrolTop10Task(payload)
	default:
		if !IsKnownType(s.TaskType) {
			return nil, nil, fmt.Errorf("unsupported task_type: %s", s.TaskType)
		}
		task, err = NewJobTask(s.TaskType, JobPayload{
			ScheduleID:          s.ID,
			ScheduleName:        s.Name,
			RequestDelaySeconds: s.RequestDelaySeconds,
		})
	}
	if err != nil {
		return nil, nil, err
	}
	// Retries redo only what failed (stored charts are skipped), so the
	// schedule's max_retries applies to the run as a whole.
	return task, Options(s.TaskType, int(s.MaxRetries), queue), nil
}
