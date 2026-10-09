package repository

import (
	"path/filepath"
	"testing"

	"github.com/pressly/goose/v3"
)

// TestMigration5KeepsRunsAndLogs checks the task_schedules / task_runs
// rebuild in 000005 keeps existing schedules, runs and their logs (dropping
// a parent table with foreign keys on would cascade-delete them) and that
// 000006 converts ranking dates to ISO form.
func TestMigration5KeepsRunsAndLogs(t *testing.T) {
	db, err := Open("file:" + filepath.Join(t.TempDir(), "m.db") + "?_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := goose.SetDialect("sqlite3"); err != nil {
		t.Fatal(err)
	}
	goose.SetLogger(goose.NopLogger())
	const dir = "../../db/migrations"
	if err := goose.UpTo(db, dir, 4); err != nil {
		t.Fatalf("migrate to 4: %v", err)
	}
	for _, stmt := range []string{
		`INSERT INTO task_schedules (id, task_type, name) VALUES (7, 'flixpatrol.top10.scrape', 'daily')`,
		`INSERT INTO task_schedule_targets (schedule_id, country_slug, provider_slug) VALUES (7, 'united-states', 'netflix')`,
		`INSERT INTO task_schedule_run_times (schedule_id, run_time_utc) VALUES (7, '13:00')`,
		`INSERT INTO task_runs (id, schedule_id, task_type, status) VALUES (40, 7, 'flixpatrol.top10.scrape', 'succeeded')`,
		`INSERT INTO task_run_logs (run_id, level, message) VALUES (40, 'info', 'hello'), (40, 'info', 'bye')`,
		`INSERT INTO titles (id, slug, name, kind, tmdb_id) VALUES (1, 'a', 'A', 'movie', '5'), (2, 'b', 'B', 'movie', NULL)`,
		`INSERT INTO rankings (title_id, ranked_on, country, streaming_provider, category, rank) VALUES
			(1, '2026-04-28 00:00:00 +0000 UTC', 'US', 'netflix', 'movies', 1),
			(2, '2026-04-28 00:00:00 +0000 UTC', 'US', 'netflix', 'movies', 2)`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("seed %q: %v", stmt, err)
		}
	}
	if err := goose.Up(db, dir); err != nil {
		t.Fatalf("migrate up: %v", err)
	}

	var n int
	mustScan := func(q string, dst ...any) {
		t.Helper()
		if err := db.QueryRow(q).Scan(dst...); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	mustScan(`SELECT COUNT(*) FROM task_run_logs WHERE run_id = 40`, &n)
	if n != 2 {
		t.Fatalf("run logs after migration = %d, want 2", n)
	}
	mustScan(`SELECT COUNT(*) FROM task_schedule_targets WHERE schedule_id = 7`, &n)
	if n != 1 {
		t.Fatalf("targets after migration = %d, want 1", n)
	}
	var backfill, recheck int
	mustScan(`SELECT backfill_days, recheck_hours FROM task_schedules WHERE id = 7`, &backfill, &recheck)
	if backfill != 3 || recheck != 6 {
		t.Fatalf("backfill/recheck = %d/%d", backfill, recheck)
	}
	var date, sig, status string
	mustScan(`SELECT DISTINCT CAST(ranked_on AS TEXT) FROM rankings`, &date)
	if date != "2026-04-28" {
		t.Fatalf("ranked_on = %q, want ISO date", date)
	}
	mustScan(`SELECT signature FROM chart_snapshots WHERE ranked_on = '2026-04-28'`, &sig)
	if sig != "1:a|2:b" {
		t.Fatalf("snapshot signature = %q", sig)
	}
	mustScan(`SELECT match_status FROM titles WHERE id = 2`, &status)
	if status != "unmatched" {
		t.Fatalf("unmapped title status = %q", status)
	}
	mustScan(`SELECT COUNT(*) FROM task_schedules WHERE task_type <> 'flixpatrol.top10.scrape'`, &n)
	if n != 6 {
		t.Fatalf("default job schedules = %d, want 6", n)
	}
	// Foreign keys must be back on and consistent.
	rows, err := db.Query(`PRAGMA foreign_key_check`)
	if err != nil {
		t.Fatal(err)
	}
	if rows.Next() {
		t.Fatal("foreign key violations after migration")
	}
	rows.Close()
	mustScan(`PRAGMA foreign_keys`, &n)
	if n != 1 {
		t.Fatal("foreign_keys left off")
	}

	// And back down to 4 keeps the FlixPatrol schedule's run.
	if err := goose.DownTo(db, dir, 4); err != nil {
		t.Fatalf("migrate down: %v", err)
	}
	mustScan(`SELECT COUNT(*) FROM task_run_logs WHERE run_id = 40`, &n)
	if n != 2 {
		t.Fatalf("run logs after down = %d", n)
	}
}
