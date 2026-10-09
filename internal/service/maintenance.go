package service

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/bugsbunny-25/metareel/internal/repository"
	"github.com/bugsbunny-25/metareel/internal/scraper/flixpatrol"
	"github.com/bugsbunny-25/metareel/internal/tasks"
)

// MaintenanceService is the maintenance job: run retention, stale-chart
// alerts and SQLite housekeeping.
type MaintenanceService struct {
	log             *slog.Logger
	ops             *repository.OpsRepository
	schedules       *repository.TaskScheduleRepository
	alerts          *AlertService
	retentionDays   int
	staleChartHours int
	now             func() time.Time
}

func NewMaintenanceService(log *slog.Logger, ops *repository.OpsRepository, schedules *repository.TaskScheduleRepository, alerts *AlertService, retentionDays, staleChartHours int) *MaintenanceService {
	return &MaintenanceService{log: log, ops: ops, schedules: schedules, alerts: alerts,
		retentionDays: retentionDays, staleChartHours: staleChartHours, now: time.Now}
}

// MaintenanceReport summarises a maintenance run.
type MaintenanceReport struct {
	RunsDeleted    int64        `json:"runs_deleted"`
	LogsDeleted    int64        `json:"logs_deleted"`
	StaleRunsFixed int64        `json:"stale_runs_failed"`
	StaleCharts    []StaleChart `json:"stale_charts"`
	AlertsSent     int          `json:"alerts_sent"`
}

// StaleChart is a scheduled chart whose next date is overdue.
type StaleChart struct {
	Country      string  `json:"country"`
	Provider     string  `json:"provider"`
	LatestDate   *string `json:"latest_date"` // nil = never scraped
	OverdueHours int     `json:"overdue_hours"`
}

func (m *MaintenanceService) Run(ctx context.Context, runLog func(level, msg string)) (*MaintenanceReport, error) {
	logf := func(level, format string, args ...any) {
		if runLog != nil {
			runLog(level, fmt.Sprintf(format, args...))
		}
	}
	rep := &MaintenanceReport{StaleCharts: []StaleChart{}}

	n, err := m.schedules.FailStaleTaskRuns(ctx, fmt.Sprintf("-%d minutes", int(tasks.MaxTimeout.Minutes())+10))
	if err != nil {
		return rep, err
	}
	rep.StaleRunsFixed = n

	if m.retentionDays > 0 {
		runs, logs, err := m.schedules.PruneTaskRuns(ctx, fmt.Sprintf("-%d days", m.retentionDays))
		if err != nil {
			return rep, err
		}
		rep.RunsDeleted, rep.LogsDeleted = runs, logs
		logf("info", "Retention: deleted %d runs and %d log lines older than %d days", runs, logs, m.retentionDays)
	}

	stale, err := m.StaleCharts(ctx)
	if err != nil {
		return rep, err
	}
	if stale != nil {
		rep.StaleCharts = stale
	}
	if len(stale) == 0 {
		logf("info", "Every scheduled chart is up to date")
	}
	for _, s := range stale {
		latest := "never"
		if s.LatestDate != nil {
			latest = *s.LatestDate
		}
		logf("warn", "Stale chart %s / %s: latest %s, next date overdue by %dh", s.Country, s.Provider, latest, s.OverdueHours)
		sent, _ := m.alerts.Send(ctx, Alert{
			Key: "stale_chart:" + s.Country + ":" + s.Provider, Level: "warn",
			Title:   fmt.Sprintf("Stale chart: %s %s", s.Provider, s.Country),
			Message: fmt.Sprintf("Latest stored chart is %s; the next one has been due for %d hours.", latest, s.OverdueHours),
		})
		if sent {
			rep.AlertsSent++
		}
	}

	if err := m.ops.Optimize(ctx); err != nil {
		logf("warn", "SQLite optimize: %v", err)
	}
	return rep, nil
}

// StaleCharts lists charts of enabled FlixPatrol schedules whose next date
// has been due (12:00 UTC on that date) for longer than the threshold.
func (m *MaintenanceService) StaleCharts(ctx context.Context) ([]StaleChart, error) {
	schedules, err := m.schedules.ListEnabledTaskSchedules(ctx)
	if err != nil {
		return nil, err
	}
	latest, err := m.ops.LatestChartDates(ctx)
	if err != nil {
		return nil, err
	}
	byChart := map[string]repository.ChartFreshness{}
	for _, f := range latest {
		byChart[f.Country+"|"+f.Provider] = f
	}
	now := m.now().UTC()
	seen := map[string]bool{}
	var out []StaleChart
	for _, s := range schedules {
		if s.TaskType != tasks.TypeFlixPatrolScheduleRun {
			continue
		}
		for _, t := range s.Targets {
			code, err := flixpatrol.GetCountryCode(t.CountrySlug)
			if err != nil {
				continue
			}
			key := string(code) + "|" + t.ProviderSlug
			if seen[key] {
				continue
			}
			seen[key] = true
			sc := StaleChart{Country: string(code), Provider: t.ProviderSlug}
			f, ok := byChart[key]
			due := now.Add(-time.Duration(m.staleChartHours+24) * time.Hour) // never scraped: overdue after a day
			if ok {
				d := repository.FormatDate(f.LatestDate)
				sc.LatestDate = &d
				due = f.LatestDate.AddDate(0, 0, 1).Add(12 * time.Hour)
			}
			overdue := now.Sub(due)
			if overdue > time.Duration(m.staleChartHours)*time.Hour {
				sc.OverdueHours = int(overdue.Hours())
				out = append(out, sc)
			}
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return strings.Compare(out[i].Country+out[i].Provider, out[j].Country+out[j].Provider) < 0
	})
	return out, nil
}
