package service

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/bugsbunny-25/metareel/internal/client"
	"github.com/bugsbunny-25/metareel/internal/repository"
)

func TestResolveNetflixSeason(t *testing.T) {
	monster := []repository.TmdbSeason{
		{Number: 1, Name: "Dahmer - Monster: The Jeffrey Dahmer Story", AirDate: "2022-09-21"},
		{Number: 2, Name: "Monsters: The Lyle and Erik Menendez Story", AirDate: "2024-09-19"},
		{Number: 3, Name: "Monster: The Ed Gein Story", AirDate: "2025-10-03"},
	}
	demonSlayer := []repository.TmdbSeason{
		{Number: 0, Name: "Specials"},
		{Number: 1, Name: "Tanjiro Kamado, Unwavering Resolve Arc", AirDate: "2019-04-06"},
		{Number: 4, Name: "Hashira Training Arc", AirDate: "2024-05-12"},
	}
	raw := []repository.TmdbSeason{
		{Number: 33, Name: "Season 33", AirDate: "2025-01-06"},
		{Number: 34, Name: "Season 34", AirDate: "2026-01-05"},
	}
	weird := []repository.TmdbSeason{{Number: 1, Name: "Season 1", AirDate: "2019-01-01"}, {Number: 2, Name: "Season 2", AirDate: "2021-01-01"}}
	single := []repository.TmdbSeason{{Number: 0, Name: "Specials"}, {Number: 1, Name: "Season 1", AirDate: "2018-01-01"}}

	tests := []struct {
		show, season, firstWeek string
		tmdb                    []repository.TmdbSeason
		want                    int64 // 0 = nil
		source                  string
	}{
		{"Wednesday", "Wednesday: Season 2", "", nil, 2, repository.SeasonSourceParsed},
		{"Monster: The Lizzie Borden Story", "Monster: The Lizzie Borden Story: Season 1", "", nil, 1, repository.SeasonSourceParsed},
		{"East of Eden", "East of Eden: Limited Series", "", nil, 1, repository.SeasonSourceParsed},
		{"Love Is Blind", "Love Is Blind: S10: Ohio", "", nil, 10, repository.SeasonSourceParsed},
		{"Homicide", "Homicide: New York: Season 2", "", nil, 2, repository.SeasonSourceParsed},
		{"The Uncanny Counter", "The Uncanny Counter: Season 2: Counter Punch", "", nil, 2, repository.SeasonSourceParsed},
		{"Idol I", "Idol I: 시즌 1", "", nil, 1, repository.SeasonSourceParsed},
		// Named seasons: TMDB's season names.
		{"Demon Slayer: Kimetsu no Yaiba", "Demon Slayer: Kimetsu no Yaiba: Hashira Training Arc", "2024-05-19", demonSlayer, 4, repository.SeasonSourceTmdbName},
		{"Monster", "Monsters: The Lyle and Erik Menendez Story", "2024-09-22", monster, 2, repository.SeasonSourceTmdbName},
		// Numbered parts and bare numbers, checked against TMDB when known.
		{"Love, Death & Robots", "Love, Death & Robots: Volume 4", "", nil, 4, repository.SeasonSourceNumber},
		{"Stranger Things", "Stranger Things 5", "", nil, 5, repository.SeasonSourceNumber},
		{"The Snow Girl", "The Snow Girl: 2. The Soul Game", "", nil, 2, repository.SeasonSourceNumber},
		{"Our Planet", "Our Planet: II", "", nil, 2, repository.SeasonSourceNumber},
		{"STEEL BALL RUN JoJo's Bizarre Adventure", "STEEL BALL RUN JoJo's Bizarre Adventure: 1st STAGE", "", nil, 1, repository.SeasonSourceNumber},
		// Dated episodes: the TMDB season aired by that date.
		{"Raw", "Raw: September 28, 2026", "2026-10-04", raw, 34, repository.SeasonSourceTmdbAir},
		// Unrecognised title shortly after a season aired.
		{"Some Show", "Some Show: The Reckoning", "2021-01-24", weird, 2, repository.SeasonSourceTmdbAir},
		// ... but not years later.
		{"Some Show", "Some Show: The Reckoning", "2026-01-25", weird, 0, ""},
		// Volume 9 does not exist on TMDB: fall through (nothing else fits).
		{"Some Show", "Some Show: Volume 9", "2026-01-25", weird, 0, ""},
		{"Only Once", "Only Once: The Story", "2026-01-25", single, 1, repository.SeasonSourceTmdbSingle},
		{"Unknown", "Unknown: The Story", "2026-01-25", nil, 0, ""},
	}
	for _, tt := range tests {
		got, source := ResolveNetflixSeason(tt.show, tt.season, tt.firstWeek, tt.tmdb)
		var n int64
		if got != nil {
			n = *got
		}
		if n != tt.want || source != tt.source {
			t.Errorf("ResolveNetflixSeason(%q) = %d (%q), want %d (%q)", tt.season, n, source, tt.want, tt.source)
		}
	}
}

func TestSeasonSuffix(t *testing.T) {
	for _, tt := range [][3]string{
		{"Wednesday", "Wednesday: Season 2", "Season 2"},
		{"Monster", "Monsters: The Lyle and Erik Menendez Story", "Monsters: The Lyle and Erik Menendez Story"},
		{"Stranger Things", "Stranger Things 5", "5"},
		{"Raw", "Raw", "Raw"},
	} {
		if got := seasonSuffix(tt[0], tt[1]); got != tt[2] {
			t.Errorf("seasonSuffix(%q, %q) = %q, want %q", tt[0], tt[1], got, tt[2])
		}
	}
}

func TestNetflixWeekOf(t *testing.T) {
	for in, want := range map[string]string{"2026-09-28": "2026-10-04", "2026-10-04": "2026-10-04", "2026-10-05": "2026-10-11"} {
		if got := netflixWeekOf(in); got != want {
			t.Errorf("netflixWeekOf(%s) = %s, want %s", in, got, want)
		}
	}
}

// End to end: Netflix season titles get numbers, FlixPatrol TV rankings get
// the season Netflix reported that week (Netflix charts) or the latest aired
// one (other services), manual fixes stick and flow through.
func TestSeasonMappingFlow(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	flix := repository.NewFlixPatrolRepository(db)
	imports := repository.NewImportsRepository(db)
	meta := repository.NewMetadataRepository(db)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	seasons := NewSeasonMapper(log, imports)
	n := NewNetflixImporter(log, nil, imports, flix, repository.NewTaskScheduleRepository(db), nil, nil, seasons, []string{"US"})

	seed(t, flix, []seedTitle{{Slug: "the-gentlemen", Name: "The Gentlemen", Kind: "tv_show", TmdbID: "236235"}},
		[]seedRanking{
			{"the-gentlemen", "2026-09-30", "US", "netflix", "tv_shows", 3}, // week ending 2026-10-04
			{"the-gentlemen", "2026-09-30", "GB", "netflix", "tv_shows", 5}, // GB week not imported: global
			{"the-gentlemen", "2026-09-30", "US", "hbo-max", "tv_shows", 2}, // not Netflix: latest aired
			{"the-gentlemen", "2024-03-10", "US", "netflix", "tv_shows", 1}, // before season 2 aired
		})
	if err := meta.SaveDetails(ctx, &client.TMDBFullDetails{Kind: "tv", ID: "236235", Title: "The Gentlemen",
		Seasons: []client.TMDBSeason{{Number: 1, Name: "Season 1", AirDate: "2024-03-07"}, {Number: 2, Name: "Season 2", AirDate: "2026-09-25"}}}); err != nil {
		t.Fatal(err)
	}

	global := "week\tcategory\tweekly_rank\tshow_title\tseason_title\tweekly_hours_viewed\truntime\tweekly_views\tcumulative_weeks_in_top_10\n" +
		"2026-10-04\tTV (English)\t6\tThe Gentlemen\tThe Gentlemen: Season 2\t1000\t\t500\t1\n" +
		"2026-10-04\tTV (English)\t9\tThe Gentlemen\tThe Gentlemen: Season 1\t800\t\t300\t2\n" +
		"2026-10-04\tTV (English)\t10\tMystery\tMystery: The Reckoning\t10\t\t5\t1\n"
	if _, _, err := n.importGlobal(ctx, strings.NewReader(global)); err != nil {
		t.Fatal(err)
	}
	// US week: only season 1 made the US list.
	countries := "country_name\tcountry_iso2\tweek\tcategory\tweekly_rank\tshow_title\tseason_title\tcumulative_weeks_in_top_10\n" +
		"United States\tUS\t2026-10-04\tTV\t4\tThe Gentlemen\tThe Gentlemen: Season 1\t2\n"
	if _, _, err := n.importCountries(ctx, strings.NewReader(countries), nil); err != nil {
		t.Fatal(err)
	}
	rep := &ImportReport{}
	if err := n.matchTitles(ctx, nil, netflixMatchBatch, rep, func(string, string, ...any) {}); err != nil {
		t.Fatal(err)
	}
	sr, err := seasons.Refresh(ctx, time.Time{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if sr.Seasons != 3 || sr.SeasonsUnmapped != 1 {
		t.Fatalf("season report = %+v", sr)
	}

	rankSeason := func(date, country, provider string) (int64, string) {
		var n *int64
		var src *string
		if err := db.QueryRow(`SELECT season_number, season_source FROM rankings WHERE ranked_on = ? AND country = ? AND streaming_provider = ?`,
			date, country, provider).Scan(&n, &src); err != nil {
			t.Fatal(err)
		}
		if n == nil {
			return 0, ""
		}
		return *n, *src
	}
	for _, tt := range []struct {
		date, country, provider string
		want                    int64
		source                  string
	}{
		{"2026-09-30", "US", "netflix", 1, repository.SeasonSourceNetflix}, // the US list had season 1
		{"2026-09-30", "GB", "netflix", 2, repository.SeasonSourceNetflix}, // global: season 2 ranked higher
		{"2026-09-30", "US", "hbo-max", 2, repository.SeasonSourceTmdbAir},
		{"2024-03-10", "US", "netflix", 1, repository.SeasonSourceTmdbAir},
	} {
		if n, src := rankSeason(tt.date, tt.country, tt.provider); n != tt.want || src != tt.source {
			t.Errorf("%s %s %s: season %d (%s), want %d (%s)", tt.date, tt.country, tt.provider, n, src, tt.want, tt.source)
		}
	}

	// Public reads carry the season.
	analytics := repository.NewAnalyticsRepository(db)
	week, err := analytics.NetflixGlobalWeek(ctx, "2026-10-04", "")
	if err != nil || len(week) != 3 || week[0].SeasonNumber == nil || *week[0].SeasonNumber != 2 || week[0].NetflixTitleID == 0 {
		t.Fatalf("global week = %+v, %v", week, err)
	}
	overview := NewTitleOverviewService(analytics, flix, meta, nil)
	nf, err := overview.Netflix(ctx, TmdbRef{Kind: "tv", ID: "236235"})
	if err != nil || len(nf.Seasons) != 2 || *nf.Seasons[0].SeasonNumber != 1 || nf.Seasons[0].Countries != 1 || *nf.Seasons[1].BestRank != 6 {
		t.Fatalf("title netflix seasons = %+v, %v", nf, err)
	}

	// Admin: the unmapped "Mystery" season is fixed by hand.
	admin := NewNetflixAdminService(log, imports, meta, nil, seasons, nil)
	page, err := admin.List(ctx, NetflixTitlesQuery{Missing: "season"}, 50, 0)
	if err != nil || page.Total != 1 || page.Items[0].ShowTitle != "Mystery" || page.Items[0].SeasonsUnmapped != 1 {
		t.Fatalf("missing season page = %+v, %v", page, err)
	}
	detail, err := admin.Get(ctx, page.Items[0].ID)
	if err != nil || len(detail.Seasons) != 1 || detail.Seasons[0].SeasonNumber != nil {
		t.Fatalf("detail = %+v, %v", detail, err)
	}
	if _, err := admin.PatchSeason(ctx, detail.Seasons[0].ID, PatchNetflixSeasonInput{}); !isValidation(err) {
		t.Fatalf("empty season patch: err = %v, want validation", err)
	}
	three := int64(3)
	detail, err = admin.PatchSeason(ctx, detail.Seasons[0].ID, PatchNetflixSeasonInput{SeasonNumber: &three})
	if err != nil || *detail.Seasons[0].SeasonNumber != 3 || *detail.Seasons[0].SeasonSource != repository.SeasonSourceManual {
		t.Fatalf("manual season = %+v, %v", detail.Seasons, err)
	}
	if _, err := seasons.Refresh(ctx, time.Time{}, nil); err != nil {
		t.Fatal(err)
	}
	if detail, _ = admin.Get(ctx, detail.ID); *detail.Seasons[0].SeasonNumber != 3 {
		t.Fatal("automatic mapping overwrote a manual season")
	}
	detail, err = admin.PatchSeason(ctx, detail.Seasons[0].ID, PatchNetflixSeasonInput{Auto: true})
	if err != nil || detail.Seasons[0].SeasonNumber != nil || detail.Seasons[0].SeasonSource != nil {
		t.Fatalf("reset season = %+v, %v", detail.Seasons, err)
	}
	if _, err := admin.PatchSeason(ctx, 9999, PatchNetflixSeasonInput{Auto: true}); !errors.Is(err, ErrNetflixSeasonNotFound) {
		t.Fatalf("unknown season: %v", err)
	}
}

func TestNetflixAdminMapping(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	flix := repository.NewFlixPatrolRepository(db)
	imports := repository.NewImportsRepository(db)
	meta := repository.NewMetadataRepository(db)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	seasons := NewSeasonMapper(log, imports)
	mapping := NewTitleMappingService(log, repository.NewMappingRepository(db), meta, nil, seasons)
	n := NewNetflixImporter(log, nil, imports, flix, repository.NewTaskScheduleRepository(db), nil, nil, seasons, nil).WithMapping(mapping)
	admin := NewNetflixAdminService(log, imports, meta, nil, seasons, nil).WithMapping(mapping)

	seed(t, flix, []seedTitle{{Slug: "unabomber", Name: "UNABOMBER", Kind: "movie", TmdbID: "999"}},
		[]seedRanking{{"unabomber", "2026-10-01", "US", "netflix", "movies", 1}})
	global := "week\tcategory\tweekly_rank\tshow_title\tseason_title\tweekly_hours_viewed\truntime\tweekly_views\tcumulative_weeks_in_top_10\n" +
		"2026-10-04\tFilms (English)\t1\tUNABOMBER\tN/A\t41500000\t1.6667\t24900000\t2\n" +
		"2026-09-27\tFilms (English)\t3\tSomething Else\tN/A\t100\t1\t50\t1\n"
	if _, _, err := n.importGlobal(ctx, strings.NewReader(global)); err != nil {
		t.Fatal(err)
	}
	if err := n.matchTitles(ctx, nil, netflixMatchBatch, &ImportReport{}, func(string, string, ...any) {}); err != nil {
		t.Fatal(err)
	}

	page, err := admin.List(ctx, NetflixTitlesQuery{}, 50, 0)
	if err != nil || page.Total != 2 || page.Items[0].ShowTitle != "UNABOMBER" {
		t.Fatalf("page = %+v, %v (newest week first)", page, err)
	}
	una := page.Items[0]
	if una.FlixPatrol == nil || una.FlixPatrol.Slug != "unabomber" || *una.BestGlobalRank != 1 || una.WeeksGlobal != 1 {
		t.Fatalf("unabomber = %+v", una)
	}
	for _, bad := range []NetflixTitlesQuery{{Kind: "x"}, {MatchStatus: "x"}, {Missing: "rt"}, {Sort: "x"}} {
		if _, err := admin.List(ctx, bad, 50, 0); !isValidation(err) {
			t.Fatalf("List(%+v) err = %v, want validation", bad, err)
		}
	}
	if p, _ := admin.List(ctx, NetflixTitlesQuery{Search: "999"}, 50, 0); p.Total != 1 {
		t.Fatalf("search by tmdb id: %d", p.Total)
	}
	other := page.Items[1]
	if p, _ := admin.List(ctx, NetflixTitlesQuery{Missing: "tmdb"}, 50, 0); p.Total != 1 || p.Items[0].ID != other.ID {
		t.Fatalf("missing tmdb = %+v", p)
	}

	// Manual fix: validated, locked, and survives matching.
	bad := "abc"
	if _, err := admin.Patch(ctx, other.ID, PatchNetflixTitleInput{TmdbID: &bad}); !isValidation(err) {
		t.Fatalf("bad tmdb id: %v", err)
	}
	if _, err := admin.Patch(ctx, other.ID, PatchNetflixTitleInput{ImdbID: &bad}); !isValidation(err) {
		t.Fatalf("bad imdb id: %v", err)
	}
	tmdb, imdb := "4242", "tt4242424"
	d, err := admin.Patch(ctx, other.ID, PatchNetflixTitleInput{TmdbID: &tmdb, ImdbID: &imdb})
	if err != nil || d.MatchStatus != repository.MatchStatusManual || *d.TmdbID != "4242" || *d.ImdbID != "tt4242424" {
		t.Fatalf("patched = %+v, %v", d, err)
	}
	if _, err := n.RunMatch(ctx, []int64{other.ID}, nil); err != nil {
		t.Fatal(err)
	}
	if d, _ = admin.Get(ctx, other.ID); d.MatchStatus != repository.MatchStatusManual || *d.TmdbID != "4242" {
		t.Fatalf("manual title changed by matching: %+v", d)
	}
	// Rematch releases the lock; clear forgets the IDs.
	if d, err = admin.Rematch(ctx, other.ID, false); err != nil || d.MatchStatus != repository.MatchStatusMatched || d.TmdbID == nil {
		t.Fatalf("rematch = %+v, %v", d, err)
	}
	if d, err = admin.Rematch(ctx, other.ID, true); err != nil || d.MatchStatus != repository.MatchStatusPending || d.TmdbID != nil {
		t.Fatalf("clear = %+v, %v", d, err)
	}
	if _, err := admin.Get(ctx, 9999); !errors.Is(err, ErrNetflixTitleNotFound) {
		t.Fatalf("unknown title: %v", err)
	}

	// Fixing the FlixPatrol title by hand moves the Netflix title linked to it.
	titles := NewTitleAdminService(flix).WithMapping(mapping)
	fp, err := flix.GetTitleBySlug(ctx, "unabomber")
	if err != nil {
		t.Fatal(err)
	}
	fixed := "1000"
	if _, err := titles.PatchTitleIDs(ctx, fp.ID, PatchTitleIDsInput{TmdbID: &fixed}); err != nil {
		t.Fatal(err)
	}
	if d, _ = admin.Get(ctx, una.ID); *d.TmdbID != "1000" || d.MatchStatus != repository.MatchStatusMatched {
		t.Fatalf("netflix title after flixpatrol fix = %+v", d)
	}
}

func isValidation(err error) bool {
	var v *ValidationError
	return errors.As(err, &v)
}
