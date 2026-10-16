package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strconv"
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
	withNetflix := func(h client.WikidataNetflixHit, nf string) client.WikidataNetflixHit { h.NetflixID = nf; return h }
	for name, tt := range map[string]struct {
		hits     []client.WikidataNetflixHit
		list     string
		year     int
		want     string
		accepted bool
		own      string // the title's own Netflix ID
	}{
		// The 2025 remake charting: Wikidata only has the 2010 film's Netflix ID.
		"own netflix id rules out a same-name film": {[]client.WikidataNetflixHit{withNetflix(hit("movie", "10191", 2010), "70109893")}, "movie", 2025, "", false, "83077834"},
		"own netflix id agrees":                     {[]client.WikidataNetflixHit{withNetflix(hit("movie", "10191", 2010), "70109893"), withNetflix(hit("movie", "1087192", 2025), "83077834")}, "movie", 2025, "1087192", true, "83077834"},
		"same kind":                                 {[]client.WikidataNetflixHit{hit("tv", "94605", 2021)}, "tv_show", 2024, "94605", true, ""},
		"special in the TV list":                    {[]client.WikidataNetflixHit{hit("movie", "1227770", 2024)}, "tv_show", 2024, "1227770", true, ""},
		"older film of the same name":               {[]client.WikidataNetflixHit{hit("movie", "393519", 2016)}, "tv_show", 2026, "", false, ""},
		"several, one recent":                       {[]client.WikidataNetflixHit{hit("movie", "1", 2004), hit("movie", "2", 2026)}, "movie", 2026, "2", true, ""},
		"several, none recent":                      {[]client.WikidataNetflixHit{hit("movie", "1", 2004), hit("movie", "2", 2010)}, "movie", 2026, "", false, ""},
		"no year for other kind":                    {[]client.WikidataNetflixHit{hit("movie", "1", 2024)}, "tv_show", 0, "", false, ""},
	} {
		h, ok := pickWikidataNetflix(tt.hits, tt.list, tt.year, tt.own)
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
	mapping := NewTitleMappingService(log, repository.NewMappingRepository(db), testWriter(db), meta, nil, seasons)
	n := NewNetflixImporter(log, nil, imports, testWriter(db), flix, repository.NewTaskScheduleRepository(db), nil, nil, seasons, nil).WithMapping(mapping)
	admin := NewNetflixAdminService(log, imports, testWriter(db), meta, nil, seasons, nil).WithMapping(mapping)
	titles := NewTitleAdminService(flix, testWriter(db)).WithMapping(mapping)

	seed(t, db, []seedTitle{
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
	mapping := NewTitleMappingService(log, repository.NewMappingRepository(db), testWriter(db), meta, nil, seasons)
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
	n := NewNetflixImporter(log, nil, imports, testWriter(db), flix, repository.NewTaskScheduleRepository(db), nil, metadata, seasons, nil).
		WithMapping(mapping).WithNetflixSearch(search)
	admin := NewNetflixAdminService(log, imports, testWriter(db), meta, nil, seasons, nil).WithMapping(mapping)

	// TMDB already knows Wednesday's Netflix ID (from Wikidata); FlixPatrol
	// matched its title to the wrong show by name.
	if err := meta.SaveWikidata(ctx, repository.TmdbRef{Kind: "tv", ID: "119051"}, client.WikidataIDs{NetflixID: "81231974"}); err != nil {
		t.Fatal(err)
	}
	seed(t, db, []seedTitle{{Slug: "wednesday", Name: "Wednesday", Kind: "tv_show", TmdbID: "555"}},
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
	mapping := NewTitleMappingService(log, repository.NewMappingRepository(db), testWriter(db), meta, nil, seasons)
	n := NewNetflixImporter(log, nil, imports, testWriter(db), flix, repository.NewTaskScheduleRepository(db), nil, nil, seasons, nil).WithMapping(mapping)
	titles := NewTitleAdminService(flix, testWriter(db)).WithMapping(mapping)

	seed(t, db, []seedTitle{
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

// A TMDB title's IDs come from the strongest source that has one: manual >
// tmdb > wikidata > match; clearing a hand-set ID hands it back.
func TestTmdbTitleIDSources(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	meta := repository.NewMetadataRepository(db)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	mapping := NewTitleMappingService(log, repository.NewMappingRepository(db), testWriter(db), meta, nil, nil)
	ref := repository.TmdbRef{Kind: "movie", ID: "100"}
	ids := func() string {
		t.Helper()
		m, err := meta.Get(ctx, ref)
		if err != nil || m == nil {
			t.Fatalf("tmdb title: %+v %v", m, err)
		}
		return ptrStr(m.IMDbID) + "/" + m.IDSources["imdb"] + " " + ptrStr(m.WikidataID) + "/" + m.IDSources["wikidata"]
	}
	save := func(imdb, wd, source string) {
		t.Helper()
		if err := meta.SaveIDs(ctx, ref, repository.TmdbIDs{IMDbID: imdb, WikidataID: wd}, source); err != nil {
			t.Fatal(err)
		}
	}
	for i, step := range []struct {
		imdb, wd, source, want string
	}{
		{"tt1", "", repository.IDSourceMatch, "tt1/match <nil>/"},
		{"tt2", "Q2", repository.IDSourceWikidata, "tt2/wikidata Q2/wikidata"},
		{"tt3", "", repository.IDSourceTMDB, "tt3/tmdb Q2/wikidata"},
		{"tt4", "Q4", repository.IDSourceMatch, "tt3/tmdb Q2/wikidata"}, // weaker: ignored
		{"tt5", "", repository.IDSourceTMDB, "tt5/tmdb Q2/wikidata"},    // TMDB corrects itself
	} {
		save(step.imdb, step.wd, step.source)
		if got := ids(); got != step.want {
			t.Fatalf("step %d: ids = %s, want %s", i, got, step.want)
		}
	}
	session := testWriter(db).Session("test")
	hand, none := "tt9", ""
	if err := mapping.SetManualIDs(ctx, session, ref, &hand, nil, nil); err != nil {
		t.Fatal(err)
	}
	save("tt6", "", repository.IDSourceTMDB)
	if got := ids(); got != "tt9/manual Q2/wikidata" {
		t.Fatalf("after a hand-set IMDb ID = %s", got)
	}
	if err := mapping.SetManualIDs(ctx, session, ref, &none, nil, nil); err != nil {
		t.Fatal(err)
	}
	if got := ids(); got != "<nil>/ Q2/wikidata" {
		t.Fatalf("after clearing it = %s", got)
	}
	save("tt6", "", repository.IDSourceTMDB)
	if got := ids(); got != "tt6/tmdb Q2/wikidata" {
		t.Fatalf("handed back = %s", got)
	}
	if p := session.Take(); len(p.Refs) != 1 || p.Refs[0] != ref {
		t.Fatalf("hand-set IDs did not queue a refresh: %+v", p)
	}
}

// A title's mapping is computed from its own evidence and that of the title
// linked to it; nothing is copied, so withdrawing evidence withdraws what
// was shared from it.
func TestEvidenceMapping(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	flix := repository.NewFlixPatrolRepository(db)
	imports := repository.NewImportsRepository(db)
	meta := repository.NewMetadataRepository(db)
	mrepo := repository.NewMappingRepository(db)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	w := testWriter(db)
	mapping := NewTitleMappingService(log, mrepo, w, meta, nil, NewSeasonMapper(log, imports))
	n := NewNetflixImporter(log, nil, imports, w, flix, repository.NewTaskScheduleRepository(db), nil, nil, nil, nil)

	seed(t, db, []seedTitle{{Slug: "show", Name: "Show", Kind: "movie"}}, nil)
	global := "week\tcategory\tweekly_rank\tshow_title\tseason_title\tweekly_hours_viewed\truntime\tweekly_views\tcumulative_weeks_in_top_10\n" +
		"2026-10-04\tFilms (English)\t1\tShow\tN/A\t10\t1\t10\t1\n"
	if _, _, err := n.importGlobal(ctx, strings.NewReader(global)); err != nil {
		t.Fatal(err)
	}
	fp, _ := flix.GetTitleBySlug(ctx, "show")
	const nfID = int64(1)
	if err := mrepo.ReplaceLinks(ctx, []repository.TitleLink{{NetflixTitleID: nfID, TitleID: fp.ID}}, nil); err != nil {
		t.Fatal(err)
	}
	state := func() string {
		t.Helper()
		f, err := flix.GetTitleBySlug(ctx, "show")
		if err != nil {
			t.Fatal(err)
		}
		nt, err := imports.GetNetflixTitle(ctx, nfID)
		if err != nil {
			t.Fatal(err)
		}
		return fmt.Sprintf("fp %s/%s/%s nf %s/%s/%s", f.TmdbID.String, f.MatchStatus, f.MatchSource.String,
			nt.TmdbID.String, nt.MatchStatus, nt.MatchSource.String)
	}
	s := w.Session("test")
	steps := []struct {
		name string
		do   func() error
		want string
	}{
		{"netflix match is shared with the unmatched FlixPatrol title", func() error {
			return s.MatchNetflixTitle(ctx, nfID, repository.NetflixMatch{TmdbKind: "movie", TmdbID: "1", Source: "justwatch"})
		}, "fp 1/matched/netflix_title nf 1/matched/justwatch"},
		{"FlixPatrol's own match beats Netflix's", func() error {
			_, err := s.MatchTitle(ctx, fp.ID, repository.TitleMatchResult{Kind: "movie", TmdbID: "2", Source: "justwatch_path", ImdbID: "tt2"})
			return err
		}, "fp 2/matched/justwatch_path nf 2/matched/flixpatrol_title"},
		{"a Netflix fix by hand wins for both", func() error {
			return s.SetNetflixMapping(ctx, nfID, "movie", "9", "")
		}, "fp 9/matched/netflix_title nf 9/manual/manual"},
		{"lifting the lock brings back the matches", func() error {
			return s.RequeueNetflixTitle(ctx, nfID, false)
		}, "fp 2/matched/justwatch_path nf 2/matched/flixpatrol_title"},
		{"clearing FlixPatrol's match falls back to Netflix's", func() error {
			_, err := s.RequeueTitle(ctx, fp.ID, true)
			return err
		}, "fp 1/matched/netflix_title nf 1/matched/justwatch"},
		{"clearing Netflix's leaves both unmapped and due", func() error {
			return s.RequeueNetflixTitle(ctx, nfID, true)
		}, "fp /pending/ nf /pending/"},
		{"a Netflix ID beats a FlixPatrol match", func() error {
			if _, err := s.MatchTitle(ctx, fp.ID, repository.TitleMatchResult{Kind: "movie", TmdbID: "2", Source: "justwatch_path"}); err != nil {
				return err
			}
			_, err := s.MatchNetflixByNetflixID(ctx, nfID, repository.TmdbRef{Kind: "movie", ID: "3"}, "")
			return err
		}, "fp 3/matched/netflix_title nf 3/matched/netflix_id"},
	}
	for _, st := range steps {
		if err := st.do(); err != nil {
			t.Fatalf("%s: %v", st.name, err)
		}
		if got := state(); got != st.want {
			t.Fatalf("%s: %s, want %s", st.name, got, st.want)
		}
	}

	// Each change is in the history with what caused it.
	events, err := mapping.Events(ctx, MappingEventsQuery{Subject: "flixpatrol:" + strconv.FormatInt(fp.ID, 10), Limit: 100})
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for i := len(events) - 1; i >= 0; i-- {
		e := events[i]
		got = append(got, e.Action+":"+ptrStr(e.FromID)+">"+ptrStr(e.ToID))
	}
	want := "adopt:<nil>>1 match:1>2 adopt:2>9 unshare:9>2 clear:2>1 unshare:1><nil> match:<nil>>2 adopt:2>3"
	if strings.Join(got, " ") != want {
		t.Fatalf("flixpatrol history = %v\nwant %s", got, want)
	}

	// The FlixPatrol match's IMDb ID disagrees with TMDB's: listed only while
	// that match decides the title's mapping.
	if err := meta.SaveIDs(ctx, repository.TmdbRef{Kind: "movie", ID: "2"}, repository.TmdbIDs{IMDbID: "tt2-tmdb"}, repository.IDSourceTMDB); err != nil {
		t.Fatal(err)
	}
	if _, err := s.MatchTitle(ctx, fp.ID, repository.TitleMatchResult{Kind: "movie", TmdbID: "2", Source: "justwatch_path", ImdbID: "tt2"}); err != nil {
		t.Fatal(err)
	}
	mm, err := mrepo.IMDbMismatches(ctx, 10)
	if err != nil || len(mm) != 1 || mm[0].SubjectID != fp.ID || mm[0].IMDbID != "tt2" || mm[0].TmdbIMDbID != "tt2-tmdb" {
		t.Fatalf("imdb mismatches = %+v %v", mm, err)
	}
	if _, err := s.SetTitleMapping(ctx, fp.ID, "movie", "2", ""); err != nil { // confirmed by hand
		t.Fatal(err)
	}
	if mm, _ := mrepo.IMDbMismatches(ctx, 10); len(mm) != 0 {
		t.Fatalf("mismatch after confirming by hand = %+v", mm)
	}
}

func ptrStr(p *string) string {
	if p == nil {
		return "<nil>"
	}
	return *p
}

// A session queues one refresh with every TMDB title it mapped to and every
// Netflix title whose mapping changed; nothing when nothing changed.
func TestMappingWriterQueuesOneRefresh(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	flix := repository.NewFlixPatrolRepository(db)
	imports := repository.NewImportsRepository(db)
	var queued []MappingRefresh
	w := NewMappingWriter(slog.New(slog.NewTextHandler(io.Discard, nil)), repository.NewMappingRepository(db)).
		WithRefreshQueue(func(_ context.Context, r MappingRefresh) error { queued = append(queued, r); return nil })
	seed(t, db, []seedTitle{{Slug: "a", Name: "A", Kind: "movie"}, {Slug: "b", Name: "B", Kind: "tv_show"}}, nil)
	n := NewNetflixImporter(slog.New(slog.NewTextHandler(io.Discard, nil)), nil, imports, w, flix, repository.NewTaskScheduleRepository(db), nil, nil, nil, nil)
	global := "week\tcategory\tweekly_rank\tshow_title\tseason_title\tweekly_hours_viewed\truntime\tweekly_views\tcumulative_weeks_in_top_10\n" +
		"2026-10-04\tFilms (English)\t1\tN\tN/A\t10\t1\t10\t1\n"
	if _, _, err := n.importGlobal(ctx, strings.NewReader(global)); err != nil {
		t.Fatal(err)
	}
	a, _ := flix.GetTitleBySlug(ctx, "a")
	b, _ := flix.GetTitleBySlug(ctx, "b")

	s := w.Session("test")
	if _, err := s.MatchTitle(ctx, a.ID, repository.TitleMatchResult{Kind: "movie", TmdbID: "1", Source: "test"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.MatchTitle(ctx, b.ID, repository.TitleMatchResult{Kind: "tv_show", RetryAfter: "+1 day"}); err != nil { // no match: no change
		t.Fatal(err)
	}
	if err := s.MatchNetflixTitle(ctx, 1, repository.NetflixMatch{TmdbKind: "movie", TmdbID: "1", Source: "test"}); err != nil {
		t.Fatal(err)
	}
	s.Flush(ctx)
	s.Flush(ctx) // nothing new
	if len(queued) != 1 || len(queued[0].Refs) != 1 || queued[0].Refs[0] != (repository.TmdbRef{Kind: "movie", ID: "1"}) ||
		len(queued[0].NetflixIDs) != 1 || queued[0].NetflixIDs[0] != 1 || s.Changes() != 2 {
		t.Fatalf("queued = %+v, changes %d", queued, s.Changes())
	}
	// The same match again is no change.
	s2 := w.Session("test")
	if _, err := s2.MatchTitle(ctx, a.ID, repository.TitleMatchResult{Kind: "movie", TmdbID: "1", Source: "test"}); err != nil {
		t.Fatal(err)
	}
	s2.Flush(ctx)
	if len(queued) != 1 {
		t.Fatalf("unchanged match queued a refresh: %+v", queued)
	}
	// Automation never touches a title fixed by hand.
	if _, err := s2.SetTitleMapping(ctx, a.ID, "movie", "2", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := s2.MatchTitle(ctx, a.ID, repository.TitleMatchResult{Kind: "movie", TmdbID: "3"}); !repository.IsNoRows(err) {
		t.Fatalf("match over a manual title: %v", err)
	}
}

// A Netflix title mapped by name keeps its mapping, but its Netflix ID
// becomes its TMDB title's (source netflix, the weakest), every view of the
// title shows the TMDB title's IDs, and the mapping reference lists it.
func TestNetflixIDCopiedToTmdbTitle(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	flix := repository.NewFlixPatrolRepository(db)
	imports := repository.NewImportsRepository(db)
	meta := repository.NewMetadataRepository(db)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	seasons := NewSeasonMapper(log, imports)
	metadata := NewTitleMetadataService(log, meta, nil, nil)
	mapping := NewTitleMappingService(log, repository.NewMappingRepository(db), testWriter(db), meta, nil, seasons)
	search := &fakeNetflixSearch{items: map[string][]client.NetflixSearchItem{
		"The Gambler": {{NetflixID: "80000111", Title: "The Gambler", Category: "FILMS", GlobalWeeks: []string{"2026-10-04"}}},
	}}
	n := NewNetflixImporter(log, nil, imports, testWriter(db), flix, repository.NewTaskScheduleRepository(db), nil, metadata, seasons, nil).
		WithMapping(mapping).WithNetflixSearch(search)
	admin := NewNetflixAdminService(log, imports, testWriter(db), meta, nil, seasons, nil).WithMapping(mapping)
	titles := NewTitleAdminService(flix, testWriter(db)).WithMapping(mapping)

	seed(t, db, []seedTitle{
		{Slug: "the-gambler", Name: "The Gambler", Kind: "movie", TmdbID: "111", ImdbID: "tt0111"},
		{Slug: "unmapped-show", Name: "Unmapped Show", Kind: "tv_show"},
	}, []seedRanking{
		{"the-gambler", "2026-10-01", "US", "netflix", "movies", 1},
		{"unmapped-show", "2026-10-01", "US", "netflix", "tv_shows", 1},
	})
	global := "week\tcategory\tweekly_rank\tshow_title\tseason_title\tweekly_hours_viewed\truntime\tweekly_views\tcumulative_weeks_in_top_10\n" +
		"2026-10-04\tFilms (English)\t1\tThe Gambler\tN/A\t10\t1\t10\t1\n" +
		"2026-10-04\tFilms (English)\t2\tNetflix Only\tN/A\t10\t1\t10\t1\n"
	if _, _, err := n.importGlobal(ctx, strings.NewReader(global)); err != nil {
		t.Fatal(err)
	}
	if _, err := n.RunMatch(ctx, nil, nil); err != nil {
		t.Fatal(err)
	}
	ref := repository.TmdbRef{Kind: "movie", ID: "111"}
	netflixIDOf := func() string {
		t.Helper()
		m, err := meta.Get(ctx, ref)
		if err != nil || m == nil {
			t.Fatalf("tmdb title: %+v %v", m, err)
		}
		return ptrStr(m.NetflixID) + "/" + m.IDSources["netflix"]
	}
	if got := netflixIDOf(); got != "80000111/netflix" {
		t.Fatalf("tmdb netflix id = %s", got)
	}
	page, _ := admin.List(ctx, NetflixTitlesQuery{Search: "The Gambler"}, 10, 0)
	gambler, err := admin.Get(ctx, page.Items[0].ID)
	if err != nil || derefStr(gambler.MatchSource) != MatchSourceFlixPatrolTitle {
		t.Fatalf("netflix title = %+v %v", gambler.NetflixTitleResponse, err)
	}
	// A copied ID is not evidence: matching again does not turn the name
	// match into an exact Netflix ID match. Lookups still find it.
	if refs, _ := meta.RefsByNetflixID(ctx, "80000111", false); len(refs) != 0 {
		t.Fatalf("matching sees a copied netflix id: %+v", refs)
	}
	if refs, _ := meta.RefsByNetflixID(ctx, "80000111", true); len(refs) != 1 || refs[0] != ref {
		t.Fatalf("lookup by copied netflix id = %+v", refs)
	}
	if _, err := n.RunMatch(ctx, []int64{gambler.ID}, nil); err != nil {
		t.Fatal(err)
	}
	if again, _ := admin.Get(ctx, gambler.ID); derefStr(again.MatchSource) != MatchSourceFlixPatrolTitle {
		t.Fatalf("rematched by its copied netflix id: %s", derefStr(again.MatchSource))
	}

	// Every view of the title has the TMDB title's IDs.
	if gambler.IDs == nil || derefStr(gambler.IDs.NetflixID) != "80000111" || gambler.IDs.Sources["netflix"] != repository.IDSourceNetflix || derefStr(gambler.IDs.IMDbID) != "tt0111" {
		t.Fatalf("netflix title ids = %+v", gambler.IDs)
	}
	fp, _ := flix.GetTitleBySlug(ctx, "the-gambler")
	fpResp, err := titles.GetTitle(ctx, fp.ID)
	if err != nil || fpResp.IDs == nil || derefStr(fpResp.IDs.NetflixID) != "80000111" || len(fpResp.IDs.NetflixTop10) != 1 {
		t.Fatalf("flixpatrol title ids = %+v %v", fpResp.IDs, err)
	}
	unmapped, _ := flix.GetTitleBySlug(ctx, "unmapped-show")
	if r, _ := titles.GetTitle(ctx, unmapped.ID); r.IDs != nil {
		t.Fatalf("unmapped title has ids: %+v", r.IDs)
	}

	// The mapping reference: the linked pair is one row, the Netflix-only
	// film and the unmapped FlixPatrol title have their own.
	refs, err := mapping.Refs(ctx, "", false)
	if err != nil || len(refs) != 3 {
		t.Fatalf("mapping refs = %+v %v", refs, err)
	}
	pair := refs[0]
	if derefStr(pair.TmdbID) != "111" || derefStr(pair.FlixPatrolSlug) != "the-gambler" || derefStr(pair.FlixPatrolKind) != "movie" ||
		derefStr(pair.NetflixTitle) != "The Gambler" || derefStr(pair.NetflixKind) != "movie" || derefStr(pair.NetflixID) != "80000111" || derefStr(pair.IMDbID) != "tt0111" {
		t.Fatalf("pair row = %+v", pair)
	}
	var netflixOnly, fpOnly bool
	for _, r := range refs[1:] {
		netflixOnly = netflixOnly || (derefStr(r.NetflixTitle) == "Netflix Only" && r.FlixPatrolSlug == nil)
		fpOnly = fpOnly || (derefStr(r.FlixPatrolSlug) == "unmapped-show" && r.TmdbID == nil && r.NetflixTitleID == nil)
	}
	if !netflixOnly || !fpOnly {
		t.Fatalf("other rows = %+v", refs[1:])
	}
	if mapped, _ := mapping.Refs(ctx, "movie", true); len(mapped) != 1 {
		t.Fatalf("mapped movie refs = %d", len(mapped))
	}
	if _, err := mapping.Refs(ctx, "show", false); !isValidation(err) {
		t.Fatalf("bad kind: %v", err)
	}

	// Any other source replaces a copied ID; it never replaces theirs.
	if err := meta.SaveIDs(ctx, ref, repository.TmdbIDs{NetflixID: "70111111"}, repository.IDSourceMatch); err != nil {
		t.Fatal(err)
	}
	if _, err := meta.SyncNetflixIDs(ctx); err != nil {
		t.Fatal(err)
	}
	if got := netflixIDOf(); got != "70111111/match" {
		t.Fatalf("after a match = %s", got)
	}

	// Moved to another TMDB title: the copy moves with it.
	other := repository.TmdbRef{Kind: "movie", ID: "222"}
	if err := meta.SaveIDs(ctx, ref, repository.TmdbIDs{NetflixID: "80000111"}, repository.IDSourceNetflix); err != nil {
		t.Fatal(err)
	}
	if got := netflixIDOf(); got != "70111111/match" {
		t.Fatalf("netflix source replaced a match: %s", got)
	}
	if _, err := db.Exec(`UPDATE tmdb_titles SET netflix_id = '80000111', netflix_source = 'netflix' WHERE tmdb_id = '111'`); err != nil {
		t.Fatal(err)
	}
	id222 := "222"
	if _, err := admin.Patch(ctx, gambler.ID, PatchNetflixTitleInput{TmdbKind: &other.Kind, TmdbID: &id222}); err != nil {
		t.Fatal(err)
	}
	if _, err := mapping.Refresh(ctx, MappingRefresh{Refs: []repository.TmdbRef{ref, other}}, nil); err != nil {
		t.Fatal(err)
	}
	if got := netflixIDOf(); got != "<nil>/" {
		t.Fatalf("old tmdb title kept the copy: %s", got)
	}
	if m, _ := meta.Get(ctx, other); m == nil || ptrStr(m.NetflixID) != "80000111" || m.IDSources["netflix"] != repository.IDSourceNetflix {
		t.Fatalf("new tmdb title = %+v", m)
	}
}

// Netflix titles whose own Netflix ID differs from their TMDB title's are
// listed in Data health: likely wrong matches first; a special fixed by hand
// onto its show (whose own Netflix title has the ID) last; copied IDs and
// agreeing ones not at all.
func TestNetflixIDMismatches(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	for _, stmt := range []string{
		`INSERT INTO tmdb_titles (tmdb_kind, tmdb_id, title, netflix_id, netflix_source) VALUES
			('movie', '1', '13 Minutes', '80088000', 'wikidata'),
			('tv', '2', 'Let''s Marry Harry', '81990051', 'wikidata'),
			('movie', '3', 'Copied', '222', 'netflix'),
			('movie', '4', 'Agrees', '999', 'tmdb'),
			('movie', '5', 'No own ID', '555', 'match')`,
		`INSERT INTO netflix_titles (id, show_title, kind, tmdb_kind, tmdb_id, netflix_id, match_status, match_source) VALUES
			(1, '13 Minutes', 'movie', 'movie', '1', '81495233', 'matched', 'wikidata_netflix'),
			(2, 'Let''s Marry Harry: The Reunion', 'tv_show', 'tv', '2', '82875780', 'manual', 'manual'),
			(3, 'Let''s Marry Harry', 'tv_show', 'tv', '2', '81990051', 'matched', 'netflix_id'),
			(4, 'Copied', 'movie', 'movie', '3', '111', 'matched', 'flixpatrol_title'),
			(5, 'Agrees', 'movie', 'movie', '4', '999', 'matched', 'netflix_id'),
			(6, 'No own ID', 'movie', 'movie', '5', NULL, 'matched', 'justwatch_netflix')`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	got, err := repository.NewMappingRepository(db).NetflixIDMismatches(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	want := []repository.NetflixIDMismatch{
		{NetflixTitleID: 1, ShowTitle: "13 Minutes", List: "movie", TmdbKind: "movie", TmdbID: "1", TmdbTitle: "13 Minutes",
			NetflixID: "81495233", TmdbNetflixID: "80088000", TmdbNetflixSource: "wikidata", MatchStatus: "matched", MatchSource: "wikidata_netflix"},
		{NetflixTitleID: 2, ShowTitle: "Let's Marry Harry: The Reunion", List: "tv_show", TmdbKind: "tv", TmdbID: "2", TmdbTitle: "Let's Marry Harry",
			NetflixID: "82875780", TmdbNetflixID: "81990051", TmdbNetflixSource: "wikidata", MatchStatus: "manual", MatchSource: "manual", OtherTitleHasID: true},
	}
	if len(got) != len(want) {
		t.Fatalf("mismatches = %+v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("mismatch %d = %+v, want %+v", i, got[i], want[i])
		}
	}
	if one, _ := repository.NewMappingRepository(db).NetflixIDMismatches(ctx, 1); len(one) != 1 || one[0].NetflixTitleID != 1 {
		t.Fatalf("limit 1 = %+v", one)
	}
}

// A Netflix title mapped by a Wikidata label match to a title whose Netflix
// ID is not its own (the 2010 film for the 2025 remake) is matched again:
// the label match is cleared, recorded, and not made again.
func TestContradictedLabelMatchIsRedone(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	flix := repository.NewFlixPatrolRepository(db)
	imports := repository.NewImportsRepository(db)
	meta := repository.NewMetadataRepository(db)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	seasons := NewSeasonMapper(log, imports)
	metadata := NewTitleMetadataService(log, meta, nil, nil)
	mapping := NewTitleMappingService(log, repository.NewMappingRepository(db), testWriter(db), meta, nil, seasons)
	n := NewNetflixImporter(log, nil, imports, testWriter(db), flix, repository.NewTaskScheduleRepository(db), nil, metadata, seasons, nil).
		WithMapping(mapping).WithNetflixSearch(&fakeNetflixSearch{})
	admin := NewNetflixAdminService(log, imports, testWriter(db), meta, nil, seasons, nil).WithMapping(mapping)

	global := "week\tcategory\tweekly_rank\tshow_title\tseason_title\tweekly_hours_viewed\truntime\tweekly_views\tcumulative_weeks_in_top_10\n" +
		"2026-10-04\tFilms (English)\t1\tHow to Train Your Dragon\tN/A\t10\t1\t10\t1\n" +
		"2026-10-04\tFilms (English)\t2\tThe Addams Family\tN/A\t10\t1\t10\t1\n"
	if _, _, err := n.importGlobal(ctx, strings.NewReader(global)); err != nil {
		t.Fatal(err)
	}
	idOf := func(name string) int64 {
		p, _ := admin.List(ctx, NetflixTitlesQuery{Search: name}, 10, 0)
		return p.Items[0].ID
	}
	dragon, addams := idOf("How to Train Your Dragon"), idOf("The Addams Family")
	// Both were matched by label to the older film. The dragon's own Netflix
	// ID is the remake's; Addams has none, so nothing contradicts its match.
	old := repository.TmdbRef{Kind: "movie", ID: "10191"}
	session := testWriter(db).Session("test")
	for _, id := range []int64{dragon, addams} {
		if err := session.MatchNetflixTitle(ctx, id, repository.NetflixMatch{TmdbKind: "movie", TmdbID: old.ID, Source: "wikidata_netflix", RetryAfter: "+1 days"}); err != nil {
			t.Fatal(err)
		}
	}
	if err := meta.SaveIDs(ctx, old, repository.TmdbIDs{NetflixID: "70109893"}, repository.IDSourceMatch); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE netflix_titles SET netflix_id = '83077834', netflix_id_checked_at = CURRENT_TIMESTAMP WHERE id = ?`, dragon); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE netflix_titles SET netflix_id_checked_at = CURRENT_TIMESTAMP WHERE id = ?`, addams); err != nil {
		t.Fatal(err)
	}
	if mm, _ := repository.NewMappingRepository(db).NetflixIDMismatches(ctx, 10); len(mm) != 1 || mm[0].NetflixTitleID != dragon {
		t.Fatalf("mismatches before = %+v", mm)
	}

	rep, err := n.RunMatch(ctx, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Contradicted != 1 {
		t.Fatalf("contradicted = %d", rep.Contradicted)
	}
	d, _ := admin.Get(ctx, dragon)
	if d.TmdbID != nil || d.MatchStatus == repository.MatchStatusMatched {
		t.Fatalf("dragon still mapped: %s via %s (%s)", derefStr(d.TmdbID), derefStr(d.MatchSource), d.MatchStatus)
	}
	if a, _ := admin.Get(ctx, addams); derefStr(a.TmdbID) != old.ID {
		t.Fatalf("addams lost its uncontradicted match: %+v", a.NetflixTitleResponse)
	}
	events, err := mapping.Events(ctx, MappingEventsQuery{Subject: fmt.Sprintf("netflix:%d", dragon)})
	if err != nil || len(events) == 0 || events[0].Action != repository.MappingActionClear {
		t.Fatalf("events = %+v %v", events, err)
	}
	if mm, _ := repository.NewMappingRepository(db).NetflixIDMismatches(ctx, 10); len(mm) != 0 {
		t.Fatalf("mismatches after = %+v", mm)
	}
	// Nothing to redo the second time.
	if rep, err := n.RunMatch(ctx, nil, nil); err != nil || rep.Contradicted != 0 {
		t.Fatalf("second run = %+v %v", rep, err)
	}
}
