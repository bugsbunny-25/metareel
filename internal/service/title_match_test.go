package service

import (
	"reflect"
	"testing"

	"github.com/bugsbunny-25/metareel/internal/scraper/flixpatrol"
)

func TestPickCandidate(t *testing.T) {
	tests := []struct {
		name       string
		q          titleQuery
		candidates []titleCandidate
		wantID     string
		wantOK     bool
	}{
		{
			name: "year disambiguates same-name titles",
			q:    titleQuery{Names: []string{"Roommates"}, Year: 2026},
			candidates: []titleCandidate{
				{Title: "Roommates", Year: 1995, TmdbID: "old"},
				{Title: "Roommates", Year: 2026, TmdbID: "new"},
			},
			wantID: "new", wantOK: true,
		},
		{
			name: "conflicting year is rejected",
			q:    titleQuery{Names: []string{"Roommates"}, Year: 2026},
			candidates: []titleCandidate{
				{Title: "Roommates", Year: 1995, TmdbID: "old"},
			},
			wantOK: false,
		},
		{
			name: "adjacent year accepted when no exact match",
			q:    titleQuery{Names: []string{"Bugonia"}, Year: 2025},
			candidates: []titleCandidate{
				{Title: "Bugonia", Year: 2026, TmdbID: "1"},
			},
			wantID: "1", wantOK: true,
		},
		{
			name: "exact year preferred over adjacent year",
			q:    titleQuery{Names: []string{"Bugonia"}, Year: 2025},
			candidates: []titleCandidate{
				{Title: "Bugonia", Year: 2024, TmdbID: "near"},
				{Title: "Bugonia", Year: 2025, TmdbID: "exact"},
			},
			wantID: "exact", wantOK: true,
		},
		{
			name: "undated candidate is last resort",
			q:    titleQuery{Names: []string{"Apex"}, Year: 2026},
			candidates: []titleCandidate{
				{Title: "Apex", TmdbID: "undated"},
				{Title: "Apex", Year: 2027, TmdbID: "near"},
			},
			wantID: "near", wantOK: true,
		},
		{
			name: "unknown query year takes first name match",
			q:    titleQuery{Names: []string{"Beef"}},
			candidates: []titleCandidate{
				{Title: "Beef House", Year: 2020, TmdbID: "x"},
				{Title: "BEEF", Year: 2023, TmdbID: "beef"},
			},
			wantID: "beef", wantOK: true,
		},
		{
			name: "matches original title and ignores punctuation",
			q:    titleQuery{Names: []string{"Spider Man: No Way Home"}, Year: 2021},
			candidates: []titleCandidate{
				{Title: "Something Else", OriginalTitle: "Spider-Man - No Way Home", Year: 2021, TmdbID: "sm"},
			},
			wantID: "sm", wantOK: true,
		},
		{
			name: "falls back to secondary name",
			q:    titleQuery{Names: []string{"KPop Demon Hunters: The Movie", "KPop Demon Hunters"}, Year: 2025},
			candidates: []titleCandidate{
				{Title: "KPop Demon Hunters", Year: 2025, TmdbID: "k"},
			},
			wantID: "k", wantOK: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := pickCandidate(tt.q, tt.candidates)
			if ok != tt.wantOK || got.TmdbID != tt.wantID {
				t.Fatalf("pickCandidate() = (%q, %v), want (%q, %v)", got.TmdbID, ok, tt.wantID, tt.wantOK)
			}
		})
	}
}

func TestTitleQueryKinds(t *testing.T) {
	tests := []struct {
		name      string
		pageKind  flixpatrol.TitleKind
		chartKind flixpatrol.TitleKind
		want      []flixpatrol.TitleKind
	}{
		{"page unknown", "", flixpatrol.TitleKindTVShow, []flixpatrol.TitleKind{flixpatrol.TitleKindTVShow}},
		{"page agrees", flixpatrol.TitleKindMovie, flixpatrol.TitleKindMovie, []flixpatrol.TitleKind{flixpatrol.TitleKindMovie}},
		{"special in tv chart", flixpatrol.TitleKindMovie, flixpatrol.TitleKindTVShow, []flixpatrol.TitleKind{flixpatrol.TitleKindMovie, flixpatrol.TitleKindTVShow}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := titleQuery{Kind: tt.pageKind}.kinds(tt.chartKind)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("kinds() = %v, want %v", got, tt.want)
			}
		})
	}
}
