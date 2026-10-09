-- name: ListEnabledTaskSchedules :many
SELECT *
FROM task_schedules
WHERE enabled = 1
ORDER BY id ASC;

-- name: ListTaskSchedules :many
SELECT *
FROM task_schedules
WHERE (? = 0 OR enabled = ?)
  AND (? = 0 OR task_type = ?)
ORDER BY id ASC;

-- name: CreateTaskSchedule :one
INSERT INTO task_schedules (task_type, name, enabled, max_retries, request_delay_seconds, respect_robots, user_agent, backfill_days, recheck_hours)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: GetTaskScheduleByID :one
SELECT *
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
    backfill_days = ?, recheck_hours = ?,
    updated_at = CURRENT_TIMESTAMP
WHERE id = ?
RETURNING *;


-- name: DeleteTaskScheduleTargets :exec
DELETE FROM task_schedule_targets WHERE schedule_id = ?;

-- name: ListAllFlixPatrolTargets :many
-- Distinct targets of every FlixPatrol schedule (enabled or not).
SELECT DISTINCT t.country_slug, t.provider_slug
FROM task_schedule_targets t
JOIN task_schedules s ON s.id = t.schedule_id
WHERE s.task_type = 'flixpatrol.top10.scrape'
ORDER BY t.country_slug, t.provider_slug;
