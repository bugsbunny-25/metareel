package service

import (
	"context"
	"sort"
	"time"

	"github.com/bugsbunny-25/metareel/internal/repository"
)

// AdminStatsService summarizes scheduler health and data coverage for the
// admin dashboard.
type AdminStatsService struct {
	tasks  *repository.TaskScheduleRepository
	titles *repository.FlixPatrolRepository
	now    func() time.Time
}

func NewAdminStatsService(tasks *repository.TaskScheduleRepository, titles *repository.FlixPatrolRepository) *AdminStatsService {
	return &AdminStatsService{tasks: tasks, titles: titles, now: time.Now}
}

type AdminStatsResponse struct {
	GeneratedAt time.Time           `json:"generated_at"`
	Schedules   ScheduleCounts      `json:"schedules"`
	Runs        RunCounts           `json:"runs"`
	Health      []ScheduleHealth    `json:"schedule_health"`
	Titles      TitleCoverageCounts `json:"titles"`
	Rankings    RankingCounts       `json:"rankings"`
}

type ScheduleCounts struct {
	Total   int `json:"total"`
	Enabled int `json:"enabled"`
}

type RunCounts struct {
	Running int64            `json:"running"`  // runs currently in progress
	Last24h map[string]int64 `json:"last_24h"` // by status
	Last7d  map[string]int64 `json:"last_7d"`  // by status
}

type ScheduleHealth struct {
	ScheduleID    int64            `json:"schedule_id"`
	Name          string           `json:"name"`
	Enabled       bool             `json:"enabled"`
	UTCRuntimes   []string         `json:"utc_runtimes"`
	NextRunAt     *time.Time       `json:"next_run_at"` // null when disabled
	LastRun       *TaskRunResponse `json:"last_run"`
	LastSuccessAt *time.Time       `json:"last_success_at"`
}

type TitleCoverageCounts struct {
	Total       int64 `json:"total"`
	MissingTmdb int64 `json:"missing_tmdb"`
	MissingImdb int64 `json:"missing_imdb"`
	MissingRT   int64 `json:"missing_rt"`
}

type RankingCounts struct {
	Total      int64   `json:"total"`
	LatestDate *string `json:"latest_date"`
}

func (s *AdminStatsService) GetStats(ctx context.Context) (*AdminStatsResponse, error) {
	now := s.now().UTC()
	schedules, err := s.tasks.ListTaskSchedules(ctx, nil, "")
	if err != nil {
		return nil, err
	}
	last24h, err := s.tasks.CountTaskRunsByStatusSince(ctx, "-24 hours")
	if err != nil {
		return nil, err
	}
	last7d, err := s.tasks.CountTaskRunsByStatusSince(ctx, "-7 days")
	if err != nil {
		return nil, err
	}
	running, err := s.tasks.CountTaskRuns(ctx, repository.TaskRunFilter{Status: "started"})
	if err != nil {
		return nil, err
	}
	latest, err := s.tasks.ListLatestTaskRunPerSchedule(ctx)
	if err != nil {
		return nil, err
	}
	lastSuccess, err := s.tasks.ListLastSuccessPerSchedule(ctx)
	if err != nil {
		return nil, err
	}
	ts, err := s.titles.TitleStats(ctx)
	if err != nil {
		return nil, err
	}

	latestBySchedule := map[int64]repository.TaskRunRecord{}
	for _, r := range latest {
		latestBySchedule[r.ScheduleID] = r
	}

	out := &AdminStatsResponse{
		GeneratedAt: now,
		Runs:        RunCounts{Running: running, Last24h: last24h, Last7d: last7d},
		Health:      make([]ScheduleHealth, 0, len(schedules)),
		Titles:      TitleCoverageCounts{Total: ts.Total, MissingTmdb: ts.MissingTmdb, MissingImdb: ts.MissingImdb, MissingRT: ts.MissingRT},
		Rankings:    RankingCounts{Total: ts.Rankings},
	}
	if ts.LatestChart != nil {
		d := ts.LatestChart.Format("2006-01-02")
		out.Rankings.LatestDate = &d
	}
	for _, sc := range schedules {
		out.Schedules.Total++
		h := ScheduleHealth{ScheduleID: sc.ID, Name: sc.Name, Enabled: sc.Enabled, UTCRuntimes: sc.RunTimesUTC}
		if sc.Enabled {
			out.Schedules.Enabled++
			h.NextRunAt = nextRunAt(now, sc.RunTimesUTC)
		}
		if r, ok := latestBySchedule[sc.ID]; ok {
			resp := toTaskRunResponse(r)
			h.LastRun = &resp
		}
		if t, ok := lastSuccess[sc.ID]; ok {
			h.LastSuccessAt = &t
		}
		out.Health = append(out.Health, h)
	}
	sort.Slice(out.Health, func(i, j int) bool { return out.Health[i].ScheduleID < out.Health[j].ScheduleID })
	return out, nil
}

// nextRunAt is the next occurrence of any "HH:MM" UTC time after now.
func nextRunAt(now time.Time, runtimes []string) *time.Time {
	var next *time.Time
	for _, rt := range runtimes {
		hm, err := time.Parse("15:04", rt)
		if err != nil {
			continue
		}
		t := time.Date(now.Year(), now.Month(), now.Day(), hm.Hour(), hm.Minute(), 0, 0, time.UTC)
		if !t.After(now) {
			t = t.AddDate(0, 0, 1)
		}
		if next == nil || t.Before(*next) {
			next = &t
		}
	}
	return next
}
