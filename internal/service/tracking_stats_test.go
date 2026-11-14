package service

import (
	"context"
	"testing"

	"github.com/bugsbunny-25/metareel/internal/repository"
)

func TestLongestStreak(t *testing.T) {
	day := func(d string) repository.DayStat { return repository.DayStat{Day: d, Watches: 1} }
	for _, tc := range []struct {
		name    string
		days    []repository.DayStat
		want    int
		wantEnd string
	}{
		{"nothing", nil, 0, ""},
		{"one day", []repository.DayStat{day("2026-03-01")}, 1, "2026-03-01"},
		{"a run", []repository.DayStat{day("2026-03-01"), day("2026-03-02"), day("2026-03-03")}, 3, "2026-03-03"},
		{"a gap resets it", []repository.DayStat{day("2026-03-01"), day("2026-03-03"), day("2026-03-04")}, 2, "2026-03-04"},
		{"across a month end", []repository.DayStat{day("2026-01-30"), day("2026-01-31"), day("2026-02-01")}, 3, "2026-02-01"},
		// 2028 is a leap year: the 29th must not break the run.
		{"across a leap day", []repository.DayStat{day("2028-02-28"), day("2028-02-29"), day("2028-03-01")}, 3, "2028-03-01"},
		{"the longest run wins", []repository.DayStat{
			day("2026-03-01"), day("2026-03-02"), day("2026-03-05"), day("2026-03-06"), day("2026-03-07")}, 3, "2026-03-07"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, end := longestStreak(tc.days)
			if got != tc.want || end != tc.wantEnd {
				t.Errorf("longestStreak = %d ending %q, want %d ending %q", got, end, tc.want, tc.wantEnd)
			}
		})
	}
}

// A streak that ran to yesterday is still a streak, so it does not look
// broken all morning.
func TestCurrentStreak(t *testing.T) {
	day := func(d string) repository.DayStat { return repository.DayStat{Day: d, Watches: 1} }
	days := []repository.DayStat{day("2026-03-09"), day("2026-03-10"), day("2026-03-11")}
	for _, tc := range []struct {
		today string
		want  int
	}{
		{"2026-03-11", 3}, // watched today
		{"2026-03-12", 3}, // watched yesterday
		{"2026-03-13", 0}, // two days off
		{"2026-03-10", 2},
	} {
		if got := currentStreak(days, tc.today); got != tc.want {
			t.Errorf("currentStreak on %s = %d, want %d", tc.today, got, tc.want)
		}
	}
}

func TestTasteBreakdown(t *testing.T) {
	titles := []repository.WatchedTitle{
		{Genres: []string{"Drama", "Crime"}, Countries: []string{"US"}, ReleaseDate: "2008-01-20", Language: "en"},
		{Genres: []string{"Drama"}, Countries: []string{"US", "GB"}, ReleaseDate: "2010-07-15", Language: "en"},
		{Genres: []string{"Science Fiction"}, ReleaseDate: "1999-03-31", Language: "en"},
		{ReleaseDate: ""},
	}
	genres, countries, decades, languages := tasteBreakdown(titles)
	if len(genres) == 0 || genres[0]["genre"] != "Drama" || genres[0]["titles"].(int64) != 2 {
		t.Errorf("genres = %v, want Drama first with 2", genres)
	}
	if countries[0]["country"] != "US" || countries[0]["titles"].(int64) != 2 {
		t.Errorf("countries = %v", countries)
	}
	// 2008 and 2010 are different decades; 1999 is the 1990s.
	byDecade := map[string]int64{}
	for _, d := range decades {
		byDecade[d["decade"].(string)] = d["titles"].(int64)
	}
	if byDecade["2000s"] != 1 || byDecade["2010s"] != 1 || byDecade["1990s"] != 1 {
		t.Errorf("decades = %v", byDecade)
	}
	if languages[0]["language"] != "en" || languages[0]["titles"].(int64) != 3 {
		t.Errorf("languages = %v", languages)
	}
}

// A missing crowd score is left out of the comparison rather than counted
// as a zero, which would drag every gap upwards.
func TestCrowdComparison(t *testing.T) {
	f := func(v float64) *float64 { return &v }
	rows := []repository.RatingComparison{
		{UserRating: 9, TmdbRating: f(8.0), ImdbRating: f(8.5)},
		{UserRating: 6, TmdbRating: f(7.0)},
		{UserRating: 8, TmdbRating: nil, ImdbRating: nil, RtRating: nil},
	}
	out := crowdComparison(rows)
	tmdb := out["tmdb"].(map[string]any)
	if tmdb["compared"].(int64) != 2 {
		t.Errorf("tmdb compared = %v, want 2", tmdb["compared"])
	}
	// (9-8) + (6-7) = 0, over two titles.
	if tmdb["average_gap"].(float64) != 0 {
		t.Errorf("tmdb average_gap = %v, want 0", tmdb["average_gap"])
	}
	if tmdb["above_the_crowd"].(int64) != 1 {
		t.Errorf("tmdb above = %v, want 1", tmdb["above_the_crowd"])
	}
	imdb := out["imdb"].(map[string]any)
	if imdb["compared"].(int64) != 1 || imdb["average_gap"].(float64) != 0.5 {
		t.Errorf("imdb = %v, want 1 compared and a 0.5 gap", imdb)
	}
	rt := out["rotten_tomatoes"].(map[string]any)
	if rt["compared"].(int64) != 0 {
		t.Errorf("rt compared = %v, want 0", rt["compared"])
	}
	if _, ok := rt["average_gap"]; ok {
		t.Error("an average gap was reported with nothing to compare")
	}
}

// How long a watch was is not stored on every row, so the totals fall back
// to the episode's runtime and then the title's.
func TestSummaryCountsTimeWithoutAStoredRuntime(t *testing.T) {
	s, _, alice, _ := newTrackingEnv(t)
	ctx := context.Background()

	// A minute-precision movie: the runtime is stored with it.
	mustLog(t, s, alice, WatchInput{Kind: "movie", TmdbID: "27205", EndedAt: "2026-03-01T22:00:00Z"})
	// An imprecise one: no runtime is stored, but the title knows it.
	mustLog(t, s, alice, WatchInput{Kind: "movie", TmdbID: "27205",
		EndedAt: "2026-03-02T22:00:00Z", Precision: PrecisionDay})
	// An episode: its own runtime, not the show's typical one.
	if _, _, err := s.LogWatch(ctx, alice, WatchInput{Kind: "tv", TmdbID: "1396",
		SeasonNumber: tp(int64(1)), EpisodeNumber: tp(int64(1)), EndedAt: "2026-03-03T22:00:00Z"}); err != nil {
		t.Fatalf("log episode: %v", err)
	}

	out, err := s.Summary(ctx, alice, "", "")
	if err != nil {
		t.Fatalf("summary: %v", err)
	}
	totals := out["totals"].(map[string]any)
	if totals["watches"].(int64) != 3 {
		t.Fatalf("watches = %v, want 3", totals["watches"])
	}
	if totals["movie_watches"].(int64) != 2 || totals["episode_watches"].(int64) != 1 {
		t.Errorf("split = %v movies, %v episodes", totals["movie_watches"], totals["episode_watches"])
	}
	// 148 + 148 (from the title) + 58 (the pilot's own runtime).
	if got := totals["minutes"].(int64); got != 354 {
		t.Errorf("minutes = %d, want 354", got)
	}
	if totals["distinct_titles"].(int64) != 2 {
		t.Errorf("distinct_titles = %v, want 2", totals["distinct_titles"])
	}
	if totals["days"].(int64) != 3 {
		t.Errorf("days = %v, want 3", totals["days"])
	}
}

func TestStatsRangeFiltering(t *testing.T) {
	s, _, alice, _ := newTrackingEnv(t)
	ctx := context.Background()
	for _, d := range []string{"2025-12-31", "2026-01-01", "2026-06-15", "2026-12-31", "2027-01-01"} {
		mustLog(t, s, alice, WatchInput{Kind: "movie", TmdbID: "999",
			EndedAt: d + "T20:00:00Z", Precision: PrecisionDay})
	}
	out, err := s.Summary(ctx, alice, "2026-01-01", "2026-12-31")
	if err != nil {
		t.Fatalf("summary: %v", err)
	}
	if got := out["totals"].(map[string]any)["watches"].(int64); got != 3 {
		t.Errorf("watches in 2026 = %d, want 3", got)
	}

	if _, err := s.Summary(ctx, alice, "2026-13-01", ""); err == nil {
		t.Error("an impossible month was accepted")
	}
	if _, err := s.Summary(ctx, alice, "2026-12-31", "2026-01-01"); err == nil {
		t.Error("a backwards range was accepted")
	}
}

// Spend is reported per currency and never converted between them.
func TestSpendIsPerCurrency(t *testing.T) {
	s, _, alice, _ := newTrackingEnv(t)
	ctx := context.Background()
	venueID, _ := makeVenue(t, s, alice)

	for _, tc := range []struct {
		price    int64
		fees     int64
		currency string
	}{
		{45000, 3500, "INR"},
		{22000, 0, "INR"},
		{1800, 200, "USD"},
	} {
		if _, _, err := s.LogWatch(ctx, alice, WatchInput{
			Kind: "movie", TmdbID: "27205", EndedAt: "2026-03-01T22:00:00Z",
			WatchContextInput: WatchContextInput{
				VenueID: &venueID, PriceMinor: tp(tc.price), FeesMinor: tp(tc.fees),
				Currency: tp(tc.currency)},
		}); err != nil {
			t.Fatalf("log: %v", err)
		}
	}

	out, err := s.Spend(ctx, alice, "", "")
	if err != nil {
		t.Fatalf("spend: %v", err)
	}
	byCurrency := out["by_currency"].([]repository.CurrencySpend)
	totals := map[string]int64{}
	for _, c := range byCurrency {
		totals[c.Currency] = c.TotalMinor
	}
	if totals["INR"] != 70500 {
		t.Errorf("INR total = %d, want 70500", totals["INR"])
	}
	if totals["USD"] != 2000 {
		t.Errorf("USD total = %d, want 2000", totals["USD"])
	}
	if len(byCurrency) != 2 {
		t.Errorf("currencies = %d, want 2 kept apart", len(byCurrency))
	}
}

// The ratings view compares a user against the crowd using real rows from
// the chart-side tables, which tracking reads but never writes.
func TestRatingsAgainstTheCrowd(t *testing.T) {
	s, _, alice, _, db := newTrackingEnvDB(t)
	ctx := context.Background()

	if _, err := db.ExecContext(ctx, `
		INSERT INTO tmdb_titles (tmdb_kind, tmdb_id, title, vote_average, imdb_id)
		VALUES ('movie', '27205', 'Inception', 8.4, 'tt1375666')`); err != nil {
		t.Fatalf("seed tmdb title: %v", err)
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO imdb_ratings (imdb_id, average_rating, num_votes)
		VALUES ('tt1375666', 8.8, 2500000)`); err != nil {
		t.Fatalf("seed imdb rating: %v", err)
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO title_ratings (tmdb_kind, tmdb_id, refreshed_at) VALUES ('movie','27205',CURRENT_TIMESTAMP);
		INSERT INTO title_rating_sources (tmdb_kind, tmdb_id, provider, source, value, fetched_at)
		VALUES ('movie','27205','rottentomatoes','tomatoes', 87, CURRENT_TIMESTAMP)`); err != nil {
		t.Fatalf("seed rt rating: %v", err)
	}

	// Rated 10 half-stars: five stars, which is 10 on the crowd's scale.
	mustLog(t, s, alice, WatchInput{Kind: "movie", TmdbID: "27205",
		EndedAt: "2026-03-01T22:00:00Z", Rating: tp(int64(10))})

	out, err := s.Ratings(ctx, alice, "", "")
	if err != nil {
		t.Fatalf("ratings: %v", err)
	}
	if out["rated"].(int64) != 1 || out["average"].(float64) != 10 {
		t.Errorf("rated = %v, average = %v", out["rated"], out["average"])
	}
	rows := out["titles"].([]repository.RatingComparison)
	if len(rows) != 1 {
		t.Fatalf("comparison rows = %d, want 1", len(rows))
	}
	r := rows[0]
	if r.TmdbRating == nil || *r.TmdbRating != 8.4 {
		t.Errorf("tmdb = %v, want 8.4", r.TmdbRating)
	}
	if r.ImdbRating == nil || *r.ImdbRating != 8.8 {
		t.Errorf("imdb = %v, want 8.8", r.ImdbRating)
	}
	// Rotten Tomatoes is a percentage, brought onto the same 0-10 scale.
	if r.RtRating == nil || *r.RtRating != 8.7 {
		t.Errorf("rt = %v, want 8.7", r.RtRating)
	}
	vs := out["vs_crowd"].(map[string]any)
	if got := vs["imdb"].(map[string]any)["average_gap"].(float64); got != 1.2 {
		t.Errorf("imdb gap = %v, want 1.2", got)
	}
}

func TestYearInReview(t *testing.T) {
	s, _, alice, _ := newTrackingEnv(t)
	ctx := context.Background()
	for _, d := range []string{"2026-01-05", "2026-01-06", "2026-07-20"} {
		mustLog(t, s, alice, WatchInput{Kind: "movie", TmdbID: "27205",
			EndedAt: d + "T20:00:00Z", Precision: PrecisionDay})
	}
	mustLog(t, s, alice, WatchInput{Kind: "movie", TmdbID: "155", EndedAt: "2025-05-05T20:00:00Z", Precision: PrecisionDay})

	out, err := s.YearInReview(ctx, alice, 2026)
	if err != nil {
		t.Fatalf("year in review: %v", err)
	}
	if out["totals"].(map[string]any)["watches"].(int64) != 3 {
		t.Fatalf("2026 watches = %v, want 3", out["totals"].(map[string]any)["watches"])
	}
	months := out["by_month"].([]repository.LabelledStat)
	if len(months) != 2 || months[0].Label != "2026-01" || months[0].Watches != 2 {
		t.Errorf("by_month = %+v", months)
	}
	if got := out["streaks"].(map[string]any)["longest"].(int); got != 2 {
		t.Errorf("longest streak = %d, want 2", got)
	}
	if _, err := s.YearInReview(ctx, alice, 42); err == nil {
		t.Error("a two-digit year was accepted")
	}
}

func TestWatchlistInsightsValidation(t *testing.T) {
	s, _, alice, _ := newTrackingEnv(t)
	ctx := context.Background()
	for _, country := range []string{"", "India"} {
		if _, err := s.WatchlistInsights(ctx, alice, country, 30); err == nil {
			t.Errorf("country %q was accepted", country)
		}
	}
	out, err := s.WatchlistInsights(ctx, alice, "in", 30)
	if err != nil {
		t.Fatalf("insights: %v", err)
	}
	if out["country"].(string) != "IN" {
		t.Errorf("country = %v, want IN uppercased", out["country"])
	}
}

// Watchlist insights only reach into the chart tables; they stay scoped to
// the user's own watchlist.
func TestWatchlistInsightsFindCharts(t *testing.T) {
	s, _, alice, bob, db := newTrackingEnvDB(t)
	ctx := context.Background()

	if _, err := db.ExecContext(ctx, `
		INSERT INTO titles (id, slug, name, kind, tmdb_id) VALUES (1, 'inception-2010', 'Inception', 'movie', '27205');
		INSERT INTO rankings (title_id, ranked_on, country, streaming_provider, category, rank)
		VALUES (1, '2026-03-01', 'IN', 'netflix', 'movies', 3)`); err != nil {
		t.Fatalf("seed chart: %v", err)
	}
	if _, err := s.SetTitleState(ctx, alice, "movie", "27205", TitleStateInput{Status: tp(StatusWatchlist)}); err != nil {
		t.Fatalf("watchlist: %v", err)
	}

	out, err := s.WatchlistInsights(ctx, alice, "IN", 365)
	if err != nil {
		t.Fatalf("insights: %v", err)
	}
	charting := out["charting"].([]repository.ChartHit)
	if len(charting) != 1 || charting[0].BestRank != 3 || charting[0].Provider != "netflix" {
		t.Fatalf("charting = %+v", charting)
	}

	// Bob has not watchlisted it, so he sees nothing.
	out, err = s.WatchlistInsights(ctx, bob, "IN", 365)
	if err != nil {
		t.Fatalf("bob insights: %v", err)
	}
	if got := out["charting"].([]repository.ChartHit); len(got) != 0 {
		t.Errorf("bob sees %d charting titles, want 0", len(got))
	}
}

// Stats never cross between users.
func TestStatsAreScopedToTheirUser(t *testing.T) {
	s, _, alice, bob := newTrackingEnv(t)
	ctx := context.Background()
	mustLog(t, s, alice, WatchInput{Kind: "movie", TmdbID: "27205", EndedAt: "2026-03-01T22:00:00Z"})

	out, err := s.Summary(ctx, bob, "", "")
	if err != nil {
		t.Fatalf("summary: %v", err)
	}
	if got := out["totals"].(map[string]any)["watches"].(int64); got != 0 {
		t.Errorf("bob's watches = %d, want 0", got)
	}
	cal, err := s.Calendar(ctx, bob, "", "")
	if err != nil {
		t.Fatalf("calendar: %v", err)
	}
	if got := cal["days"].([]repository.DayStat); len(got) != 0 {
		t.Errorf("bob's calendar has %d days, want 0", len(got))
	}
}

func TestTopPeopleNeedsCredits(t *testing.T) {
	s, _, alice, _, db := newTrackingEnvDB(t)
	ctx := context.Background()
	if _, err := db.ExecContext(ctx, `
		INSERT INTO tmdb_titles (tmdb_kind, tmdb_id, title) VALUES ('movie','27205','Inception');
		INSERT INTO tmdb_people (person_id, name) VALUES (525, 'Christopher Nolan');
		INSERT INTO tmdb_credits (tmdb_kind, tmdb_id, person_id, credit_type, job, department, credit_order)
		VALUES ('movie','27205',525,'crew','Director','Directing',0)`); err != nil {
		t.Fatalf("seed credits: %v", err)
	}
	mustLog(t, s, alice, WatchInput{Kind: "movie", TmdbID: "27205", EndedAt: "2026-03-01T22:00:00Z"})

	out, err := s.TopPeople(ctx, alice, "", "", jobDirector, 5)
	if err != nil {
		t.Fatalf("top people: %v", err)
	}
	people := out["people"].([]repository.PersonStat)
	if len(people) != 1 || people[0].Name != "Christopher Nolan" || people[0].Titles != 1 {
		t.Fatalf("directors = %+v", people)
	}

	// Cast is a different question and has no rows here.
	out, err = s.TopPeople(ctx, alice, "", "", "", 5)
	if err != nil {
		t.Fatalf("cast: %v", err)
	}
	if got := out["people"].([]repository.PersonStat); len(got) != 0 {
		t.Errorf("cast = %+v, want none", got)
	}
}
