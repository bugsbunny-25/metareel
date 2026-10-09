package service

import (
	"context"
	"time"

	"github.com/bugsbunny-25/metareel/internal/repository"
	"github.com/bugsbunny-25/metareel/internal/scraper/flixpatrol"
	"github.com/bugsbunny-25/metareel/internal/tasks"
)

// DataQualityService reports how complete and fresh the data is (admin).
type DataQualityService struct {
	flix        *repository.FlixPatrolRepository
	ops         *repository.OpsRepository
	imports     *repository.ImportsRepository
	metadata    *repository.MetadataRepository
	schedules   *repository.TaskScheduleRepository
	maintenance *MaintenanceService
	now         func() time.Time
}

func NewDataQualityService(flix *repository.FlixPatrolRepository, ops *repository.OpsRepository, imports *repository.ImportsRepository, metadata *repository.MetadataRepository, schedules *repository.TaskScheduleRepository, maintenance *MaintenanceService) *DataQualityService {
	return &DataQualityService{flix: flix, ops: ops, imports: imports, metadata: metadata, schedules: schedules, maintenance: maintenance, now: time.Now}
}

// DataQualityReport is GET /api/v1/admin/data-quality.
type DataQualityReport struct {
	GeneratedAt        time.Time                 `json:"generated_at"`
	TitleMatchStatus   map[string]int64          `json:"title_match_status"`
	NetflixMatchStatus map[string]int64          `json:"netflix_title_match_status"`
	Metadata           repository.MetadataCounts `json:"metadata"`
	IMDbRatings        int64                     `json:"imdb_ratings"`
	Imports            []repository.ImportState  `json:"imports"`
	Coverage           []repository.CoverageWeek `json:"weekly_coverage"`
	UnmatchedByCountry []repository.CountryCount `json:"unmatched_by_country"`
	StaleCharts        []StaleChart              `json:"stale_charts"`
	ChartGaps          []ChartGap                `json:"chart_gaps"`
	GapWindowDays      int                       `json:"gap_window_days"`
	FreshnessByChart   []ChartFreshnessItem      `json:"chart_freshness"`
}

// ChartGap lists dates missing for a scheduled chart within the gap window.
type ChartGap struct {
	Country  string   `json:"country"`
	Provider string   `json:"provider"`
	Missing  []string `json:"missing_dates"`
}

// ChartFreshnessItem is the latest stored chart of a country × provider.
type ChartFreshnessItem struct {
	Country       string     `json:"country"`
	Provider      string     `json:"provider"`
	LatestDate    string     `json:"latest_date"`
	LastScrapedAt *time.Time `json:"last_scraped_at"`
}

const gapWindowDays = 30

func (s *DataQualityService) Report(ctx context.Context) (*DataQualityReport, error) {
	now := s.now().UTC()
	rep := &DataQualityReport{GeneratedAt: now, GapWindowDays: gapWindowDays}
	var err error
	if rep.TitleMatchStatus, err = s.flix.CountTitlesByMatchStatus(ctx); err != nil {
		return nil, err
	}
	if rep.NetflixMatchStatus, err = s.imports.CountNetflixTitlesByMatchStatus(ctx); err != nil {
		return nil, err
	}
	if rep.Metadata, err = s.metadata.Counts(ctx); err != nil {
		return nil, err
	}
	if rep.IMDbRatings, err = s.imports.CountIMDbRatings(ctx); err != nil {
		return nil, err
	}
	if rep.Imports, err = s.imports.ListImports(ctx); err != nil {
		return nil, err
	}
	since := now.AddDate(0, 0, -7*12)
	if rep.Coverage, err = s.ops.WeeklyMappingCoverage(ctx, since); err != nil {
		return nil, err
	}
	if rep.UnmatchedByCountry, err = s.ops.UnmatchedTitlesByCountry(ctx, now.AddDate(0, 0, -gapWindowDays)); err != nil {
		return nil, err
	}
	if rep.StaleCharts, err = s.maintenance.StaleCharts(ctx); err != nil {
		return nil, err
	}
	if rep.StaleCharts == nil {
		rep.StaleCharts = []StaleChart{}
	}
	fresh, err := s.ops.LatestChartDates(ctx)
	if err != nil {
		return nil, err
	}
	for _, f := range fresh {
		rep.FreshnessByChart = append(rep.FreshnessByChart, ChartFreshnessItem{
			Country: f.Country, Provider: f.Provider, LatestDate: repository.FormatDate(f.LatestDate), LastScrapedAt: f.LastScrapedAt,
		})
	}
	if rep.ChartGaps, err = s.gaps(ctx, now); err != nil {
		return nil, err
	}
	return rep, nil
}

// gaps lists, for each chart of an enabled FlixPatrol schedule, the dates in
// the window (from its first stored date on) with no stored chart.
func (s *DataQualityService) gaps(ctx context.Context, now time.Time) ([]ChartGap, error) {
	schedules, err := s.schedules.ListEnabledTaskSchedules(ctx)
	if err != nil {
		return nil, err
	}
	current := flixpatrol.EffectiveTop10Date(now)
	from := current.AddDate(0, 0, -gapWindowDays+1)
	seen := map[string]bool{}
	out := []ChartGap{}
	for _, sch := range schedules {
		if sch.TaskType != tasks.TypeFlixPatrolScheduleRun {
			continue
		}
		for _, t := range sch.Targets {
			code, err := flixpatrol.GetCountryCode(t.CountrySlug)
			if err != nil || seen[string(code)+t.ProviderSlug] {
				continue
			}
			seen[string(code)+t.ProviderSlug] = true
			stored, err := s.flix.StoredChartDates(ctx, string(code), t.ProviderSlug, from, current)
			if err != nil {
				return nil, err
			}
			gap := ChartGap{Country: string(code), Provider: t.ProviderSlug, Missing: []string{}}
			started := false
			for d := from; !d.After(current); d = d.AddDate(0, 0, 1) {
				key := repository.FormatDate(d)
				if stored[key] {
					started = true
					continue
				}
				if started || len(stored) == 0 {
					gap.Missing = append(gap.Missing, key)
				}
			}
			if len(gap.Missing) > 0 {
				out = append(out, gap)
			}
		}
	}
	return out, nil
}
