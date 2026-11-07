package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/bugsbunny-25/metareel/internal/client"
	"github.com/bugsbunny-25/metareel/internal/repository"
)

func TestClassifyRelease(t *testing.T) {
	for _, c := range []struct {
		name    string
		dates   []string
		pattern string
		drops   int
		ok      bool
	}{
		{"binge", []string{"2026-01-01", "2026-01-01", "2026-01-01"}, PatternBinge, 1, true},
		{"volumes", []string{"2026-01-01", "2026-01-01", "2026-01-01", "2026-01-01", "2026-02-01", "2026-02-01"}, PatternBatch, 2, true},
		{"three parts", []string{"2026-01-01", "2026-01-01", "2026-01-01", "2026-01-01", "2026-01-25", "2026-01-25", "2026-01-25", "2026-01-31"}, PatternBatch, 3, true},
		{"weekly after a double premiere", []string{"2026-01-01", "2026-01-01", "2026-01-08", "2026-01-15", "2026-01-22", "2026-01-29"}, PatternWeekly, 5, true},
		{"short weekly", []string{"2026-01-01", "2026-01-08", "2026-01-15"}, PatternWeekly, 3, true},
		{"daily", []string{"2026-01-01", "2026-01-02", "2026-01-03", "2026-01-04", "2026-01-05"}, PatternDaily, 5, true},
		{"irregular", []string{"2026-01-01", "2026-01-15", "2026-02-12", "2026-03-30", "2026-04-02"}, PatternIrregular, 5, true},
		{"single", []string{"2026-01-01"}, "", 0, false},
	} {
		p, d, ok := classifyRelease(c.dates)
		if p != c.pattern || d != c.drops || ok != c.ok {
			t.Errorf("%s: got %q %d %v, want %q %d %v", c.name, p, d, ok, c.pattern, c.drops, c.ok)
		}
	}
}

func TestEpisodesDueAndSeasons(t *testing.T) {
	now := time.Date(2026, 11, 6, 6, 0, 0, 0, time.UTC)
	ago := func(h int) string { return now.Add(-time.Duration(h) * time.Hour).Format("2006-01-02 15:04:05") }
	seasons := []repository.TmdbSeason{{Number: 0, AirDate: "2010-01-01"}, {Number: 1, AirDate: "2015-01-01"}, {Number: 2, AirDate: "2024-01-01"},
		{Number: 3, AirDate: "2026-11-20"}, {Number: 4}}
	for _, c := range []struct {
		name string
		t    repository.EpisodeTarget
		want bool
	}{
		{"never fetched", repository.EpisodeTarget{Status: "Ended"}, true},
		{"airing, yesterday", repository.EpisodeTarget{Status: "Ended", Airing: true, FetchedAt: ago(23)}, true},
		{"airing, this morning", repository.EpisodeTarget{Status: "Returning Series", Airing: true, FetchedAt: ago(5)}, false},
		{"season starting soon", repository.EpisodeTarget{Status: "Returning Series", Seasons: seasons, FetchedAt: ago(23)}, true},
		{"returning, 3 days", repository.EpisodeTarget{Status: "Returning Series", FetchedAt: ago(72)}, false},
		{"returning, 7 days", repository.EpisodeTarget{Status: "Returning Series", FetchedAt: ago(7 * 24)}, true},
		{"ended, 20 days", repository.EpisodeTarget{Status: "Ended", FetchedAt: ago(20 * 24)}, false},
		{"ended, 30 days", repository.EpisodeTarget{Status: "Canceled", FetchedAt: ago(30 * 24)}, true},
	} {
		if got := episodesDue(c.t, now); got != c.want {
			t.Errorf("%s: due = %v, want %v", c.name, got, c.want)
		}
	}
	if got := seasonsToFetch(seasons, now.Truncate(24*time.Hour)); fmt.Sprint(got) != "[2 3 4]" {
		t.Errorf("seasons to fetch = %v, want [2 3 4]", got)
	}
	if got := seasonsToFetch([]repository.TmdbSeason{{Number: 1, AirDate: "2001-01-01"}, {Number: 2, AirDate: "2002-01-01"}}, now); fmt.Sprint(got) != "[2]" {
		t.Errorf("old show: seasons to fetch = %v, want the latest only", got)
	}
}

type fakeEpisodes struct {
	data  map[string]map[int][]client.TMDBEpisode
	fail  map[string]bool
	calls []string
}

func (f *fakeEpisodes) Enabled() bool { return true }
func (f *fakeEpisodes) TVSeasonEpisodes(_ context.Context, id string, seasons []int) (map[int][]client.TMDBEpisode, error) {
	f.calls = append(f.calls, fmt.Sprintf("%s%v", id, seasons))
	if f.fail[id] {
		return nil, errors.New("boom")
	}
	show, ok := f.data[id]
	if !ok {
		return nil, nil
	}
	out := map[int][]client.TMDBEpisode{}
	for _, n := range seasons {
		if eps, ok := show[n]; ok {
			out[n] = eps
		}
	}
	return out, nil
}

func ep(n int, date, typ string) client.TMDBEpisode {
	return client.TMDBEpisode{EpisodeNumber: n, Name: fmt.Sprintf("Episode %d", n), AirDate: date, EpisodeType: typ, Runtime: 45}
}

// saveShow stores a TV show with seasons and offers.
func saveShow(t *testing.T, meta *repository.MetadataRepository, id, title string, seasons []client.TMDBSeason, offers ...client.TMDBWatchProvider) {
	t.Helper()
	if err := meta.SaveDetails(context.Background(), &client.TMDBFullDetails{Kind: "tv", ID: id, Title: title, Status: "Returning Series",
		Seasons: seasons, WatchProviders: offers}, true); err != nil {
		t.Fatal(err)
	}
}

func TestEpisodesRefreshAndCalendar(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	meta := repository.NewMetadataRepository(db)
	repo := repository.NewEpisodesRepository(db)
	saveShow(t, meta, "1", "Binge Show", []client.TMDBSeason{{Number: 1, AirDate: "2026-11-10"}},
		offer("US", "flatrate", 8, "Netflix"), offer("US", "rent", 2, "Apple TV"), offer("GB", "flatrate", 8, "Netflix"))
	saveShow(t, meta, "2", "Weekly Show", []client.TMDBSeason{{Number: 1, AirDate: "2025-01-01"}, {Number: 2, AirDate: "2026-11-01"}},
		offer("US", "ads", 300, "Pluto TV"))
	saveShow(t, meta, "3", "Rent Only", []client.TMDBSeason{{Number: 1, AirDate: "2026-11-10"}}, offer("US", "buy", 2, "Apple TV"))
	saveShow(t, meta, "4", "Gone", []client.TMDBSeason{{Number: 1, AirDate: "2026-01-01"}})
	saveShow(t, meta, "5", "Broken", []client.TMDBSeason{{Number: 1, AirDate: "2026-01-01"}})

	src := &fakeEpisodes{fail: map[string]bool{"5": true}, data: map[string]map[int][]client.TMDBEpisode{
		"1": {1: {ep(1, "2026-11-10", "standard"), ep(2, "2026-11-10", "standard"), ep(3, "2026-11-10", "finale")}},
		"2": {1: {ep(1, "2025-01-01", "standard")}, 2: {ep(1, "2026-11-01", "standard"), ep(2, "2026-11-08", "standard"),
			ep(3, "2026-11-15", "mid_season"), ep(4, "", "standard")}},
		"3": {1: {ep(1, "2026-11-10", "standard")}},
	}}
	r := NewEpisodesRefresher(slog.New(slog.NewTextHandler(io.Discard, nil)), src, repo)
	r.gap = 0
	r.now = func() time.Time { return time.Date(2026, 11, 6, 5, 0, 0, 0, time.UTC) }
	rep, err := r.Run(ctx, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Shows != 5 || rep.Due != 5 || rep.Fetched != 4 || rep.Missing != 1 || rep.Failed != 1 || rep.Episodes != 9 {
		t.Fatalf("report = %+v", rep)
	}
	// A second run right away: only the failed show is due.
	src.calls = nil
	if rep, err = r.Run(ctx, false, nil); err == nil || rep.Due != 1 {
		t.Fatalf("second run: %+v, %v; want only the failed show (and an error: every request failed)", rep, err)
	}

	cal := NewCalendarService(repo)
	cal.now = r.now
	got, err := cal.Calendar(ctx, CalendarQuery{Country: "us"})
	if err != nil {
		t.Fatal(err)
	}
	if got.From != "2026-11-06" || got.To != "2026-11-19" {
		t.Errorf("range = %s..%s", got.From, got.To)
	}
	var lines []string
	for _, e := range got.Items {
		var nums []int64
		for _, x := range e.Episodes {
			nums = append(nums, x.EpisodeNumber)
		}
		var offers []string
		for _, o := range e.Offers {
			offers = append(offers, fmt.Sprintf("%s/%s", o.ProviderName, o.Monetization))
		}
		lines = append(lines, fmt.Sprintf("%s %s S%d E%v premiere=%v finale=%v %v", e.AirDate, e.Title, e.SeasonNumber, nums, e.Premiere, e.Finale, offers))
	}
	want := []string{
		"2026-11-08 Weekly Show S2 E[2] premiere=false finale=false [Pluto TV/ads]",
		"2026-11-10 Binge Show S1 E[1 2 3] premiere=true finale=true [Netflix/flatrate]",
		"2026-11-15 Weekly Show S2 E[3] premiere=false finale=false [Pluto TV/ads]",
	}
	if fmt.Sprint(lines) != fmt.Sprint(want) {
		t.Fatalf("calendar:\n%v\nwant:\n%v", joinLines(lines), joinLines(want))
	}

	from := day("2026-11-01")
	netflix, err := cal.Calendar(ctx, CalendarQuery{Country: "US", From: &from, ProviderID: "8"})
	if err != nil || len(netflix.Items) != 1 || netflix.Items[0].Title != "Binge Show" || *netflix.ProviderID != 8 {
		t.Fatalf("Netflix calendar = %+v, %v", netflix, err)
	}
	gb, err := cal.Calendar(ctx, CalendarQuery{Country: "GB"})
	if err != nil || len(gb.Items) != 1 {
		t.Fatalf("GB calendar = %+v, %v", gb, err)
	}

	far := day("2027-03-01")
	for name, q := range map[string]CalendarQuery{
		"no country":   {},
		"bad country":  {Country: "zz"},
		"bad provider": {Country: "US", ProviderID: "netflix"},
		"long range":   {Country: "US", From: &from, To: &far},
		"reversed":     {Country: "US", From: &far, To: &from},
	} {
		var verr *ValidationError
		if _, err := cal.Calendar(ctx, q); !errors.As(err, &verr) {
			t.Errorf("%s: err = %v, want a ValidationError", name, err)
		}
	}
}

// TestReleasePatterns: a binge season stays longer than a weekly one in
// this data; unaired, single-episode and uncharted seasons are excluded,
// and a chart not stored over the whole window doesn't count.
func TestReleasePatterns(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	meta := repository.NewMetadataRepository(db)
	repo := repository.NewEpisodesRepository(db)
	for id, title := range map[string]string{"10": "Binge", "20": "Weekly", "30": "Unaired", "40": "Uncharted", "50": "Single"} {
		saveShow(t, meta, id, title, []client.TMDBSeason{{Number: 1, AirDate: "2026-06-01"}})
	}
	weekly := []client.TMDBEpisode{}
	for i := 0; i < 6; i++ {
		weekly = append(weekly, ep(i+1, repository.FormatDate(day("2026-06-01").AddDate(0, 0, 7*i)), "standard"))
	}
	for id, eps := range map[string]map[int][]client.TMDBEpisode{
		"10": {1: {ep(1, "2026-06-01", ""), ep(2, "2026-06-01", ""), ep(3, "2026-06-01", "finale")}},
		"20": {1: weekly},
		"30": {1: {ep(1, "2026-06-01", ""), ep(2, "", "")}},
		"40": {1: {ep(1, "2026-06-01", ""), ep(2, "2026-06-01", "")}},
		"50": {1: {ep(1, "2026-06-01", "")}},
	} {
		if err := repo.SaveEpisodes(ctx, id, eps); err != nil {
			t.Fatal(err)
		}
	}
	var titles = []seedTitle{{Slug: "binge", Name: "Binge", Kind: "tv_show", TmdbID: "10"}, {Slug: "weekly", Name: "Weekly", Kind: "tv_show", TmdbID: "20"},
		{Slug: "filler", Name: "Filler", Kind: "tv_show"}}
	var rankings []seedRanking
	start := day("2026-05-20")
	for i := 0; i < 90; i++ {
		d := repository.FormatDate(start.AddDate(0, 0, i))
		rankings = append(rankings, seedRanking{"filler", d, "US", "netflix", "tv_shows", 10})
		off := int(day(d).Sub(day("2026-06-01")).Hours() / 24)
		if off >= 0 && off < 40 { // binge: 40 days in a row
			rankings = append(rankings, seedRanking{"binge", d, "US", "netflix", "tv_shows", 1})
		}
		if off >= 0 && off < 10 { // weekly: 10 days
			rankings = append(rankings, seedRanking{"weekly", d, "US", "netflix", "tv_shows", 5})
		}
		// A chart that started after the premiere: ignored.
		if off >= 5 && off < 30 {
			rankings = append(rankings, seedRanking{"weekly", d, "GB", "netflix", "tv_shows", 1})
		}
	}
	seed(t, db, titles, rankings)

	svc := NewAnalyticsService(repository.NewAnalyticsRepository(db), meta, repository.NewImportsRepository(db), repository.NewRatingsRepository(db)).WithEpisodes(repo)
	svc.now = func() time.Time { return day("2026-08-20") }
	svc.charts.now = svc.now
	from, to := day("2026-05-01"), day("2026-07-01")
	got, err := svc.ReleasePatterns(ctx, AnalyticsQuery{From: &from, To: &to})
	if err != nil {
		t.Fatal(err)
	}
	if got.Excluded != (ReleaseExcluded{Unaired: 1, Single: 1, Uncharted: 1}) {
		t.Errorf("excluded = %+v", got.Excluded)
	}
	var ps []string
	for _, p := range got.Patterns {
		ps = append(ps, fmt.Sprintf("%s seasons=%d runs=%d days=%g last=%g stayed=%g best=%g points=%g",
			p.Pattern, p.Seasons, p.ChartRuns, p.MedianDaysInTop10, p.MedianLastDay, p.ShareStayed, p.MedianBestRank, p.MedianPoints))
	}
	want := []string{
		"binge seasons=1 runs=1 days=40 last=39 stayed=1 best=1 points=400",
		"weekly seasons=1 runs=1 days=10 last=9 stayed=0 best=5 points=60",
	}
	if fmt.Sprint(ps) != fmt.Sprint(want) {
		t.Fatalf("patterns:\n%v\nwant:\n%v", joinLines(ps), joinLines(want))
	}
	if len(got.Seasons) != 2 || got.Seasons[0].Title != "Binge" || got.Seasons[1].Drops != 6 || got.WindowDays != 60 {
		t.Errorf("seasons = %+v", got.Seasons)
	}

	// A shorter window counts fewer days.
	short, err := svc.ReleasePatterns(ctx, AnalyticsQuery{From: &from, To: &to, Window: "30"})
	if err != nil {
		t.Fatal(err)
	}
	if len(short.Seasons) != 2 || short.Patterns[0].MedianDaysInTop10 != 30 {
		t.Errorf("30-day window: %+v", short.Patterns)
	}
	// Charts end 2026-08-17: a 90-day window (to 2026-08-29) is not over.
	long, err := svc.ReleasePatterns(ctx, AnalyticsQuery{From: &from, To: &to, Window: "90"})
	if err != nil {
		t.Fatal(err)
	}
	if len(long.Seasons) != 0 || long.Excluded.InProgress != 2 || long.Excluded.Uncharted != 1 {
		t.Errorf("90-day window: seasons %d, excluded %+v; want 2 in progress", len(long.Seasons), long.Excluded)
	}
	var verr *ValidationError
	if _, err := svc.ReleasePatterns(ctx, AnalyticsQuery{Window: "7"}); !errors.As(err, &verr) {
		t.Errorf("window 7: err = %v, want a ValidationError", err)
	}
}
