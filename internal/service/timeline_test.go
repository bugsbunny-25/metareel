package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/bugsbunny-25/metareel/internal/client"
	"github.com/bugsbunny-25/metareel/internal/repository"
	"github.com/bugsbunny-25/metareel/internal/repository/sqlc"
)

func newTimelineService(db *sql.DB) *TimelineService {
	meta := repository.NewMetadataRepository(db)
	top := NewTop10ReadService(repository.NewFlixPatrolRepository(db)).WithMetadata(meta)
	return NewTimelineService(top, meta, repository.NewHistoryRepository(db), repository.NewAnalyticsRepository(db),
		repository.NewEngagementRepository(db))
}

// seedTimeline stores one TV show (tmdb tv:500) with data in every source.
func seedTimeline(t *testing.T, db *sql.DB) TmdbRef {
	t.Helper()
	ctx := context.Background()
	ref := TmdbRef{Kind: "tv", ID: "500"}
	seed(t, db,
		[]seedTitle{{Slug: "show", Name: "Show", Kind: "tv_show", TmdbID: "500"}, {Slug: "other", Name: "Other", Kind: "tv_show"}},
		[]seedRanking{
			{"show", "2026-10-01", "US", "netflix", "tv_shows", 5}, // debut
			{"show", "2026-10-02", "US", "netflix", "tv_shows", 2},
			{"show", "2026-10-03", "US", "netflix", "tv_shows", 1}, // peak, #1 run of 2 days
			{"show", "2026-10-04", "US", "netflix", "tv_shows", 1},
			{"show", "2026-10-05", "US", "netflix", "tv_shows", 3},
			{"other", "2026-10-10", "US", "netflix", "tv_shows", 1}, // stored without the show
			{"show", "2026-10-20", "US", "netflix", "tv_shows", 4},  // re-entry after 14 days away
			{"show", "2026-10-02", "GB", "netflix", "tv_shows", 1},  // debut at #1
			{"show", "2026-10-20", "GB", "netflix", "tv_shows", 2},  // GB not stored in between: no re-entry
		})

	meta := repository.NewMetadataRepository(db)
	details := func(offers ...client.TMDBWatchProvider) {
		t.Helper()
		if err := meta.SaveDetails(ctx, &client.TMDBFullDetails{Kind: "tv", ID: "500", Title: "Show", ReleaseDate: "2025-06-01", IMDbID: "tt0500",
			Seasons: []client.TMDBSeason{
				{Number: 0, Name: "Specials", AirDate: "2025-01-01"},
				{Number: 1, Name: "Season 1", AirDate: "2025-06-01", EpisodeCount: 8},
				{Number: 2, Name: "Season 2", AirDate: "2026-12-01", EpisodeCount: 1},
			}, WatchProviders: offers}, true); err != nil {
			t.Fatal(err)
		}
	}
	details(offer("US", "flatrate", 8, "Netflix"))
	details(offer("US", "flatrate", 8, "Netflix"), offer("GB", "flatrate", 8, "Netflix"), offer("DE", "flatrate", 8, "Netflix"))

	q := sqlc.New(db)
	rating := func(provider, source, date string, v float64) {
		t.Helper()
		if err := q.RecordTitleRating(ctx, sqlc.RecordTitleRatingParams{TmdbKind: "tv", TmdbID: "500", Provider: provider, Source: source, CapturedOn: date, Value: v}); err != nil {
			t.Fatal(err)
		}
	}
	rating("justwatch", "imdb", "2026-10-01", 7.0) // replaced by the IMDb dataset
	rating("justwatch", "imdb", "2026-10-05", 7.5)
	rating("justwatch", "tomatoes", "2026-10-01", 70) // replaced by MDBList
	rating("mdblist", "tomatoes", "2026-10-01", 80)
	rating("mdblist", "tomatoes", "2026-10-02", 80.2) // displays as 80: no event
	rating("mdblist", "tomatoes", "2026-10-06", 85)
	rating("justwatch", "tmdb_popularity", "2026-10-01", 10) // not a rating
	rating("justwatch", "tmdb_popularity", "2026-10-02", 50)
	for _, p := range []struct {
		date  string
		value float64
		votes int64
	}{{"2026-10-01", 6.9, 100}, {"2026-10-02", 6.9, 150}, {"2026-10-03", 7.1, 300}} {
		if err := q.RecordIMDbRating(ctx, sqlc.RecordIMDbRatingParams{ImdbID: "tt0500", CapturedOn: p.date, AverageRating: p.value, NumVotes: p.votes}); err != nil {
			t.Fatal(err)
		}
	}

	if err := meta.SaveEnwiki(ctx, ref, "Show (TV series)"); err != nil {
		t.Fatal(err)
	}
	var views []repository.PageviewPoint
	start := day("2026-09-10")
	for i := 0; i < 20; i++ {
		views = append(views, repository.PageviewPoint{Date: repository.FormatDate(start.AddDate(0, 0, i)), Views: 400})
	}
	views = append(views, repository.PageviewPoint{Date: "2026-09-30", Views: 2000}, repository.PageviewPoint{Date: "2026-10-01", Views: 3000},
		repository.PageviewPoint{Date: "2026-10-02", Views: 500})
	if err := repository.NewHistoryRepository(db).SavePageviews(ctx, ref, pageviewsProject, views); err != nil {
		t.Fatal(err)
	}

	for _, stmt := range []string{
		`INSERT INTO netflix_titles (id, show_title, kind, match_status, tmdb_kind, tmdb_id) VALUES (1, 'Show', 'tv_show', 'matched', 'tv', '500')`,
		`INSERT INTO netflix_seasons (netflix_title_id, season_title, season_number) VALUES (1, 'Show: Season 1', 1)`,
		`INSERT INTO netflix_top10_global (week, category, weekly_rank, netflix_title_id, show_title, season_title, weekly_views, cumulative_weeks)
		 VALUES ('2026-10-04', 'TV (English)', 2, 1, 'Show', 'Show: Season 1', 1200000, 1)`,
		`INSERT INTO netflix_top10_countries (country, week, category, weekly_rank, netflix_title_id, show_title, season_title, cumulative_weeks) VALUES
		 ('US', '2026-10-04', 'TV', 1, 1, 'Show', 'Show: Season 1', 1), ('GB', '2026-10-04', 'TV', 3, 1, 'Show', 'Show: Season 1', 1),
		 ('US', '2026-10-11', 'TV', 5, 1, 'Show', 'Show: Season 1', 2)`,
		`INSERT INTO netflix_engagement (period_start, period_end, kind, title, hours_viewed, views, netflix_title_id, season_number)
		 VALUES ('2025-07-01', '2025-12-31', 'tv_show', 'Show: Season 1', 5000000, 900000, 1, 1)`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}
	return ref
}

func TestTimeline(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	svc := newTimelineService(db)
	ref := seedTimeline(t, db)

	got, err := svc.Timeline(ctx, ref, TimelineQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if got.Title == nil || *got.Title != "Show" {
		t.Errorf("title = %v, want Show", got.Title)
	}
	for i := 1; i < len(got.Events); i++ {
		if got.Events[i-1].Date < got.Events[i].Date {
			t.Fatalf("events not newest first: %s before %s", got.Events[i-1].Date, got.Events[i].Date)
		}
	}
	today := repository.FormatDate(time.Now().UTC())
	key := func(e TimelineEvent) string {
		s := e.Type + " " + e.Date
		if e.EndDate != nil {
			s += ".." + *e.EndDate
		}
		switch {
		case e.Chart != nil:
			s += fmt.Sprintf(" %s #%d", e.Chart.Country, e.Chart.Rank)
			if e.Chart.Days != nil {
				s += fmt.Sprintf(" %dd", *e.Chart.Days)
			}
		case e.Availability != nil:
			s += fmt.Sprintf(" %s %v", e.Availability.ProviderName, e.Availability.Countries)
		case e.Rating != nil:
			s += fmt.Sprintf(" %s/%s %g->%g", e.Rating.Provider, e.Rating.Source, e.Rating.PreviousValue, e.Rating.Value)
		case e.Pageviews != nil:
			p := e.Pageviews
			s += fmt.Sprintf(" peak %s %d base %d x%.1f %dd", p.PeakDay, p.PeakViews, p.BaselineViews, p.Ratio, p.Days)
		case e.Netflix != nil:
			n := e.Netflix
			s += fmt.Sprintf(" global=%v countries=%d", rankStr(n.GlobalRank), n.Countries)
		case e.Engagement != nil:
			s += fmt.Sprintf(" views=%d", *e.Engagement.Views)
		case e.Season != nil:
			s += fmt.Sprintf(" S%d", e.Season.SeasonNumber)
		}
		return s
	}
	want := []string{
		"season_premiere 2026-12-01 S2",
		"availability_added " + today + " Netflix [DE GB]",
		"chart_reentry 2026-10-20 US #4 14d",
		"rating_changed 2026-10-06 mdblist/tomatoes 80->85",
		"netflix_top10_week 2026-10-05..2026-10-11 global=<nil> countries=1",
		"chart_peak 2026-10-03 US #1 2d",
		"chart_number_one 2026-10-03..2026-10-04 US #1 2d",
		"rating_changed 2026-10-03 imdb_dataset/imdb 6.9->7.1",
		"chart_debut 2026-10-02 GB #1",
		"chart_number_one 2026-10-02 GB #1 1d",
		"chart_debut 2026-10-01 US #5",
		"pageview_spike 2026-09-30..2026-10-01 peak 2026-10-01 3000 base 400 x7.5 2d",
		"netflix_top10_week 2026-09-28..2026-10-04 global=2 countries=2",
		"netflix_engagement 2025-07-01..2025-12-31 views=900000",
		"release 2025-06-01",
		"season_premiere 2025-06-01 S1",
	}
	var keys []string
	for _, e := range got.Events {
		keys = append(keys, key(e))
		if e.Summary == "" {
			t.Errorf("event %s has no summary", key(e))
		}
	}
	if fmt.Sprint(keys) != fmt.Sprint(want) {
		t.Fatalf("events:\n%v\nwant:\n%v", joinLines(keys), joinLines(want))
	}
	if got.Total != len(want) || got.Counts[TimelineChartDebut] != 2 || got.Counts[TimelineRatingChanged] != 2 {
		t.Errorf("total = %d, counts = %v", got.Total, got.Counts)
	}

	// Type filter.
	ones, err := svc.Timeline(ctx, ref, TimelineQuery{Types: "chart_number_one, chart_peak"})
	if err != nil {
		t.Fatal(err)
	}
	if ones.Total != 3 {
		t.Errorf("number one + peak events = %d, want 3", ones.Total)
	}

	// Country filter: US chart events and US Netflix weeks with the US rank;
	// worldwide events stay; DE/GB availability goes.
	us, err := svc.Timeline(ctx, ref, TimelineQuery{Country: "us", Types: "chart_debut,netflix_top10_week,availability_added,release"})
	if err != nil {
		t.Fatal(err)
	}
	var usKeys []string
	for _, e := range us.Events {
		k := key(e)
		if e.Netflix != nil && e.Netflix.CountryRank != nil {
			k += fmt.Sprintf(" us=%d", *e.Netflix.CountryRank)
		}
		usKeys = append(usKeys, k)
	}
	wantUS := []string{
		"netflix_top10_week 2026-10-05..2026-10-11 global=<nil> countries=1 us=5",
		"chart_debut 2026-10-01 US #5",
		"netflix_top10_week 2026-09-28..2026-10-04 global=2 countries=2 us=1",
		"release 2025-06-01",
	}
	if fmt.Sprint(usKeys) != fmt.Sprint(wantUS) {
		t.Fatalf("US events:\n%v\nwant:\n%v", joinLines(usKeys), joinLines(wantUS))
	}

	// Range: spans overlapping the range are kept.
	from, to := day("2026-10-04"), day("2026-10-06")
	ranged, err := svc.Timeline(ctx, ref, TimelineQuery{From: &from, To: &to})
	if err != nil {
		t.Fatal(err)
	}
	var rangedKeys []string
	for _, e := range ranged.Events {
		rangedKeys = append(rangedKeys, key(e))
	}
	wantRanged := []string{
		"rating_changed 2026-10-06 mdblist/tomatoes 80->85",
		"netflix_top10_week 2026-10-05..2026-10-11 global=<nil> countries=1",
		"chart_number_one 2026-10-03..2026-10-04 US #1 2d",
		"netflix_top10_week 2026-09-28..2026-10-04 global=2 countries=2",
	}
	if fmt.Sprint(rangedKeys) != fmt.Sprint(wantRanged) {
		t.Fatalf("ranged events:\n%v\nwant:\n%v", joinLines(rangedKeys), joinLines(wantRanged))
	}

	// A narrow range still has the spike's baseline (read from before
	// `from`) and finds re-entries dated inside it.
	for _, c := range []struct{ from, to, types, want string }{
		{"2026-10-01", "2026-10-01", "pageview_spike", "pageview_spike 2026-09-30..2026-10-01 peak 2026-10-01 3000 base 400 x7.5 2d"},
		{"2026-10-15", "2026-10-25", "chart_reentry", "chart_reentry 2026-10-20 US #4 14d"},
	} {
		f, l := day(c.from), day(c.to)
		got, err := svc.Timeline(ctx, ref, TimelineQuery{From: &f, To: &l, Types: c.types})
		if err != nil {
			t.Fatal(err)
		}
		if len(got.Events) != 1 || key(got.Events[0]) != c.want {
			var ks []string
			for _, e := range got.Events {
				ks = append(ks, key(e))
			}
			t.Errorf("%s..%s %s = %v, want %s", c.from, c.to, c.types, ks, c.want)
		}
	}

	// Limit keeps the newest and reports the total.
	limited, err := svc.Timeline(ctx, ref, TimelineQuery{Limit: 3})
	if err != nil || len(limited.Events) != 3 || limited.Total != len(want) {
		t.Fatalf("limited = %d events of %d, %v", len(limited.Events), limited.Total, err)
	}
}

func TestTimelineErrors(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	svc := newTimelineService(db)
	ref := seedTimeline(t, db)
	from, to := day("2026-10-05"), day("2026-10-01")
	for name, q := range map[string]TimelineQuery{
		"unknown type": {Types: "chart_debut,nope"},
		"bad country":  {Country: "zz"},
		"bad range":    {From: &from, To: &to},
		"limit":        {Limit: maxTimelineLimit + 1},
	} {
		var verr *ValidationError
		if _, err := svc.Timeline(ctx, ref, q); !errors.As(err, &verr) {
			t.Errorf("%s: err = %v, want a ValidationError", name, err)
		}
	}
	if _, err := svc.Timeline(ctx, TmdbRef{Kind: "movie", ID: "999"}, TimelineQuery{}); !errors.Is(err, ErrTitleNotFound) {
		t.Errorf("unknown title: err = %v, want ErrTitleNotFound", err)
	}
}

// TestPageviewSpikes: no spike without a baseline, and a run ends when a day
// drops below the threshold.
func TestPageviewSpikes(t *testing.T) {
	var pts []repository.PageviewPoint
	start := day("2026-01-01")
	add := func(i int, v int64) {
		pts = append(pts, repository.PageviewPoint{Date: repository.FormatDate(start.AddDate(0, 0, i)), Views: v})
	}
	add(0, 50000) // huge, but no baseline yet
	for i := 1; i <= 20; i++ {
		add(i, 1000)
	}
	add(21, 2500) // 2.5x: not a spike
	add(22, 4000) // spike
	add(23, 900)
	add(24, 5000) // a new spike (not consecutive with the first)
	got := pageviewSpikes(pts)
	if len(got) != 2 || got[0].Date != "2026-01-23" || got[1].Date != "2026-01-25" || got[0].EndDate != nil {
		var ds []string
		for _, e := range got {
			ds = append(ds, e.Date)
		}
		t.Fatalf("spikes = %v, want 2026-01-23 and 2026-01-25", ds)
	}
	if got[0].Pageviews.BaselineViews != 1000 || got[0].Pageviews.Ratio != 4 {
		t.Errorf("first spike = %+v", got[0].Pageviews)
	}
}

func joinLines(s []string) string {
	out := ""
	for _, l := range s {
		out += "  " + l + "\n"
	}
	return out
}

func rankStr(p *int64) string {
	if p == nil {
		return "<nil>"
	}
	return fmt.Sprint(*p)
}
