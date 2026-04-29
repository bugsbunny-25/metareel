-- name: ListEnabledTaskSchedules :many
SELECT id, task_type, name, enabled, max_retries, request_delay_seconds, respect_robots, user_agent, created_at, updated_at
FROM task_schedules
WHERE enabled = 1
ORDER BY id ASC;

-- name: ListTaskSchedules :many
SELECT id, task_type, name, enabled, max_retries, request_delay_seconds, respect_robots, user_agent, created_at, updated_at
FROM task_schedules
WHERE (? = 0 OR enabled = ?)
  AND (? = 0 OR task_type = ?)
ORDER BY id ASC;

-- name: CreateTaskSchedule :one
INSERT INTO task_schedules (task_type, name, enabled, max_retries, request_delay_seconds, respect_robots, user_agent)
VALUES (?, ?, ?, ?, ?, ?, ?)
RETURNING id, task_type, name, enabled, max_retries, request_delay_seconds, respect_robots, user_agent, created_at, updated_at;

-- name: GetTaskScheduleByID :one
SELECT id, task_type, name, enabled, max_retries, request_delay_seconds, respect_robots, user_agent, created_at, updated_at
FROM task_schedules
WHERE id = ?;

-- name: ListTaskScheduleTargetsByScheduleID :many
SELECT id, schedule_id, country_slug, provider_slug
FROM task_schedule_targets
WHERE schedule_id = ?
ORDER BY id ASC;

-- name: CreateTaskScheduleTarget :exec
INSERT INTO task_schedule_targets (schedule_id, country_slug, provider_slug)
VALUES (?, ?, ?)
ON CONFLICT(schedule_id, country_slug, provider_slug) DO NOTHING;

-- name: ListTaskScheduleRunTimesByScheduleID :many
SELECT id, schedule_id, run_time_utc
FROM task_schedule_run_times
WHERE schedule_id = ?
ORDER BY run_time_utc ASC;

-- name: CreateTaskScheduleRunTime :exec
INSERT INTO task_schedule_run_times (schedule_id, run_time_utc)
VALUES (?, ?)
ON CONFLICT(schedule_id, run_time_utc) DO NOTHING;

-- name: DeleteTaskScheduleRunTimesByScheduleID :exec
DELETE FROM task_schedule_run_times WHERE schedule_id = ?;

-- name: UpdateTaskSchedule :one
UPDATE task_schedules
SET name = ?, enabled = ?, max_retries = ?,
    request_delay_seconds = ?, respect_robots = ?, user_agent = ?,
    updated_at = CURRENT_TIMESTAMP
WHERE id = ?
RETURNING id, task_type, name, enabled, max_retries, request_delay_seconds, respect_robots, user_agent, created_at, updated_at;

