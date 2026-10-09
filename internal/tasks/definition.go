package tasks

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/hibiken/asynq"
)

// Task types. Each schedule (task_schedules.task_type) runs one of these.
const (
	TypeFlixPatrolScheduleRun = "flixpatrol.top10.scrape"
	TypeTitlesEnrich          = "titles.enrich"
	TypeTitlesMetadata        = "titles.metadata"
	TypeRatingsPrewarm        = "ratings.prewarm"
	TypeNetflixTop10Import    = "netflix.top10.import"
	TypeIMDbRatingsImport     = "imdb.ratings.import"
	TypeMaintenance           = "maintenance"
)

// Types lists every task type a schedule can have, in display order.
var Types = []string{
	TypeFlixPatrolScheduleRun,
	TypeTitlesEnrich,
	TypeTitlesMetadata,
	TypeRatingsPrewarm,
	TypeNetflixTop10Import,
	TypeIMDbRatingsImport,
	TypeMaintenance,
}

// IsKnownType reports whether t is one of Types.
func IsKnownType(t string) bool {
	for _, k := range Types {
		if k == t {
			return true
		}
	}
	return false
}

// FlixPatrolTop10Timeout bounds one FlixPatrol run. Runs fetch a page per
// target and date (each behind the schedule's request delay, and slow when
// routed through FlareSolverr), so asynq's 30m default is too short.
const FlixPatrolTop10Timeout = 2 * time.Hour

// Timeout returns the asynq timeout for a task type.
func Timeout(taskType string) time.Duration {
	switch taskType {
	case TypeFlixPatrolScheduleRun, TypeTitlesEnrich:
		return FlixPatrolTop10Timeout
	case TypeIMDbRatingsImport, TypeNetflixTop10Import, TypeTitlesMetadata:
		return time.Hour
	default:
		return 30 * time.Minute
	}
}

// MaxTimeout is the longest Timeout of any task type (stale-run detection).
const MaxTimeout = FlixPatrolTop10Timeout

type FlixPatrolTop10TargetPayload struct {
	CountrySlug  string `json:"country_slug"`
	ProviderSlug string `json:"provider_slug"`
}

type FlixPatrolTop10Payload struct {
	ScheduleID          int64                          `json:"schedule_id"`
	ScheduleName        string                         `json:"schedule_name"`
	RequestDelaySeconds int64                          `json:"request_delay_seconds"`
	UserAgent           string                         `json:"user_agent"`
	RespectRobots       bool                           `json:"respect_robots"`
	Targets             []FlixPatrolTop10TargetPayload `json:"targets"`

	// Dates (YYYY-MM-DD) to scrape instead of the current chart date: set
	// for backfills. Charts already stored are skipped unless Force.
	Dates []string `json:"dates,omitempty"`
	Force bool     `json:"force,omitempty"`
	// BackfillDays also scrapes charts missing from the previous N days.
	BackfillDays int64 `json:"backfill_days,omitempty"`
	// RecheckHours: when FlixPatrol still shows the previous day's chart,
	// check again hourly for this long after the run started before storing
	// it anyway.
	RecheckHours int64 `json:"recheck_hours,omitempty"`
	// RecheckUntil (RFC 3339) is set on follow-up re-check tasks.
	RecheckUntil string `json:"recheck_until,omitempty"`
}

func NewFlixPatrolTop10Task(payload FlixPatrolTop10Payload) (*asynq.Task, error) {
	if payload.ScheduleID < 0 {
		return nil, fmt.Errorf("schedule id must be >= 0")
	}
	if len(payload.Targets) == 0 {
		return nil, fmt.Errorf("at least one target is required")
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal flixpatrol payload: %w", err)
	}
	return asynq.NewTask(TypeFlixPatrolScheduleRun, body), nil
}

// JobPayload is the payload of every task type other than FlixPatrol scrapes.
type JobPayload struct {
	ScheduleID   int64  `json:"schedule_id,omitempty"` // 0 for ad-hoc runs
	ScheduleName string `json:"schedule_name,omitempty"`
	// RequestDelaySeconds paces FlixPatrol title-page fetches (titles.enrich).
	RequestDelaySeconds int64 `json:"request_delay_seconds,omitempty"`
	// TitleIDs limits titles.enrich / titles.metadata to these titles (and
	// ignores their retry backoff); empty = the due backlog.
	TitleIDs []int64 `json:"title_ids,omitempty"`
	// Force refreshes even fresh data (imports: ignore "unchanged" checks).
	Force bool `json:"force,omitempty"`
	// Reason is shown in the run log, e.g. "after scrape run 12".
	Reason string `json:"reason,omitempty"`
}

func NewJobTask(taskType string, payload JobPayload) (*asynq.Task, error) {
	if !IsKnownType(taskType) || taskType == TypeFlixPatrolScheduleRun {
		return nil, fmt.Errorf("unsupported job task type: %s", taskType)
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal job payload: %w", err)
	}
	return asynq.NewTask(taskType, body), nil
}

// Options returns the enqueue options shared by scheduled and manual runs.
// Unique keeps a second copy of the same task (same type and payload) from
// being queued while one is queued, running or retrying.
func Options(taskType string, maxRetry int, queue string) []asynq.Option {
	opts := []asynq.Option{
		asynq.MaxRetry(maxRetry),
		asynq.Timeout(Timeout(taskType)),
		asynq.Unique(Timeout(taskType)),
	}
	if queue != "" {
		opts = append(opts, asynq.Queue(queue))
	}
	return opts
}
