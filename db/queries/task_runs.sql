-- name: CreateTaskRun :one
INSERT INTO task_runs (schedule_id, task_type, asynq_task_id, status, retry_count, max_retry)
VALUES (?, ?, ?, ?, ?, ?)
RETURNING id, schedule_id, task_type, asynq_task_id, status, retry_count, max_retry, started_at, finished_at, error_message;

-- name: CompleteTaskRun :exec
UPDATE task_runs
SET status = ?, finished_at = CURRENT_TIMESTAMP, error_message = ?
WHERE id = ?;

-- name: CreateTaskRunLog :exec
INSERT INTO task_run_logs (run_id, level, message)
VALUES (?, ?, ?);

-- name: ListTaskRunsByScheduleID :many
SELECT id, schedule_id, task_type, asynq_task_id, status, retry_count, max_retry, started_at, finished_at, error_message
FROM task_runs
WHERE schedule_id = ?
ORDER BY started_at DESC
LIMIT ? OFFSET ?;

-- name: ListTaskRuns :many
-- Empty / zero filters match everything.
SELECT id, schedule_id, task_type, asynq_task_id, status, retry_count, max_retry, started_at, finished_at, error_message
FROM task_runs
WHERE (CAST(sqlc.arg(task_type) AS TEXT) = '' OR task_type = sqlc.arg(task_type))
  AND (CAST(sqlc.arg(status) AS TEXT) = '' OR status = sqlc.arg(status))
  AND (CAST(sqlc.arg(schedule_id) AS INTEGER) = 0 OR schedule_id = sqlc.arg(schedule_id))
ORDER BY started_at DESC, id DESC
LIMIT sqlc.arg(limit) OFFSET sqlc.arg(offset);

-- name: CountTaskRuns :one
SELECT COUNT(*)
FROM task_runs
WHERE (CAST(sqlc.arg(task_type) AS TEXT) = '' OR task_type = sqlc.arg(task_type))
  AND (CAST(sqlc.arg(status) AS TEXT) = '' OR status = sqlc.arg(status))
  AND (CAST(sqlc.arg(schedule_id) AS INTEGER) = 0 OR schedule_id = sqlc.arg(schedule_id));

-- name: CountTaskRunsByStatusSince :many
-- since is a SQLite datetime modifier, e.g. '-24 hours'.
SELECT status, COUNT(*) AS runs
FROM task_runs
WHERE started_at >= datetime('now', CAST(sqlc.arg(since) AS TEXT))
GROUP BY status;

-- name: ListLatestTaskRunPerSchedule :many
SELECT id, schedule_id, task_type, asynq_task_id, status, retry_count, max_retry, started_at, finished_at, error_message
FROM task_runs r
WHERE r.id = (SELECT MAX(x.id) FROM task_runs x WHERE x.schedule_id = r.schedule_id);

-- name: ListLastSuccessPerSchedule :many
SELECT schedule_id, CAST(MAX(finished_at) AS TEXT) AS finished_at
FROM task_runs
WHERE status = 'succeeded'
GROUP BY schedule_id;

-- name: GetTaskRunByID :one
SELECT id, schedule_id, task_type, asynq_task_id, status, retry_count, max_retry, started_at, finished_at, error_message
FROM task_runs
WHERE id = ?;

-- name: ListTaskRunLogsByRunID :many
SELECT id, run_id, level, message, created_at
FROM task_run_logs
WHERE run_id = ?
ORDER BY created_at ASC, id ASC;


-- name: FailStaleTaskRuns :execrows
-- Runs still "started" long after the task timeout were interrupted (e.g. the
-- server restarted mid-run); asynq retries the task as a new run.
UPDATE task_runs
SET status = 'failed',
    finished_at = CURRENT_TIMESTAMP,
    error_message = 'interrupted: no result recorded (server restarted or task timed out)'
WHERE status = 'started'
  AND started_at < datetime('now', CAST(sqlc.arg(older_than) AS TEXT));
