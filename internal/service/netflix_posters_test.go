package service

import (
	"context"
	"testing"

	"github.com/bugsbunny-25/metareel/internal/client"
	"github.com/bugsbunny-25/metareel/internal/repository"
)

// Netflix's lists carry the TMDB poster of each mapped title, so a client
// needs no second lookup; unmapped titles have none.
func TestNetflixListsHavePosters(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	meta := repository.NewMetadataRepository(db)
	if err := meta.SaveDetails(ctx, &client.TMDBFullDetails{Kind: "tv", ID: "500", Title: "Show", PosterPath: "/show.jpg"}, false); err != nil {
		t.Fatal(err)
	}
	for _, stmt := range []string{
		`INSERT INTO netflix_titles (id, show_title, kind, match_status, tmdb_kind, tmdb_id) VALUES (1, 'Show', 'tv_show', 'matched', 'tv', '500')`,
		`INSERT INTO netflix_titles (id, show_title, kind, match_status) VALUES (2, 'Unmapped', 'movie', 'unmatched')`,
		`INSERT INTO netflix_top10_global (week, category, weekly_rank, netflix_title_id, show_title) VALUES
		 ('2026-10-04', 'TV (English)', 1, 1, 'Show'), ('2026-10-04', 'Films (English)', 1, 2, 'Unmapped')`,
		`INSERT INTO netflix_top10_countries (country, week, category, weekly_rank, netflix_title_id, show_title) VALUES ('US', '2026-10-04', 'TV', 1, 1, 'Show')`,
		`INSERT INTO netflix_most_popular (category, rank, netflix_title_id, show_title) VALUES ('TV (English)', 1, 1, 'Show'), ('Films (English)', 1, 2, 'Unmapped')`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}
	svc := NewAnalyticsService(repository.NewAnalyticsRepository(db), meta, nil, nil)
	poster := map[string]string{}
	week, err := svc.NetflixWeek(ctx, "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range week.Items {
		poster[it.ShowTitle] = deref1(it.PosterPath)
	}
	if poster["Show"] != "/show.jpg" || poster["Unmapped"] != "" {
		t.Fatalf("global posters = %v", poster)
	}
	us, err := svc.NetflixWeek(ctx, "US", "", "")
	if err != nil || len(us.Items) != 1 || deref1(us.Items[0].PosterPath) != "/show.jpg" {
		t.Fatalf("country week = %+v, %v", us, err)
	}
	pop, err := svc.NetflixMostPopular(ctx, "")
	if err != nil || len(pop.Items) != 2 {
		t.Fatalf("most popular = %+v, %v", pop, err)
	}
	for _, it := range pop.Items {
		if want := map[string]string{"Show": "/show.jpg", "Unmapped": ""}[it.ShowTitle]; deref1(it.PosterPath) != want {
			t.Fatalf("most popular %s poster = %q", it.ShowTitle, deref1(it.PosterPath))
		}
	}
}

func deref1(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
