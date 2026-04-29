package scheduler

import (
	"time"

	"github.com/hibiken/asynq"
)

func NewPeriodicTaskManager(redisOpt asynq.RedisConnOpt, provider asynq.PeriodicTaskConfigProvider) (*asynq.PeriodicTaskManager, error) {
	return asynq.NewPeriodicTaskManager(asynq.PeriodicTaskManagerOpts{
		RedisConnOpt:               redisOpt,
		PeriodicTaskConfigProvider: provider,
		SchedulerOpts:              &asynq.SchedulerOpts{Location: time.UTC},
		SyncInterval:               1 * time.Minute,
	})
}

