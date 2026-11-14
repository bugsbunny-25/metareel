package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/hibiken/asynq"

	"github.com/bugsbunny-25/metareel/internal/repository"
	"github.com/bugsbunny-25/metareel/internal/tasks"
)

// TaskTypeInfo describes a job type for the admin UI.
type TaskTypeInfo struct {
	Type        string `json:"type"`
	Label       string `json:"label"`
	Description string `json:"description"`
	HasTargets  bool   `json:"has_targets"` // FlixPatrol targets, backfill and re-check settings
}

var taskTypeInfo = map[string]TaskTypeInfo{
	tasks.TypeFlixPatrolScheduleRun: {Label: "FlixPatrol Top 10 scrape", HasTargets: true,
		Description: "Scrapes the current FlixPatrol charts of each target, fills gaps from the last backfill_days and re-checks hourly for recheck_hours when a chart is not published yet. New titles are matched right after."},
	tasks.TypeTitlesEnrich: {Label: "Title matching",
		Description: "Matches unmatched FlixPatrol titles to TMDB / IMDb (retrying with backoff: 1, 3, 7, then 30 days) and finds Rotten Tomatoes slugs via Wikidata, then RT search."},
	tasks.TypeTitlesMetadata: {Label: "TMDB metadata + watch providers",
		Description: "Refreshes TMDB details and watch providers (charting titles every 3 days, others monthly; needs TMDB_API_KEY) and Wikidata IDs (RT, Metacritic, Letterboxd) monthly."},
	tasks.TypeRatingsPrewarm: {Label: "Ratings pre-warm",
		Description: "Refreshes expired ratings of titles that charted in the last 3 days, so API requests are served from the database."},
	tasks.TypeNetflixTop10Import: {Label: "Netflix official Top 10",
		Description: "Imports Netflix's weekly global and per-country Top 10 and most-popular lists (skipped when unchanged), matches new titles to TMDB and maps season titles to season numbers."},
	tasks.TypeNetflixTitlesMatch: {Label: "Netflix title matching",
		Description: "Works through the backlog of unmatched Netflix titles (retrying with backoff: 1, 3, 7, then 30 days), then maps Netflix season titles and FlixPatrol TV rankings to season numbers."},
	tasks.TypeIMDbRatingsImport: {Label: "IMDb ratings dataset",
		Description: "Imports IMDb's daily title.ratings dataset (personal, non-commercial use) for the IMDb IDs metareel knows; used as the headline IMDb rating."},
	tasks.TypeTMDBPopularityImport: {Label: "TMDB popularity export",
		Description: "Records the daily TMDB popularity of the TMDB titles metareel knows from TMDB's daily ID exports (no API key needed; skipped when unchanged), for popularity history."},
	tasks.TypeWikipediaPageviews: {Label: "Wikipedia pageviews",
		Description: "Stores the daily English Wikipedia pageviews of every title with an article (found on Wikidata by the metadata job), since the last stored day (90 days for a new title); Force refetches the last week."},
	tasks.TypeNetflixEngagement: {Label: "Netflix engagement reports",
		Description: "Imports Netflix's \"What We Watched\" reports (hours viewed and views of every title, twice a year) from the list of report URLs (skipped when unchanged) and links them to Netflix Top 10 titles. Run with a url to add a new report."},
	tasks.TypeTMDBEpisodes: {Label: "TMDB episodes",
		Description: "Stores the episodes (air dates, names, finales) of the TV shows metareel knows from TMDB: recent, upcoming and latest seasons; airing shows daily, returning shows weekly, ended shows monthly (needs TMDB_API_KEY). Force refetches every show. Feeds the episode calendar and release patterns."},
	tasks.TypeTMDBCredits: {Label: "TMDB credits",
		Description: "Stores the top-billed cast and key crew (directors, writers, creators, composers, cinematographers) of the titles metareel knows from TMDB, new titles first, each again after 60 days, at most 1,000 a run (needs TMDB_API_KEY). Force refetches every title. Feeds title credits, people and people analytics."},
	tasks.TypeVenuesImport: {Label: "Cinema import (OpenStreetMap)",
		Description: "Imports every cinema OpenStreetMap has in each country (VENUES_IMPORT_COUNTRIES, or countries given to the run) as a venue to pick when logging a watch, with its address, coordinates and time zone. Matches venues entered before by OSM ID, Wikidata ID or the same name nearby; re-runs only fill empty fields and never delete. Run it by hand now and then; data © OpenStreetMap contributors (ODbL)."},
	tasks.TypeMappingRefresh: {Label: "Mapping refresh",
		Description: "Follows up mapping changes (queued automatically): shares mappings over links, fetches TMDB details and Wikidata IDs of the TMDB titles now mapped to, fills mapped titles' missing IMDb / Wikidata IDs and remaps season numbers. Run by hand, it shares mappings and fills IDs."},
	tasks.TypeMaintenance: {Label: "Maintenance",
		Description: "Deletes old task runs, alerts on stale charts, marks interrupted runs failed and optimises the database."},
}

// TaskTypes lists the job types a schedule can run.
func (s *TaskScheduleAdminService) TaskTypes() []TaskTypeInfo {
	out := make([]TaskTypeInfo, 0, len(tasks.Types))
	for _, t := range tasks.Types {
		info := taskTypeInfo[t]
		info.Type = t
		out = append(out, info)
	}
	return out
}

// RunJobInput starts a job outside its schedule.
type RunJobInput struct {
	TitleIDs []int64 `json:"title_ids,omitempty"` // titles.enrich (FlixPatrol title IDs) or netflix.titles.match (Netflix title IDs)
	Force    bool    `json:"force,omitempty"`     // imports: re-import unchanged files
	URL      string  `json:"url,omitempty"`       // netflix.engagement.import: a report to add
	// venues.import: countries (ISO 3166-1 alpha-2) to import instead of the configured ones.
	Countries []string `json:"countries,omitempty"`
}

// RunJob enqueues an ad-hoc run of a non-FlixPatrol job type.
func (s *TaskScheduleAdminService) RunJob(ctx context.Context, taskType string, in RunJobInput) (*RunTaskNowResponse, error) {
	if !tasks.IsKnownType(taskType) || taskType == tasks.TypeFlixPatrolScheduleRun {
		return nil, &ValidationError{Message: fmt.Sprintf("unsupported job type %q (FlixPatrol scrapes run from a schedule or the backfill endpoint)", taskType)}
	}
	if len(in.TitleIDs) > 0 && taskType != tasks.TypeTitlesEnrich && taskType != tasks.TypeNetflixTitlesMatch {
		return nil, &ValidationError{Message: "title_ids only apply to titles.enrich and netflix.titles.match"}
	}
	if in.URL != "" {
		if taskType != tasks.TypeNetflixEngagement {
			return nil, &ValidationError{Message: "url only applies to netflix.engagement.import"}
		}
		var err error
		if in.URL, err = ValidEngagementURL(in.URL); err != nil {
			return nil, err
		}
	}
	if len(in.Countries) > 0 {
		if taskType != tasks.TypeVenuesImport {
			return nil, &ValidationError{Message: "countries only apply to venues.import"}
		}
		if len(in.Countries) > 30 {
			return nil, &ValidationError{Message: "at most 30 countries"}
		}
		for i, cc := range in.Countries {
			cc = strings.ToUpper(strings.TrimSpace(cc))
			if !isCountryCode(cc) {
				return nil, &ValidationError{Message: fmt.Sprintf("country %q must be an ISO 3166-1 alpha-2 code, e.g. IN", in.Countries[i])}
			}
			in.Countries[i] = cc
		}
	}
	if len(in.TitleIDs) > 500 {
		return nil, &ValidationError{Message: "at most 500 title_ids"}
	}
	task, err := tasks.NewJobTask(taskType, tasks.JobPayload{TitleIDs: in.TitleIDs, Force: in.Force, URL: in.URL, Countries: in.Countries, RequestDelaySeconds: 5, Reason: "started from the admin API"})
	if err != nil {
		return nil, &ValidationError{Message: err.Error()}
	}
	if err := s.enqueue(ctx, task, tasks.Options(taskType, 0, s.queueName)); err != nil {
		return nil, err
	}
	return &RunTaskNowResponse{TaskType: taskType, Queue: s.queueName, Status: "enqueued"}, nil
}

// BackfillInput scrapes FlixPatrol charts for a date range.
type BackfillInput struct {
	ScheduleID int64                     `json:"schedule_id,omitempty"` // use this schedule's targets and settings
	Targets    []FlixPatrolTargetPayload `json:"targets,omitempty"`     // or these targets
	From       string                    `json:"from"`                  // YYYY-MM-DD
	To         string                    `json:"to"`                    // YYYY-MM-DD, default from
	Force      bool                      `json:"force,omitempty"`       // re-scrape charts already stored
}

// MaxBackfillDays caps one backfill request.
const MaxBackfillDays = 60

// Backfill enqueues an ad-hoc FlixPatrol scrape of every target for each date
// in [from, to]. Dates already stored are skipped unless force.
func (s *TaskScheduleAdminService) Backfill(ctx context.Context, in BackfillInput) (*RunTaskNowResponse, error) {
	from, err := time.Parse(repository.DateLayout, strings.TrimSpace(in.From))
	if err != nil {
		return nil, &ValidationError{Message: "from must be a date (YYYY-MM-DD)"}
	}
	to := from
	if strings.TrimSpace(in.To) != "" {
		if to, err = time.Parse(repository.DateLayout, strings.TrimSpace(in.To)); err != nil {
			return nil, &ValidationError{Message: "to must be a date (YYYY-MM-DD)"}
		}
	}
	if to.Before(from) {
		return nil, &ValidationError{Message: "to must not be before from"}
	}
	if days := int(to.Sub(from).Hours()/24) + 1; days > MaxBackfillDays {
		return nil, &ValidationError{Message: fmt.Sprintf("at most %d days per backfill", MaxBackfillDays)}
	}
	if latest := s.now().UTC(); to.After(latest) {
		return nil, &ValidationError{Message: "to must not be in the future"}
	}

	payload := tasks.FlixPatrolTop10Payload{RequestDelaySeconds: 10, UserAgent: "metareel-flixpatrol-bot/0.1", RespectRobots: true, Force: in.Force}
	maxRetry := 2
	switch {
	case in.ScheduleID > 0 && len(in.Targets) > 0:
		return nil, &ValidationError{Message: "give schedule_id or targets, not both"}
	case in.ScheduleID > 0:
		sched, err := s.repo.GetTaskScheduleByID(ctx, in.ScheduleID)
		if err != nil {
			return nil, err
		}
		if sched.TaskType != TaskTypeFlixPatrolTop10 {
			return nil, &ValidationError{Message: "schedule is not a FlixPatrol scrape"}
		}
		payload.ScheduleID, payload.ScheduleName = sched.ID, sched.Name+" (backfill)"
		payload.RequestDelaySeconds, payload.UserAgent, payload.RespectRobots = sched.RequestDelaySeconds, sched.UserAgent, sched.RespectRobots
		maxRetry = int(sched.MaxRetries)
		for _, t := range sched.Targets {
			payload.Targets = append(payload.Targets, tasks.FlixPatrolTop10TargetPayload{CountrySlug: t.CountrySlug, ProviderSlug: t.ProviderSlug})
		}
	case len(in.Targets) > 0:
		targets, err := flixTargets(in.Targets)
		if err != nil {
			return nil, err
		}
		payload.ScheduleName = "backfill"
		for _, t := range targets {
			payload.Targets = append(payload.Targets, tasks.FlixPatrolTop10TargetPayload{CountrySlug: t.CountrySlug, ProviderSlug: t.ProviderSlug})
		}
	default:
		return nil, &ValidationError{Message: "schedule_id or targets is required"}
	}
	if len(payload.Targets) == 0 {
		return nil, &ValidationError{Message: "no targets to backfill"}
	}
	for d := from; !d.After(to); d = d.AddDate(0, 0, 1) {
		payload.Dates = append(payload.Dates, repository.FormatDate(d))
	}
	task, err := tasks.NewFlixPatrolTop10Task(payload)
	if err != nil {
		return nil, &ValidationError{Message: err.Error()}
	}
	if err := s.enqueue(ctx, task, tasks.Options(tasks.TypeFlixPatrolScheduleRun, maxRetry, s.queueName)); err != nil {
		return nil, err
	}
	return &RunTaskNowResponse{ScheduleID: payload.ScheduleID, TaskType: tasks.TypeFlixPatrolScheduleRun, Queue: s.queueName, Status: "enqueued"}, nil
}

func (s *TaskScheduleAdminService) enqueue(ctx context.Context, task *asynq.Task, opts []asynq.Option) error {
	return s.enqueueUnique(ctx, task, opts, "the same job is already queued, running or retrying")
}

// archivedScanPages caps the archived tasks scanned for a failed duplicate
// (100 per page).
const archivedScanPages = 20

// enqueueUnique enqueues a unique task. When an identical task holds the
// lock only because it failed (archived), that task is deleted, which frees
// the lock, and the task is enqueued again; one queued, running or retrying
// is a conflict.
func (s *TaskScheduleAdminService) enqueueUnique(ctx context.Context, task *asynq.Task, opts []asynq.Option, conflict string) error {
	if s.asynq == nil {
		return fmt.Errorf("asynq client not configured")
	}
	_, err := s.asynq.EnqueueContext(ctx, task, opts...)
	if errors.Is(err, asynq.ErrDuplicateTask) && s.archived != nil {
		if freed, ferr := s.freeFailedDuplicate(task); ferr != nil {
			return fmt.Errorf("free failed run: %w", ferr)
		} else if freed {
			_, err = s.asynq.EnqueueContext(ctx, task, opts...)
		}
	}
	if err != nil {
		if errors.Is(err, asynq.ErrDuplicateTask) {
			return &ConflictError{Message: conflict}
		}
		return fmt.Errorf("enqueue task: %w", err)
	}
	return nil
}

// freeFailedDuplicate deletes archived (failed) tasks with the same type and
// payload as task; it reports whether it deleted any.
func (s *TaskScheduleAdminService) freeFailedDuplicate(task *asynq.Task) (bool, error) {
	queue := s.queueName
	if queue == "" {
		queue = "default"
	}
	freed := false
	for page := 1; page <= archivedScanPages; page++ {
		infos, err := s.archived.ListArchivedTasks(queue, asynq.PageSize(100), asynq.Page(page))
		if err != nil {
			if errors.Is(err, asynq.ErrQueueNotFound) {
				return false, nil
			}
			return freed, err
		}
		for _, info := range infos {
			if info.Type == task.Type() && bytes.Equal(info.Payload, task.Payload()) {
				if err := s.archived.DeleteTask(queue, info.ID); err != nil && !errors.Is(err, asynq.ErrTaskNotFound) {
					return freed, err
				}
				freed = true
			}
		}
		if len(infos) < 100 {
			break
		}
	}
	return freed, nil
}

func isCountryCode(s string) bool {
	return len(s) == 2 && s[0] >= 'A' && s[0] <= 'Z' && s[1] >= 'A' && s[1] <= 'Z'
}
