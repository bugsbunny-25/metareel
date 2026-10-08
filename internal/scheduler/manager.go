package scheduler

import (
	"encoding/json"
	"log/slog"
	"time"

	"github.com/hibiken/asynq"
)

func NewPeriodicTaskManager(redisOpt asynq.RedisConnOpt, provider asynq.PeriodicTaskConfigProvider, log *slog.Logger, logLevel string) (*asynq.PeriodicTaskManager, error) {
	return asynq.NewPeriodicTaskManager(asynq.PeriodicTaskManagerOpts{
		RedisConnOpt:               redisOpt,
		PeriodicTaskConfigProvider: provider,
		SchedulerOpts: &asynq.SchedulerOpts{
			Location:        time.UTC,
			Logger:          NewAsynqLogger(log),
			LogLevel:        AsynqLogLevel(logLevel),
			PostEnqueueFunc: logEnqueue(log),
		},
		SyncInterval: 1 * time.Minute,
	})
}

// logEnqueue records every cron-triggered enqueue, so a missing run can be
// told apart from a run that was enqueued but failed.
func logEnqueue(log *slog.Logger) func(info *asynq.TaskInfo, err error) {
	return func(info *asynq.TaskInfo, err error) {
		if err != nil {
			log.Error("scheduled task enqueue failed", slog.Any("err", err))
			return
		}
		var payload struct {
			ScheduleID   int64  `json:"schedule_id"`
			ScheduleName string `json:"schedule_name"`
		}
		_ = json.Unmarshal(info.Payload, &payload)
		log.Info("scheduled task enqueued",
			slog.String("task_id", info.ID),
			slog.String("task_type", info.Type),
			slog.String("queue", info.Queue),
			slog.Int64("schedule_id", payload.ScheduleID),
			slog.String("schedule_name", payload.ScheduleName))
	}
}
