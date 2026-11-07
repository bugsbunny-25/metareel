package service

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/hibiken/asynq"
)

// fakeQueue holds unique locks like asynq: a task with the same type and
// payload is a duplicate while another holds the lock; an archived (failed)
// task keeps its lock until it is deleted.
type fakeQueue struct {
	locked   map[string]string // type+payload -> state (pending | archived)
	ids      map[string]string // type+payload -> task id
	deleted  []string
	enqueued int
}

func key(t *asynq.Task) string { return t.Type() + "\x00" + string(t.Payload()) }

func (q *fakeQueue) EnqueueContext(_ context.Context, t *asynq.Task, _ ...asynq.Option) (*asynq.TaskInfo, error) {
	if _, ok := q.locked[key(t)]; ok {
		return nil, asynq.ErrDuplicateTask
	}
	q.locked[key(t)] = "pending"
	q.enqueued++
	return &asynq.TaskInfo{}, nil
}

func (q *fakeQueue) ListArchivedTasks(queue string, _ ...asynq.ListOption) ([]*asynq.TaskInfo, error) {
	if queue != "jobs" {
		return nil, asynq.ErrQueueNotFound
	}
	var out []*asynq.TaskInfo
	for k, state := range q.locked {
		if state == "archived" {
			typ, payload, _ := bytes.Cut([]byte(k), []byte{0})
			out = append(out, &asynq.TaskInfo{ID: q.ids[k], Type: string(typ), Payload: payload})
		}
	}
	return out, nil
}

func (q *fakeQueue) DeleteTask(_ string, id string) error {
	for k, tid := range q.ids {
		if tid == id {
			delete(q.locked, k)
			q.deleted = append(q.deleted, id)
			return nil
		}
	}
	return asynq.ErrTaskNotFound
}

func TestRerunReplacesFailedRun(t *testing.T) {
	ctx := context.Background()
	job := asynq.NewTask("wikipedia.pageviews.import", []byte(`{"reason":"started from the admin API"}`))
	other := asynq.NewTask("imdb.ratings.import", []byte(`{}`))
	q := &fakeQueue{
		locked: map[string]string{key(job): "archived", key(other): "archived"},
		ids:    map[string]string{key(job): "failed-1", key(other): "failed-2"},
	}
	s := &TaskScheduleAdminService{asynq: q, archived: q, queueName: "jobs"}

	// The failed run is deleted (its lock freed) and the job enqueued.
	if err := s.enqueue(ctx, job, nil); err != nil {
		t.Fatalf("rerun after a failure: %v", err)
	}
	if q.enqueued != 1 || len(q.deleted) != 1 || q.deleted[0] != "failed-1" {
		t.Fatalf("enqueued %d, deleted %v", q.enqueued, q.deleted)
	}
	// Now one is queued: a second run is a conflict, nothing else deleted.
	var conflict *ConflictError
	if err := s.enqueue(ctx, job, nil); !errors.As(err, &conflict) {
		t.Fatalf("second run: err = %v, want ConflictError", err)
	}
	if len(q.deleted) != 1 || q.locked[key(other)] != "archived" {
		t.Fatalf("touched other tasks: deleted %v", q.deleted)
	}

	// Without an inspector the old behaviour stays: a conflict.
	plain := &TaskScheduleAdminService{asynq: &fakeQueue{locked: map[string]string{key(other): "archived"}, ids: map[string]string{}}, queueName: "jobs"}
	if err := plain.enqueue(ctx, other, nil); !errors.As(err, &conflict) {
		t.Fatalf("no inspector: err = %v", err)
	}
	// Not configured at all.
	if err := NewTaskScheduleAdminService(nil, nil, "").enqueue(ctx, job, nil); err == nil || errors.As(err, &conflict) {
		t.Fatalf("no client: err = %v", err)
	}
}
