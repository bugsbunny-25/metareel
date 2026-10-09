package service

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/bugsbunny-25/metareel/internal/repository"
)

func TestAPIKeys(t *testing.T) {
	ctx := context.Background()
	svc := NewAPIKeyService(slog.New(slog.NewTextHandler(io.Discard, nil)), repository.NewAPIKeyRepository(newTestDB(t)))
	if err := svc.Load(ctx); err != nil {
		t.Fatal(err)
	}

	// Required by default, and no keys exist yet.
	if err := svc.Authorize("", "1.2.3.4"); !errors.Is(err, ErrAPIKeyMissing) {
		t.Fatalf("no key: err = %v, want ErrAPIKeyMissing", err)
	}

	created, err := svc.Create(ctx, "tides")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if !strings.HasPrefix(created.Key, "mr_") || len(created.Key) != 46 || !strings.HasPrefix(created.Key, strings.TrimSuffix(created.Prefix, "…")) {
		t.Fatalf("key = %q prefix = %q", created.Key, created.Prefix)
	}
	if err := svc.Authorize(created.Key, "1.2.3.4"); err != nil {
		t.Fatalf("valid key rejected: %v", err)
	}
	if err := svc.Authorize("mr_wrong", "1.2.3.4"); !errors.Is(err, ErrAPIKeyInvalid) {
		t.Fatalf("wrong key: err = %v, want ErrAPIKeyInvalid", err)
	}

	// The plaintext is not stored anywhere.
	keys, err := svc.List(ctx)
	if err != nil || len(keys) != 1 || keys[0].Name != "tides" {
		t.Fatalf("List = %+v, %v", keys, err)
	}

	rotated, err := svc.Rotate(ctx, created.ID)
	if err != nil {
		t.Fatalf("Rotate: %v", err)
	}
	if err := svc.Authorize(created.Key, "1.2.3.4"); !errors.Is(err, ErrAPIKeyInvalid) {
		t.Fatalf("old key after rotation: err = %v, want ErrAPIKeyInvalid", err)
	}
	if err := svc.Authorize(rotated.Key, "1.2.3.4"); err != nil || rotated.RotatedAt == nil {
		t.Fatalf("rotated key: err = %v rotated_at = %v", err, rotated.RotatedAt)
	}

	// Keys optional: anonymous requests pass, wrong keys still fail.
	off := false
	if _, err := svc.UpdateSettings(ctx, UpdateSettingsInput{RequireAPIKey: &off}); err != nil {
		t.Fatal(err)
	}
	if err := svc.Authorize("", "1.2.3.4"); err != nil {
		t.Fatalf("optional key, none sent: %v", err)
	}
	if err := svc.Authorize("mr_wrong", "1.2.3.4"); !errors.Is(err, ErrAPIKeyInvalid) {
		t.Fatalf("optional key, wrong key: err = %v", err)
	}

	// Rate limit: burst is max(limit/4, 10).
	limit := 40
	if _, err := svc.UpdateSettings(ctx, UpdateSettingsInput{RateLimitPerMinute: &limit}); err != nil {
		t.Fatal(err)
	}
	var limited int
	for i := 0; i < 15; i++ {
		if errors.Is(svc.Authorize(rotated.Key, "1.2.3.4"), ErrRateLimited) {
			limited++
		}
	}
	if limited != 5 {
		t.Fatalf("rate limited %d of 15 requests, want 5", limited)
	}
	bad := -1
	var vErr *ValidationError
	if _, err := svc.UpdateSettings(ctx, UpdateSettingsInput{RateLimitPerMinute: &bad}); !errors.As(err, &vErr) {
		t.Fatalf("negative limit: err = %v", err)
	}

	// Settings survive a reload (i.e. a restart).
	reloaded := NewAPIKeyService(svc.log, svc.repo)
	if err := reloaded.Load(ctx); err != nil {
		t.Fatal(err)
	}
	if s := reloaded.Settings(); s.RequireAPIKey || s.RateLimitPerMinute != 40 {
		t.Fatalf("reloaded settings = %+v", s)
	}

	if err := svc.Delete(ctx, created.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if err := svc.Authorize(rotated.Key, "5.6.7.8"); !errors.Is(err, ErrAPIKeyInvalid) {
		t.Fatalf("deleted key: err = %v", err)
	}
	if err := svc.Delete(ctx, created.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("delete twice: err = %v, want sql.ErrNoRows", err)
	}
}

func TestTitlesListFiltersAndSort(t *testing.T) {
	ctx := context.Background()
	_, repo := newTestTop10Service(t)
	seed(t, repo,
		[]seedTitle{
			{Slug: "alpha", Name: "Alpha", Kind: "movie", TmdbID: "1", ImdbID: "tt1", RtURL: "m/alpha"},
			{Slug: "bravo", Name: "bravo", Kind: "movie", TmdbID: "2"},
			{Slug: "charlie-2026", Name: "Charlie", Kind: "tv_show"},
		},
		[]seedRanking{
			{"bravo", "2026-10-01", "US", "netflix", "movies", 1},
			{"bravo", "2026-10-02", "US", "netflix", "movies", 1},
			{"alpha", "2026-10-05", "US", "netflix", "movies", 2},
		})
	svc := NewTitleAdminService(repo)

	slugs := func(q TitlesQuery) []string {
		t.Helper()
		page, err := svc.ListTitlesPage(ctx, q, 50, 0)
		if err != nil {
			t.Fatalf("ListTitlesPage(%+v): %v", q, err)
		}
		var out []string
		for _, it := range page.Items {
			out = append(out, it.Slug)
		}
		if int(page.Total) != len(out) {
			t.Fatalf("total %d != %d items", page.Total, len(out))
		}
		return out
	}
	for _, tc := range []struct {
		q    TitlesQuery
		want string
	}{
		{TitlesQuery{}, "charlie-2026,bravo,alpha"}, // newest first
		{TitlesQuery{Sort: "name_asc"}, "alpha,bravo,charlie-2026"},
		{TitlesQuery{Sort: "name_desc"}, "charlie-2026,bravo,alpha"},
		{TitlesQuery{Sort: "rankings_desc"}, "bravo,alpha,charlie-2026"},
		{TitlesQuery{Sort: "last_ranked_desc"}, "alpha,bravo,charlie-2026"},
		{TitlesQuery{Missing: "tmdb"}, "charlie-2026"},
		{TitlesQuery{Missing: "imdb"}, "charlie-2026,bravo"},
		{TitlesQuery{Missing: "rt", Kind: "movie"}, "bravo"},
		{TitlesQuery{Search: "2026"}, "charlie-2026"}, // matches slug
	} {
		if got := strings.Join(slugs(tc.q), ","); got != tc.want {
			t.Errorf("%+v: got %s, want %s", tc.q, got, tc.want)
		}
	}

	page, _ := svc.ListTitlesPage(ctx, TitlesQuery{Sort: "name_asc"}, 50, 0)
	if a := page.Items[0]; *a.RankingsCount != 1 || *a.LastRankedOn != "2026-10-05" {
		t.Errorf("alpha ranking stats = %v, %v", *a.RankingsCount, *a.LastRankedOn)
	}
	var vErr *ValidationError
	for _, q := range []TitlesQuery{{Sort: "bogus"}, {Missing: "x"}, {Kind: "show"}} {
		if _, err := svc.ListTitlesPage(ctx, q, 50, 0); !errors.As(err, &vErr) {
			t.Errorf("%+v: err = %v, want ValidationError", q, err)
		}
	}
}

func TestTaskRunsFilterAndStats(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	tasks := repository.NewTaskScheduleRepository(db)
	flix := repository.NewFlixPatrolRepository(db)
	for _, stmt := range []string{
		`DELETE FROM task_schedules`, // drop the default job schedules seeded by migrations
		`INSERT INTO task_schedules (id, task_type, name, enabled) VALUES (1, 'flixpatrol.top10.scrape', 'netflix-daily', 1), (2, 'flixpatrol.top10.scrape', 'off', 0)`,
		`INSERT INTO task_schedule_run_times (schedule_id, run_time_utc) VALUES (1, '16:00'), (1, '23:00'), (2, '12:00')`,
		`INSERT INTO task_runs (schedule_id, task_type, status, started_at, finished_at) VALUES
			(1, 'flixpatrol.top10.scrape', 'succeeded', datetime('now', '-3 days'), datetime('now', '-3 days')),
			(1, 'flixpatrol.top10.scrape', 'failed', datetime('now', '-2 hours'), datetime('now', '-2 hours')),
			(2, 'flixpatrol.top10.scrape', 'started', datetime('now', '-1 minutes'), NULL)`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}

	admin := NewTaskScheduleAdminService(tasks, nil, "")
	runs, total, err := admin.ListTaskRuns(ctx, TaskRunsQuery{ScheduleID: 1}, 1, 0)
	if err != nil || total != 2 || len(runs) != 1 || runs[0].Status != "failed" {
		t.Fatalf("schedule 1 page 1: runs=%+v total=%d err=%v", runs, total, err)
	}
	if _, total, _ := admin.ListTaskRuns(ctx, TaskRunsQuery{Status: "started"}, 50, 0); total != 1 {
		t.Fatalf("started total = %d", total)
	}
	var vErr *ValidationError
	if _, _, err := admin.ListTaskRuns(ctx, TaskRunsQuery{Status: "done"}, 50, 0); !errors.As(err, &vErr) {
		t.Fatalf("bad status: err = %v", err)
	}

	stats := NewAdminStatsService(tasks, flix)
	stats.now = func() time.Time { return time.Date(2026, 10, 8, 20, 0, 0, 0, time.UTC) }
	out, err := stats.GetStats(ctx)
	if err != nil {
		t.Fatalf("GetStats: %v", err)
	}
	if out.Schedules.Total != 2 || out.Schedules.Enabled != 1 || out.Runs.Running != 1 {
		t.Fatalf("counts = %+v running=%d", out.Schedules, out.Runs.Running)
	}
	if out.Runs.Last24h["failed"] != 1 || out.Runs.Last24h["succeeded"] != 0 || out.Runs.Last7d["succeeded"] != 1 {
		t.Fatalf("run counts 24h=%v 7d=%v", out.Runs.Last24h, out.Runs.Last7d)
	}
	h := out.Health[0]
	if h.NextRunAt == nil || !h.NextRunAt.Equal(time.Date(2026, 10, 8, 23, 0, 0, 0, time.UTC)) || h.LastRun.Status != "failed" || h.LastSuccessAt == nil {
		t.Fatalf("health[0] = %+v", h)
	}
	if out.Health[1].NextRunAt != nil {
		t.Fatalf("disabled schedule has next run %v", out.Health[1].NextRunAt)
	}
}

func TestNextRunAt(t *testing.T) {
	now := time.Date(2026, 10, 8, 23, 30, 0, 0, time.UTC)
	got := nextRunAt(now, []string{"12:30", "23:00", "bad"})
	if want := time.Date(2026, 10, 9, 12, 30, 0, 0, time.UTC); got == nil || !got.Equal(want) {
		t.Fatalf("nextRunAt = %v, want %v (tomorrow)", got, want)
	}
	if nextRunAt(now, nil) != nil {
		t.Fatal("no runtimes should give nil")
	}
}
