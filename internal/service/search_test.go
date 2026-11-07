package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/bugsbunny-25/metareel/internal/client"
	"github.com/bugsbunny-25/metareel/internal/repository"
)

func TestFoldName(t *testing.T) {
	for in, want := range map[string]string{
		"Pokémon: Detective Pikachu": "pokemon detective pikachu",
		"Law & Order: SVU":           "law and order svu",
		"  Amélie  ":                 "amelie",
		"พี่นาค 5":                   "พนาค 5", // Thai combining marks are dropped like accents
		"Spider-Man: No Way Home":    "spider man no way home",
	} {
		if got := foldName(in); got != want {
			t.Errorf("foldName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestMatchScore(t *testing.T) {
	name := func(s string) searchName {
		f := foldName(s)
		return searchName{norm: f, tokens: splitWords(f), raw: s}
	}
	cases := []struct {
		q, name string
		want    int
	}{
		{"stranger things", "Stranger Things", 100},
		{"stranger", "Stranger Things", 85},
		{"strang thin", "Stranger Things", 70},
		{"things strang", "Stranger Things", 60},
		{"ranger thi", "Stranger Things", 40}, // inside the name, not at word starts
		{"rangers", "Stranger Things", 0},
		{"anger", "Stranger Things", 40},
		{"pokemon", "Pokémon: Detective Pikachu", 85},
		{"wednesday", "Wednesday: Season 2", 85},
		{"season", "Wednesday: Season 2", 70},
		{"xyz", "Stranger Things", 0},
	}
	for _, c := range cases {
		q := foldName(c.q)
		if got := matchScore(q, splitWords(q), name(c.name)); got != c.want {
			t.Errorf("matchScore(%q, %q) = %d, want %d", c.q, c.name, got, c.want)
		}
	}
}

func TestSearch(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	seed(t, db,
		[]seedTitle{
			{Slug: "stranger-things", Name: "Stranger Things", Kind: "tv_show", TmdbID: "66732"},
			{Slug: "strange-darling", Name: "Strange Darling", Kind: "movie", TmdbID: "1"},
			{Slug: "amelie", Name: "Amelie", Kind: "movie", TmdbID: "194"},
			{Slug: "unmapped", Name: "Strangers", Kind: "movie"},
		},
		[]seedRanking{
			{"stranger-things", "2026-10-01", "US", "netflix", "tv_shows", 1},
			{"strange-darling", "2026-09-01", "US", "netflix", "movies", 1},
			{"amelie", "2026-08-01", "US", "netflix", "movies", 2},
		})
	meta := repository.NewMetadataRepository(db)
	for _, d := range []client.TMDBFullDetails{
		{Kind: "tv", ID: "66732", Title: "Stranger Things", ReleaseDate: "2016-07-15", PosterPath: "/st.jpg"},
		{Kind: "movie", ID: "194", Title: "Amélie", OriginalTitle: "Le Fabuleux Destin d'Amélie Poulain", ReleaseDate: "2001-04-25"},
	} {
		if err := meta.SaveDetails(ctx, &d, false); err != nil {
			t.Fatal(err)
		}
	}
	svc := NewSearchService(repository.NewSearchRepository(db))

	got, err := svc.Search(ctx, SearchQuery{Q: "strange"})
	if err != nil {
		t.Fatal(err)
	}
	// Both start with "strange"; the more recently charted first. The
	// unmapped "Strangers" is not a TMDB title and is left out.
	if len(got.Items) != 2 || got.Items[0].TmdbID != "66732" || got.Items[1].TmdbID != "1" {
		t.Fatalf("strange = %+v", got.Items)
	}
	st := got.Items[0]
	if st.Kind != "tv" || st.Year != 2016 || st.PosterPath != "/st.jpg" || st.LastCharted != "2026-10-01" || len(st.Sources) != 1 || st.Sources[0] != "flixpatrol" {
		t.Fatalf("stranger things = %+v", st)
	}
	if got.Items[1].Title != "Strange Darling" { // no TMDB details: the chart name
		t.Fatalf("title without details = %+v", got.Items[1])
	}

	// Accents and the original title match.
	got, _ = svc.Search(ctx, SearchQuery{Q: "fabuleux destin"})
	if len(got.Items) != 1 || got.Items[0].TmdbID != "194" || got.Items[0].Matched != "Le Fabuleux Destin d'Amélie Poulain" {
		t.Fatalf("original title = %+v", got.Items)
	}
	if got, _ = svc.Search(ctx, SearchQuery{Q: "AMELIE"}); len(got.Items) != 1 || got.Items[0].Score != 100 {
		t.Fatalf("accent-free = %+v", got.Items)
	}
	if got, _ = svc.Search(ctx, SearchQuery{Q: "strange", Kind: "movie"}); len(got.Items) != 1 || got.Items[0].TmdbID != "1" {
		t.Fatalf("kind filter = %+v", got.Items)
	}
	if got, _ = svc.Search(ctx, SearchQuery{Q: "strange", Limit: 1}); len(got.Items) != 1 {
		t.Fatalf("limit = %+v", got.Items)
	}

	// The index follows new data once the stamp is checked again.
	seed(t, db, []seedTitle{{Slug: "strange-new", Name: "Strange New Worlds", Kind: "tv_show", TmdbID: "103516"}}, nil)
	if got, _ = svc.Search(ctx, SearchQuery{Q: "strange new"}); len(got.Items) != 0 {
		t.Fatalf("index rebuilt before the stamp TTL: %+v", got.Items)
	}
	svc.now = func() time.Time { return time.Now().Add(time.Minute) }
	if got, _ = svc.Search(ctx, SearchQuery{Q: "strange new"}); len(got.Items) != 1 || got.Items[0].TmdbID != "103516" {
		t.Fatalf("after rebuild = %+v", got.Items)
	}

	var v *ValidationError
	for _, q := range []SearchQuery{{Q: "a"}, {Q: " - "}, {Q: "ok", Kind: "show"}, {Q: strings.Repeat("a", 201)}} {
		if _, err := svc.Search(ctx, q); !errors.As(err, &v) {
			t.Errorf("%+v: err = %v", q, err)
		}
	}
}

func splitWords(s string) []string { return strings.Fields(s) }
