package scheduler

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/hibiken/asynq"

	"github.com/bugsbunny-25/metareel/internal/repository"
	"github.com/bugsbunny-25/metareel/internal/tasks"
)

type DBPeriodicTaskConfigProvider struct {
	repo      *repository.TaskScheduleRepository
	queueName string
	log       *slog.Logger
}

func NewDBPeriodicTaskConfigProvider(repo *repository.TaskScheduleRepository, queueName string, log *slog.Logger) *DBPeriodicTaskConfigProvider {
	return &DBPeriodicTaskConfigProvider{
		repo:      repo,
		queueName: queueName,
		log:       log,
	}
}

// GetConfigs turns every enabled schedule into one cron entry per run time.
// A schedule that can't be built (e.g. a FlixPatrol schedule without targets)
// is skipped with a warning rather than failing every other schedule.
func (p *DBPeriodicTaskConfigProvider) GetConfigs() ([]*asynq.PeriodicTaskConfig, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	schedules, err := p.repo.ListEnabledTaskSchedules(ctx)
	if err != nil {
		return nil, err
	}

	out := make([]*asynq.PeriodicTaskConfig, 0, len(schedules))
	for _, schedule := range schedules {
		for _, runTime := range schedule.RunTimesUTC {
			spec, err := utcTimeToCronSpec(runTime)
			if err != nil {
				return nil, fmt.Errorf("invalid run time %q for schedule %d: %w", runTime, schedule.ID, err)
			}
			task, opts, err := tasks.ForSchedule(schedule, p.queueName)
			if err != nil {
				if p.log != nil {
					p.log.Warn("schedule skipped", slog.Int64("schedule_id", schedule.ID), slog.String("name", schedule.Name), slog.Any("err", err))
				}
				break
			}
			out = append(out, &asynq.PeriodicTaskConfig{
				Cronspec: spec,
				Task:     task,
				Opts:     opts,
			})
		}
	}

	return out, nil
}

func utcTimeToCronSpec(runAtUTC string) (string, error) {
	parts := strings.Split(runAtUTC, ":")
	if len(parts) != 2 {
		return "", fmt.Errorf("invalid UTC time format (want HH:MM)")
	}

	hour, err := strconv.Atoi(parts[0])
	if err != nil || hour < 0 || hour > 23 {
		return "", fmt.Errorf("invalid hour")
	}
	minute, err := strconv.Atoi(parts[1])
	if err != nil || minute < 0 || minute > 59 {
		return "", fmt.Errorf("invalid minute")
	}

	return fmt.Sprintf("%d %d * * *", minute, hour), nil
}
