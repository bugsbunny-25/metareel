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

func TestComputeTitleLinks(t *testing.T) {
	titles := []repository.LinkingNetflixTitle{
		{ID: 1, ShowTitle: "Wednesday", Kind: "tv_show"},
		{ID: 2, ShowTitle: "Taylor Tomlinson: Have It All", Kind: "tv_show"},
		{ID: 3, ShowTitle: "Man on Fire", Kind: "movie"}, // FlixPatrol's is in the TV list
		{ID: 4, ShowTitle: "The Gambler", Kind: "movie"}, // charted months apart
		{ID: 5, ShowTitle: "Spider-Man: No Way Home", Kind: "movie"},
	}
	weeks := []repository.NetflixLinkWeek{
		{NetflixTitleID: 1, Week: "2026-10-04"}, {NetflixTitleID: 2, Week: "2026-10-11"},
		{NetflixTitleID: 3, Week: "2026-10-04"}, {NetflixTitleID: 4, Week: "2026-01-04"},
		{NetflixTitleID: 5, Week: "2026-10-04"},
	}
	fp := []repository.FlixPatrolNetflixWeek{
		{TitleID: 10, Name: "Wednesday: Season 2", Category: "tv_shows", Week: "2026-10-04"},
		{TitleID: 11, Name: "Have It All", FpName: "Taylor Tomlinson: Have It All", Category: "tv_shows", Week: "2026-10-04"}, // adjacent week
		{TitleID: 12, Name: "Man on Fire", Category: "tv_shows", Week: "2026-10-04"},
		{TitleID: 13, Name: "The Gambler", Category: "movies", Week: "2026-03-01"},
		{TitleID: 14, Name: "Spider Man - No Way Home", Category: "movies", Week: "2026-10-04"},
	}
	got := ComputeTitleLinks(titles, weeks, fp)
	want := []repository.TitleLink{{NetflixTitleID: 1, TitleID: 10}, {NetflixTitleID: 2, TitleID: 11}, {NetflixTitleID: 5, TitleID: 14}}
	if len(got) != len(want) {
		t.Fatalf("links = %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("links = %+v, want %+v", got, want)
		}
	}
}

func TestGroupAnchor(t *testing.T) {
	movie := func(id string) repository.TmdbRef { return repository.TmdbRef{Kind: "movie", ID: id} }
	tv := func(id string) repository.TmdbRef { return repository.TmdbRef{Kind: "tv", ID: id} }
	for name, tt := range map[string]struct {
		members  []mappingMember
		want     repository.TmdbRef
		ok, conf bool
	}{
		"manual netflix beats flixpatrol match": {
			members: []mappingMember{{id: 1, ref: tv("1"), matched: true}, {netflix: true, id: 2, ref: movie("9"), manual: true}},
			want:    movie("9"), ok: true,
		},
		"flixpatrol match beats netflix match": {
			members: []mappingMember{{id: 1, ref: tv("1"), matched: true}, {netflix: true, id: 2, ref: tv("2"), matched: true}},
			want:    tv("1"), ok: true,
		},
		"netflix match fills an unmatched flixpatrol title": {
			members: []mappingMember{{id: 1}, {netflix: true, id: 2, ref: movie("5"), matched: true}},
			want:    movie("5"), ok: true,
		},
		"manual titles disagree": {
			members: []mappingMember{{id: 1, ref: tv("1"), manual: true}, {netflix: true, id: 2, ref: movie("1"), manual: true}},
			conf:    true,
		},
		"manual clear has no opinion": {
			members: []mappingMember{{id: 1, manual: true}, {netflix: true, id: 2, ref: movie("5"), matched: true}},
			want:    movie("5"), ok: true,
		},
		"nothing mapped": {members: []mappingMember{{id: 1}, {netflix: true, id: 2}}},
	} {
		a, ok, conf := groupAnchor(tt.members)
		if ok != tt.ok || conf != tt.conf || (ok && a.ref != tt.want) {
			t.Errorf("%s: anchor %+v ok=%v conflict=%v", name, a.ref, ok, conf)
		}
	}
}

func TestPickWikidataNetflix(t *testing.T) {
	hit := func(kind, id string, year int) client.WikidataNetflixHit {
		return client.WikidataNetflixHit{TmdbKind: kind, TmdbID: id, Year: year}
	}
	for name, tt := range map[string]struct {
		hits     []client.WikidataNetflixHit
		list     string
		year     int
		want     string
		accepted bool
	}{
		"same kind":                   {[]client.WikidataNetflixHit{hit("tv", "94605", 2021)}, "tv_show", 2024, "94605", true},
		"special in the TV list":      {[]client.WikidataNetflixHit{hit("movie", "1227770", 2024)}, "tv_show", 2024, "1227770", true},
		"older film of the same name": {[]client.WikidataNetflixHit{hit("movie", "393519", 2016)}, "tv_show", 2026, "", false},
		"several, one recent":         {[]client.WikidataNetflixHit{hit("movie", "1", 2004), hit("movie", "2", 2026)}, "movie", 2026, "2", true},
		"several, none recent":        {[]client.WikidataNetflixHit{hit("movie", "1", 2004), hit("movie", "2", 2010)}, "movie", 2026, "", false},
		"no year for other kind":      {[]client.WikidataNetflixHit{hit("movie", "1", 2024)}, "tv_show", 0, "", false},
	} {
		h, ok := pickWikidataNetflix(tt.hits, tt.list, tt.year)
		if ok != tt.accepted || h.TmdbID != tt.want {
			t.Errorf("%s: got %+v ok=%v", name, h, ok)
		}
	}
}

// End to end: Netflix and FlixPatrol titles are linked, mappings flow both
// ways (TMDB kind included), conflicts are reported and a remap fixes every
// title at once.
func TestTitleMappingFlow(t *testing.T) {
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
	titles := NewTitleAdminService(flix).WithMapping(mapping)

	seed(t, flix, []seedTitle{
		{Slug: "taylor-tomlinson-have-it-all", Name: "Taylor Tomlinson: Have It All", Kind: "tv_show"}, // not matched
		{Slug: "wednesday", Name: "Wednesday: Season 2", Kind: "tv_show", TmdbID: "119051", ImdbID: "tt13443470", RtURL: "tv/wednesday"},
		{Slug: "the-gambler", Name: "The Gambler", Kind: "movie", TmdbID: "111"},
	}, []seedRanking{
		{"taylor-tomlinson-have-it-all", "2026-10-01", "US", "netflix", "tv_shows", 1},
		{"wednesday", "2026-10-01", "US", "netflix", "tv_shows", 2},
		{"the-gambler", "2026-10-01", "US", "netflix", "movies", 1},
	})
	global := "week\tcategory\tweekly_rank\tshow_title\tseason_title\tweekly_hours_viewed\truntime\tweekly_views\tcumulative_weeks_in_top_10\n" +
		"2026-10-04\tTV (English)\t1\tTaylor Tomlinson: Have It All\tTaylor Tomlinson: Have It All: Limited Series\t10\t1\t10\t1\n" +
		"2026-10-04\tTV (English)\t2\tWednesday\tWednesday: Season 2\t10\t1\t10\t1\n" +
		"2026-10-04\tFilms (English)\t1\tThe Gambler\tN/A\t10\t1\t10\t1\n"
	if _, _, err := n.importGlobal(ctx, strings.NewReader(global)); err != nil {
		t.Fatal(err)
	}
	rep := &ImportReport{}
	if err := n.matchTitles(ctx, nil, netflixMatchBatch, rep, func(string, string, ...any) {}); err != nil {
		t.Fatal(err)
	}
	if rep.Mapping == nil || rep.Mapping.Links != 3 || rep.Mapping.NetflixUpdated != 2 {
		t.Fatalf("mapping report = %+v", rep.Mapping)
	}
	if _, err := seasons.Refresh(ctx, time.Time{}, nil); err != nil {
		t.Fatal(err)
	}
	netflixID := func(name string) int64 {
		p, err := admin.List(ctx, NetflixTitlesQuery{Search: name}, 10, 0)
		if err != nil || p.Total != 1 {
			t.Fatalf("netflix title %q: %+v %v", name, p, err)
		}
		return p.Items[0].ID
	}
	wed, err := admin.Get(ctx, netflixID("Wednesday"))
	if err != nil || *wed.TmdbKind != "tv" || *wed.TmdbID != "119051" || *wed.MatchSource != MatchSourceFlixPatrolTitle || len(wed.Links) != 1 {
		t.Fatalf("wednesday = %+v %v", wed, err)
	}

	// A special in Netflix's TV list fixed by hand as a TMDB movie: the
	// linked FlixPatrol title follows (kind included), and the "Limited
	// Series" season title no longer gets a season number.
	taylorID := netflixID("Taylor Tomlinson")
	if d, _ := admin.Get(ctx, taylorID); d.Seasons[0].SeasonNumber == nil {
		t.Fatalf("limited series before the fix = %+v", d.Seasons)
	}
	badKind := "show"
	if _, err := admin.Patch(ctx, taylorID, PatchNetflixTitleInput{TmdbKind: &badKind}); !isValidation(err) {
		t.Fatalf("bad tmdb_kind: %v", err)
	}
	movie, special := "movie", "1227770"
	d, err := admin.Patch(ctx, taylorID, PatchNetflixTitleInput{TmdbKind: &movie, TmdbID: &special})
	if err != nil || *d.TmdbKind != "movie" || d.Kind != "tv_show" || d.Seasons[0].SeasonNumber != nil || d.SeasonsUnmapped != 0 {
		t.Fatalf("patched special = %+v %v", d, err)
	}
	fp, _ := flix.GetTitleBySlug(ctx, "taylor-tomlinson-have-it-all")
	if fp.Kind != "movie" || fp.TmdbID.String != "1227770" || fp.MatchStatus != repository.MatchStatusMatched || fp.MatchSource.String != MatchSourceNetflixTitle {
		t.Fatalf("flixpatrol title after netflix fix = %+v", fp)
	}
	if p, _ := admin.List(ctx, NetflixTitlesQuery{Flag: "cross_kind"}, 10, 0); p.Total != 1 || p.Items[0].ID != taylorID {
		t.Fatalf("cross_kind = %+v", p)
	}
	if p, _ := admin.List(ctx, NetflixTitlesQuery{Missing: "season"}, 10, 0); p.Total != 0 {
		t.Fatalf("a movie counts as missing seasons: %+v", p)
	}

	// The TMDB-keyed mapping page.
	page, err := mapping.List(ctx, MappingQuery{Flag: "cross_kind"}, 10, 0)
	if err != nil || page.Total != 1 || page.Items[0].TmdbID != "1227770" || len(page.Items[0].FlixPatrol) != 1 || len(page.Items[0].Netflix) != 1 {
		t.Fatalf("cross-kind mappings = %+v %v", page, err)
	}
	if _, err := mapping.List(ctx, MappingQuery{Flag: "x"}, 10, 0); !isValidation(err) {
		t.Fatalf("bad flag: %v", err)
	}
	if p, _ := mapping.List(ctx, MappingQuery{Flag: "both"}, 10, 0); p.Total != 3 {
		t.Fatalf("both sources = %d", p.Total)
	}

	// Every ID of a TMDB title in one place, Netflix's via Wikidata.
	if err := meta.SaveWikidata(ctx, repository.TmdbRef{Kind: "tv", ID: "119051"}, client.WikidataIDs{WikidataID: "Q105553568", NetflixID: "81231974"}); err != nil {
		t.Fatal(err)
	}
	if _, err := mapping.Sync(ctx, false, nil); err != nil {
		t.Fatal(err)
	}
	ids, err := mapping.ExternalIDs(ctx, repository.TmdbRef{Kind: "tv", ID: "119051"})
	if err != nil || *ids.NetflixID != "81231974" || *ids.IMDbID != "tt13443470" || *ids.RottenTomatoesID != "tv/wednesday" || *ids.WikidataID != "Q105553568" ||
		strings.Join(ids.FlixPatrolSlugs, ",") != "wednesday" || strings.Join(ids.NetflixTop10, ",") != "Wednesday" {
		t.Fatalf("ids = %+v %v", ids, err)
	}
	overview := NewTitleOverviewService(repository.NewAnalyticsRepository(db), flix, meta, nil).WithMapping(mapping)
	if res, err := overview.Lookup(ctx, "", "", "", "81231974"); err != nil || len(res.Refs) != 1 || res.Refs[0].TmdbID != "119051" || len(res.Titles) != 1 {
		t.Fatalf("lookup by netflix id = %+v %v", res, err)
	}
	if _, err := overview.Lookup(ctx, "", "", "", "abc"); !isValidation(err) {
		t.Fatalf("bad netflix id: %v", err)
	}

	// Two titles fixed by hand that disagree are a conflict, left alone.
	gamblerNetflix := netflixID("The Gambler")
	fg, _ := flix.GetTitleBySlug(ctx, "the-gambler")
	a, b := "222", "333"
	if _, err := titles.PatchTitleIDs(ctx, fg.ID, PatchTitleIDsInput{TmdbID: &a}); err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Patch(ctx, gamblerNetflix, PatchNetflixTitleInput{TmdbID: &b}); err != nil {
		t.Fatal(err)
	}
	sr, err := mapping.Sync(ctx, true, nil)
	if err != nil || sr.Conflicts != 2 { // each of the two sees the other
		t.Fatalf("sync = %+v %v", sr, err)
	}
	if p, _ := mapping.List(ctx, MappingQuery{Flag: "conflict"}, 10, 0); p.Total != 2 {
		t.Fatalf("conflicts = %+v", p)
	}
	// One remap fixes it.
	if _, err := mapping.Remap(ctx, repository.TmdbRef{Kind: "movie", ID: "333"}, RemapInput{TmdbKind: "movie", TmdbID: "333"}); !isValidation(err) {
		t.Fatalf("remap onto itself: %v", err)
	}
	if _, err := mapping.Remap(ctx, repository.TmdbRef{Kind: "movie", ID: "404"}, RemapInput{TmdbKind: "movie", TmdbID: "1"}); !errors.Is(err, ErrTitleNotFound) {
		t.Fatalf("remap unknown: %v", err)
	}
	e, err := mapping.Remap(ctx, repository.TmdbRef{Kind: "movie", ID: "333"}, RemapInput{TmdbKind: "movie", TmdbID: "222"})
	if err != nil || len(e.FlixPatrol) != 1 || len(e.Netflix) != 1 || e.Conflicts != 0 || e.Netflix[0].MatchStatus != repository.MatchStatusManual {
		t.Fatalf("remapped = %+v %v", e, err)
	}
	if sr, _ = mapping.Sync(ctx, false, nil); sr.Conflicts != 0 {
		t.Fatalf("conflicts after remap = %d", sr.Conflicts)
	}

	// Clearing a wrong FlixPatrol match also clears what the linked Netflix
	// title took from it, so the next sync does not hand it back.
	fw, _ := flix.GetTitleBySlug(ctx, "wednesday")
	if _, err := titles.Rematch(ctx, fw.ID, true); err != nil {
		t.Fatal(err)
	}
	if _, err := mapping.Sync(ctx, true, nil); err != nil {
		t.Fatal(err)
	}
	fw, _ = flix.GetTitleBySlug(ctx, "wednesday")
	if wed, _ := admin.Get(ctx, netflixID("Wednesday")); wed.TmdbID != nil || fw.TmdbID.Valid {
		t.Fatalf("after clearing: netflix %v, flixpatrol %v", wed.TmdbID, fw.TmdbID)
	}
}

type fakeNetflixSearch struct {
	items map[string][]client.NetflixSearchItem
	shows map[string]string
	terms []string
}

func (f *fakeNetflixSearch) Search(_ context.Context, term string) ([]client.NetflixSearchItem, error) {
	f.terms = append(f.terms, term)
	return f.items[term], nil
}

func (f *fakeNetflixSearch) ShowID(_ context.Context, id string) (string, error) {
	if s, ok := f.shows[id]; ok {
		return s, nil
	}
	return id, nil
}

// Netflix IDs from Tudum's search map titles exactly: TV by a season
// title's ID (redirected to the show), films by name + list + week; a
// same-name result from other weeks is not taken, and an exact match beats
// FlixPatrol's name match in a linked group.
func TestNetflixIDMatching(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	flix := repository.NewFlixPatrolRepository(db)
	imports := repository.NewImportsRepository(db)
	meta := repository.NewMetadataRepository(db)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	seasons := NewSeasonMapper(log, imports)
	metadata := NewTitleMetadataService(log, meta, nil, nil)
	mapping := NewTitleMappingService(log, repository.NewMappingRepository(db), meta, nil, seasons)
	search := &fakeNetflixSearch{
		items: map[string][]client.NetflixSearchItem{
			"Wednesday": {{NetflixID: "81435630", Title: "Wednesday: Season 2", Category: "TV", GlobalWeeks: []string{"2026-10-04"}}},
			"Man on Fire": {
				{NetflixID: "70011111", Title: "Man on Fire", Category: "FILMS", GlobalWeeks: []string{"2021-07-04"}}, // the 2004 film, other weeks
				{NetflixID: "81999999", Title: "Man on Fire", Category: "TV", GlobalWeeks: []string{"2026-10-04"}},
			},
		},
		shows: map[string]string{"81435630": "81231974"},
	}
	n := NewNetflixImporter(log, nil, imports, flix, repository.NewTaskScheduleRepository(db), nil, metadata, seasons, nil).
		WithMapping(mapping).WithNetflixSearch(search)
	admin := NewNetflixAdminService(log, imports, meta, nil, seasons, nil).WithMapping(mapping)

	// TMDB already knows Wednesday's Netflix ID (from Wikidata); FlixPatrol
	// matched its title to the wrong show by name.
	if err := meta.SaveWikidata(ctx, repository.TmdbRef{Kind: "tv", ID: "119051"}, client.WikidataIDs{NetflixID: "81231974"}); err != nil {
		t.Fatal(err)
	}
	seed(t, flix, []seedTitle{{Slug: "wednesday", Name: "Wednesday", Kind: "tv_show", TmdbID: "555"}},
		[]seedRanking{{"wednesday", "2026-10-01", "US", "netflix", "tv_shows", 1}})
	global := "week\tcategory\tweekly_rank\tshow_title\tseason_title\tweekly_hours_viewed\truntime\tweekly_views\tcumulative_weeks_in_top_10\n" +
		"2026-10-04\tTV (English)\t1\tWednesday\tWednesday: Season 2\t10\t1\t10\t1\n" +
		"2026-10-04\tFilms (English)\t1\tMan on Fire\tN/A\t10\t1\t10\t1\n"
	if _, _, err := n.importGlobal(ctx, strings.NewReader(global)); err != nil {
		t.Fatal(err)
	}
	// Straight after an import: no season rows yet.
	if _, err := n.RunMatch(ctx, nil, nil); err != nil {
		t.Fatal(err)
	}

	page, _ := admin.List(ctx, NetflixTitlesQuery{Search: "Wednesday"}, 10, 0)
	wed, err := admin.Get(ctx, page.Items[0].ID)
	if err != nil || derefStr(wed.NetflixID) != "81231974" || derefStr(wed.TmdbID) != "119051" || derefStr(wed.MatchSource) != MatchSourceNetflixID {
		t.Fatalf("wednesday = %+v %v", wed.NetflixTitleResponse, err)
	}
	if len(wed.Seasons) != 1 || derefStr(wed.Seasons[0].NetflixID) != "81435630" {
		t.Fatalf("season netflix id = %+v", wed.Seasons)
	}
	// The exact match wins the linked group: FlixPatrol's title follows.
	fp, _ := flix.GetTitleBySlug(ctx, "wednesday")
	if fp.TmdbID.String != "119051" || fp.MatchSource.String != MatchSourceNetflixTitle {
		t.Fatalf("flixpatrol title = %+v", fp)
	}
	// A film whose same-name search result charted in other weeks (and
	// the TV one is the other list) gets no Netflix ID.
	page, _ = admin.List(ctx, NetflixTitlesQuery{Search: "Man on Fire"}, 10, 0)
	if mf := page.Items[0]; mf.NetflixID != nil {
		t.Fatalf("man on fire netflix id = %v", *mf.NetflixID)
	}
	// Looked-up titles are not searched again within the window.
	before := len(search.terms)
	if _, err := n.RunMatch(ctx, nil, nil); err != nil {
		t.Fatal(err)
	}
	if len(search.terms) != before {
		t.Fatalf("searched again: %v", search.terms[before:])
	}
	if ids, _ := mapping.ExternalIDs(ctx, repository.TmdbRef{Kind: "tv", ID: "119051"}); derefStr(ids.NetflixID) != "81231974" {
		t.Fatalf("tmdb title netflix id = %+v", ids)
	}
}

// Two FlixPatrol titles of one name (a show and its revival) linked to the
// same Netflix title: the Netflix title is a conflict until one is fixed by
// hand, and that fix never overwrites the other FlixPatrol title's own match.
func TestMappingDoesNotSpreadThroughLinks(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	flix := repository.NewFlixPatrolRepository(db)
	imports := repository.NewImportsRepository(db)
	meta := repository.NewMetadataRepository(db)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	seasons := NewSeasonMapper(log, imports)
	mapping := NewTitleMappingService(log, repository.NewMappingRepository(db), meta, nil, seasons)
	n := NewNetflixImporter(log, nil, imports, flix, repository.NewTaskScheduleRepository(db), nil, nil, seasons, nil).WithMapping(mapping)
	titles := NewTitleAdminService(flix).WithMapping(mapping)

	seed(t, flix, []seedTitle{
		{Slug: "a-different-world", Name: "A Different World", Kind: "tv_show", TmdbID: "115"},
		{Slug: "a-different-world-2026", Name: "A Different World", Kind: "tv_show", TmdbID: "999"},
	}, []seedRanking{
		{"a-different-world", "2026-10-01", "US", "netflix", "tv_shows", 1},
		{"a-different-world-2026", "2026-10-01", "US", "netflix", "tv_shows", 2},
	})
	global := "week\tcategory\tweekly_rank\tshow_title\tseason_title\tweekly_hours_viewed\truntime\tweekly_views\tcumulative_weeks_in_top_10\n" +
		"2026-10-04\tTV (English)\t1\tA Different World\tA Different World: Season 1\t10\t1\t10\t1\n"
	if _, _, err := n.importGlobal(ctx, strings.NewReader(global)); err != nil {
		t.Fatal(err)
	}
	sr, err := mapping.Sync(ctx, true, nil)
	if err != nil || sr.Links != 2 || sr.Conflicts != 1 || sr.NetflixUpdated != 0 {
		t.Fatalf("sync = %+v %v", sr, err)
	}
	if p, _ := mapping.List(ctx, MappingQuery{Flag: "conflict"}, 10, 0); p.Total != 2 {
		t.Fatalf("conflict entries = %d", p.Total)
	}

	revival, _ := flix.GetTitleBySlug(ctx, "a-different-world-2026")
	fixed := "305357"
	if _, err := titles.PatchTitleIDs(ctx, revival.ID, PatchTitleIDsInput{TmdbID: &fixed}); err != nil {
		t.Fatal(err)
	}
	if _, err := mapping.Sync(ctx, true, nil); err != nil {
		t.Fatal(err)
	}
	var tmdb, source string
	if err := db.QueryRow(`SELECT tmdb_id, match_source FROM netflix_titles WHERE show_title = 'A Different World'`).Scan(&tmdb, &source); err != nil {
		t.Fatal(err)
	}
	if tmdb != "305357" || source != MatchSourceFlixPatrolTitle {
		t.Fatalf("netflix title = %s via %s, want the hand fix", tmdb, source)
	}
	orig, _ := flix.GetTitleBySlug(ctx, "a-different-world")
	if orig.TmdbID.String != "115" {
		t.Fatalf("the other flixpatrol title was overwritten: %s", orig.TmdbID.String)
	}
	// Even unmatched, it does not take the Netflix title's mapping: that
	// title is linked to both and so says nothing certain about either.
	if _, err := titles.Rematch(ctx, orig.ID, true); err != nil {
		t.Fatal(err)
	}
	if _, err := mapping.Sync(ctx, true, nil); err != nil {
		t.Fatal(err)
	}
	if orig, _ = flix.GetTitleBySlug(ctx, "a-different-world"); orig.TmdbID.Valid {
		t.Fatalf("unmatched flixpatrol title took an ambiguous link's mapping: %s", orig.TmdbID.String)
	}
}
