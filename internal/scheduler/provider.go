package scheduler

import (
	"context"
	"fmt"
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
}

func NewDBPeriodicTaskConfigProvider(repo *repository.TaskScheduleRepository, queueName string) *DBPeriodicTaskConfigProvider {
	return &DBPeriodicTaskConfigProvider{
		repo:      repo,
		queueName: queueName,
	}
}

func (p *DBPeriodicTaskConfigProvider) GetConfigs() ([]*asynq.PeriodicTaskConfig, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	out := make([]*asynq.PeriodicTaskConfig, 0)

	flixpatrolSchedules, err := p.repo.ListFlixPatrolTop10TaskScheduleConfigs(ctx)
	if err != nil {
		return nil, err
	}
	flixpatrolConfigs, err := p.buildFlixPatrolTop10Configs(flixpatrolSchedules)
	if err != nil {
		return nil, err
	}
	out = append(out, flixpatrolConfigs...)

	return out, nil
}

func (p *DBPeriodicTaskConfigProvider) buildFlixPatrolTop10Configs(schedules []repository.FlixPatrolTaskSchedule) ([]*asynq.PeriodicTaskConfig, error) {
	out := make([]*asynq.PeriodicTaskConfig, 0, len(schedules))
	for _, schedule := range schedules {
		targets := make([]tasks.FlixPatrolTop10TargetPayload, 0, len(schedule.Targets))
		for _, t := range schedule.Targets {
			targets = append(targets, tasks.FlixPatrolTop10TargetPayload{
				CountrySlug:  t.CountrySlug,
				ProviderSlug: t.ProviderSlug,
			})
		}

		payload := tasks.FlixPatrolTop10Payload{
			ScheduleID:          schedule.ID,
			ScheduleName:        schedule.Name,
			RequestDelaySeconds: schedule.RequestDelaySeconds,
			UserAgent:           schedule.UserAgent,
			RespectRobots:       schedule.RespectRobots,
			Targets:             targets,
		}

		for _, runTime := range schedule.RunTimesUTC {
			spec, err := utcTimeToCronSpec(runTime)
			if err != nil {
				return nil, fmt.Errorf("invalid run time %q for schedule %d: %w", runTime, schedule.ID, err)
			}
			task, err := tasks.NewFlixPatrolTop10Task(payload)
			if err != nil {
				return nil, err
			}

			opts := []asynq.Option{
				asynq.MaxRetry(int(schedule.MaxRetries) * len(payload.Targets)),
			}
			if p.queueName != "" {
				opts = append(opts, asynq.Queue(p.queueName))
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
