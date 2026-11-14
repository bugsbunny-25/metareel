package service

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/hibiken/asynq"

	"github.com/bugsbunny-25/metareel/internal/tasks"
)

type captureEnqueuer struct{ task *asynq.Task }

func (c *captureEnqueuer) EnqueueContext(_ context.Context, task *asynq.Task, _ ...asynq.Option) (*asynq.TaskInfo, error) {
	c.task = task
	return &asynq.TaskInfo{}, nil
}

func TestRunJobCountries(t *testing.T) {
	cap := &captureEnqueuer{}
	s := &TaskScheduleAdminService{asynq: cap, queueName: "default"}
	ctx := context.Background()

	if _, err := s.RunJob(ctx, tasks.TypeVenuesImport, RunJobInput{Countries: []string{" in", "Gb"}}); err != nil {
		t.Fatal(err)
	}
	var p tasks.JobPayload
	if err := json.Unmarshal(cap.task.Payload(), &p); err != nil {
		t.Fatal(err)
	}
	if cap.task.Type() != tasks.TypeVenuesImport || len(p.Countries) != 2 || p.Countries[0] != "IN" || p.Countries[1] != "GB" {
		t.Fatalf("payload = %+v", p)
	}
	var verr *ValidationError
	for _, in := range []struct {
		typ string
		in  RunJobInput
	}{
		{tasks.TypeVenuesImport, RunJobInput{Countries: []string{"IND"}}},
		{tasks.TypeVenuesImport, RunJobInput{Countries: []string{"1N"}}},
		{tasks.TypeTMDBCredits, RunJobInput{Countries: []string{"IN"}}},
		{tasks.TypeVenuesImport, RunJobInput{Countries: make([]string, 31)}},
	} {
		if _, err := s.RunJob(ctx, in.typ, in.in); !errors.As(err, &verr) {
			t.Errorf("%s %+v: err = %v", in.typ, in.in, err)
		}
	}
}
