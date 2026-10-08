package client

import (
	"math"
	"testing"
)

func TestJaroSimilarity(t *testing.T) {
	tests := []struct {
		a, b string
		want float64
	}{
		{"martha", "marhta", 0.944},
		{"dixon", "dicksonx", 0.767},
		{"jellyfish", "smellyfish", 0.896},
		{"abc", "abc", 1},
		{"abc", "xyz", 0},
		{"", "", 1},
		{"a", "", 0},
	}
	for _, tt := range tests {
		if got := jaroSimilarity(tt.a, tt.b); math.Abs(got-tt.want) > 0.001 {
			t.Errorf("jaroSimilarity(%q, %q) = %.3f, want %.3f", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestBestRTHit(t *testing.T) {
	scored := &struct {
		CriticsScore   *int64 `json:"criticsScore"`
		AudienceScore  *int64 `json:"audienceScore"`
		CertifiedFresh *bool  `json:"certifiedFresh"`
	}{}
	hits := []rtAlgoliaHit{
		{Title: "Apex", ReleaseYear: 2021, Vanity: "apex_2021", RottenTomatoes: scored},
		{Title: "Apex", ReleaseYear: 2026, Vanity: "apex_2026", RottenTomatoes: scored},
		{Title: "APEX", ReleaseYear: 1994, Vanity: "apex", RottenTomatoes: scored},
		{Title: "Apex Predators", ReleaseYear: 2026, Vanity: "apex_predators"},
	}

	tests := []struct {
		name   string
		query  string
		year   int
		want   string
		wantOK bool
	}{
		{"year picks among same names", "Apex", 2026, "apex_2026", true},
		{"adjacent year still wins", "Apex", 2022, "apex_2021", true},
		{"no close title", "Something Else Entirely", 2026, "", false},
		{"alternate title", "Le Grand Film", 2020, "grand", true},
	}
	hits = append(hits, rtAlgoliaHit{Title: "The Big Film", Aka: []string{"Le Grand Film"}, ReleaseYear: 2020, Vanity: "grand", RottenTomatoes: scored})
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := bestRTHit(hits, tt.query, tt.year)
			if (got != nil) != tt.wantOK || (got != nil && got.Vanity != tt.want) {
				t.Fatalf("bestRTHit(%q, %d) = %+v, want %q", tt.query, tt.year, got, tt.want)
			}
		})
	}
}

func TestRTHitRating(t *testing.T) {
	critics, audience, certified := int64(68), int64(46), false
	h := rtAlgoliaHit{Title: "Apex", ReleaseYear: 2026, Vanity: "apex_2026"}
	h.RottenTomatoes = &struct {
		CriticsScore   *int64 `json:"criticsScore"`
		AudienceScore  *int64 `json:"audienceScore"`
		CertifiedFresh *bool  `json:"certifiedFresh"`
	}{&critics, &audience, &certified}

	movie := h.rating(RTKindMovie)
	if movie.Slug != "m/apex_2026" || movie.URL() != "https://www.rottentomatoes.com/m/apex_2026" || *movie.CriticsScore != 68 || *movie.AudienceScore != 46 || movie.CertifiedFresh == nil {
		t.Fatalf("movie rating = %+v", movie)
	}
	tv := h.rating(RTKindTV)
	if tv.Slug != "tv/apex_2026" || tv.CertifiedFresh != nil {
		t.Fatalf("tv rating = %+v (certified fresh is movie-only)", tv)
	}
}
