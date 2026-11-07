package service

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/bugsbunny-25/metareel/internal/client"
	"github.com/bugsbunny-25/metareel/internal/repository"
)

func TestKeepCredits(t *testing.T) {
	c := &client.TMDBCredits{}
	for i := 30; i >= 0; i-- { // out of order
		c.Cast = append(c.Cast, client.TMDBCredit{PersonID: 100 + i, Name: "A", Order: i})
	}
	c.Cast = append(c.Cast, client.TMDBCredit{PersonID: 100, Name: "A", Order: 40}) // same person again
	for i := 0; i < 12; i++ {
		c.Crew = append(c.Crew, client.TMDBCredit{PersonID: 200 + i, Name: "D", Job: "Director", Episodes: i})
	}
	c.Crew = append(c.Crew,
		client.TMDBCredit{PersonID: 300, Name: "W", Job: "Screenplay"}, client.TMDBCredit{PersonID: 300, Name: "W", Job: "Screenplay"},
		client.TMDBCredit{PersonID: 301, Name: "G", Job: "Gaffer"}, client.TMDBCredit{PersonID: 0, Name: "X", Job: "Director"})
	cast, crew := keepCredits(c)
	if len(cast) != creditsMaxCast || cast[0].PersonID != 100 || cast[24].PersonID != 124 {
		t.Fatalf("cast = %d, first %d, last %d", len(cast), cast[0].PersonID, cast[len(cast)-1].PersonID)
	}
	if len(crew) != 11 || crew[0].PersonID != 211 || crew[9].PersonID != 202 || crew[10].Job != "Screenplay" {
		t.Fatalf("crew = %+v", crew)
	}
}

type fakeCredits map[string]*client.TMDBCredits

func (f fakeCredits) Enabled() bool { return true }
func (f fakeCredits) Credits(_ context.Context, kind, id string) (*client.TMDBCredits, error) {
	if id == "999" {
		return nil, errors.New("boom")
	}
	return f[kind+":"+id], nil
}

func TestCreditsRefreshAndPeople(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	meta := repository.NewMetadataRepository(db)
	for _, d := range []client.TMDBFullDetails{
		{Kind: "movie", ID: "1", Title: "Heist", ReleaseDate: "2026-03-01"},
		{Kind: "tv", ID: "2", Title: "Show", ReleaseDate: "2025-01-01"},
		{Kind: "movie", ID: "3", Title: "Unknown to TMDB"},
		{Kind: "movie", ID: "4", Title: "Sequel", ReleaseDate: "2026-09-01"},
	} {
		if err := meta.SaveDetails(ctx, &d, false); err != nil {
			t.Fatal(err)
		}
	}
	actor := func(id, order int, character string) client.TMDBCredit {
		return client.TMDBCredit{PersonID: id, Name: map[int]string{10: "Ana", 11: "Ben", 12: "Cy"}[id], Department: "Acting", Character: character, Order: order}
	}
	crew := func(id int, job string) client.TMDBCredit {
		return client.TMDBCredit{PersonID: id, Name: map[int]string{20: "Dee", 21: "Eli"}[id], Job: job, Department: "Directing"}
	}
	src := fakeCredits{
		"movie:1": {Cast: []client.TMDBCredit{actor(10, 0, "Lead"), actor(11, 12, "Extra")}, Crew: []client.TMDBCredit{crew(20, "Director"), crew(21, "Screenplay"), crew(21, "Story")}},
		"tv:2":    {Cast: []client.TMDBCredit{actor(10, 1, "Host"), actor(12, 0, "Star")}, Crew: []client.TMDBCredit{crew(21, "Creator")}},
		"movie:4": {Cast: []client.TMDBCredit{actor(11, 0, "Lead")}, Crew: []client.TMDBCredit{crew(20, "Director")}},
	}
	repo := repository.NewCreditsRepository(db)
	r := NewCreditsRefresher(slog.New(slog.NewTextHandler(io.Discard, nil)), src, repo)
	r.gap = 0
	rep, err := r.Run(ctx, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Due != 4 || rep.Fetched != 4 || rep.Missing != 1 || rep.Cast != 5 || rep.Crew != 5 {
		t.Fatalf("report = %+v", rep)
	}
	if rep, _ = r.Run(ctx, false, nil); rep.Due != 0 {
		t.Fatalf("second run due = %d", rep.Due)
	}
	r.now = func() time.Time { return time.Now().Add(creditsRefreshAfter + time.Hour) }
	if rep, _ = r.Run(ctx, false, nil); rep.Due != 4 {
		t.Fatalf("after 60 days due = %d", rep.Due)
	}

	svc := NewCreditsService(repo, meta)
	tc, err := svc.TitleCredits(ctx, TmdbRef{Kind: "movie", ID: "1"})
	if err != nil {
		t.Fatal(err)
	}
	if len(tc.Cast) != 2 || tc.Cast[0].Name != "Ana" || tc.Cast[0].Character != "Lead" || len(tc.Crew) != 3 || *tc.Title != "Heist" {
		t.Fatalf("title credits = %+v", tc)
	}
	if _, err := svc.TitleCredits(ctx, TmdbRef{Kind: "movie", ID: "77"}); !errors.Is(err, ErrTitleNotFound) {
		t.Fatalf("unknown title: %v", err)
	}

	// Charts: Heist 3 days at #1 (30 points), Show 1 day at #5 (6), Sequel
	// 1 day at #2 (9).
	d := func(n int) string { return time.Now().UTC().AddDate(0, 0, -n).Format("2006-01-02") }
	seed(t, db, []seedTitle{{Slug: "heist", Name: "Heist", Kind: "movie", TmdbID: "1"}, {Slug: "show", Name: "Show", Kind: "tv_show", TmdbID: "2"},
		{Slug: "sequel", Name: "Sequel", Kind: "movie", TmdbID: "4"}},
		[]seedRanking{{"heist", d(3), "US", "netflix", "movies", 1}, {"heist", d(4), "US", "netflix", "movies", 1}, {"heist", d(5), "GB", "netflix", "movies", 1},
			{"show", d(3), "US", "netflix", "tv_shows", 5}, {"sequel", d(2), "US", "netflix", "movies", 2}})

	ana, err := svc.Person(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if ana.Name != "Ana" || len(ana.Titles) != 2 || ana.Points != 36 || ana.Charting != 2 || ana.Titles[0].TmdbID != "1" ||
		ana.Titles[0].Roles[0] != "as Lead" || ana.Titles[0].Days != 3 || *ana.Titles[0].BestRank != 1 || ana.Titles[0].Countries != 2 {
		t.Fatalf("ana = %+v", ana)
	}
	if eli, _ := svc.Person(ctx, 21); len(eli.Titles) != 2 || len(eli.Titles[0].Roles) != 2 {
		t.Fatalf("eli (two writing credits on Heist) = %+v", eli)
	}
	if _, err := svc.Person(ctx, 404); !errors.Is(err, ErrPersonNotFound) {
		t.Fatalf("unknown person: %v", err)
	}

	an := NewAnalyticsService(repository.NewAnalyticsRepository(db), meta, nil, nil).WithCredits(repo)
	people, err := an.People(ctx, AnalyticsQuery{})
	if err != nil {
		t.Fatal(err)
	}
	// Actors billed in the top 10: Ana (Heist 30 + Show 6), Ben (Sequel 9;
	// his Heist role is billed 12th), Cy (Show 6).
	if people.Role != "actor" || people.Total != 3 || people.People[0].Name != "Ana" || people.People[0].Points != 36 || people.People[0].Titles != 2 ||
		people.People[1].Name != "Ben" || people.People[1].Points != 9 || people.People[0].TopTitles[0].TmdbID != "1" {
		t.Fatalf("actors = %+v", people.People)
	}
	dirs, _ := an.People(ctx, AnalyticsQuery{Role: "director"})
	if dirs.Total != 1 || dirs.People[0].Name != "Dee" || dirs.People[0].Points != 39 || dirs.People[0].BestRank != 1 {
		t.Fatalf("directors = %+v", dirs.People)
	}
	if w, _ := an.People(ctx, AnalyticsQuery{Role: "writer"}); w.Total != 1 || w.People[0].Points != 30 || w.People[0].Titles != 1 {
		t.Fatalf("writers (Screenplay + Story count once) = %+v", w.People)
	}
	if us, _ := an.People(ctx, AnalyticsQuery{Role: "director", Country: "GB"}); us.People[0].Points != 10 {
		t.Fatalf("GB only = %+v", us.People)
	}
	var v *ValidationError
	if _, err := an.People(ctx, AnalyticsQuery{Role: "gaffer"}); !errors.As(err, &v) {
		t.Fatalf("bad role: %v", err)
	}
}
