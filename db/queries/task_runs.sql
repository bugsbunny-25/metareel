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
SELECT id, schedule_id, task_type, asynq_task_id, status, retry_count, max_retry, started_at, finished_at, error_message
FROM task_runs
WHERE (? = 0 OR task_type = ?)
ORDER BY started_at DESC
LIMIT ? OFFSET ?;

-- name: GetTaskRunByID :one
SELECT id, schedule_id, task_type, asynq_task_id, status, retry_count, max_retry, started_at, finished_at, error_message
FROM task_runs
WHERE id = ?;

-- name: ListTaskRunLogsByRunID :many
SELECT id, run_id, level, message, created_at
FROM task_run_logs
WHERE run_id = ?
ORDER BY created_at ASC, id ASC;

