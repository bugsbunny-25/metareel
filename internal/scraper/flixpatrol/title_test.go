package flixpatrol

import (
	"reflect"
	"testing"
)

func TestParseTitleDetails_FromSamples(t *testing.T) {
	tests := []struct {
		path string
		want TitleDetails
	}{
		{
			path: "testdata/title_bugonia.html",
			want: TitleDetails{Name: "Bugonia", TitleKind: TitleKindMovie, Year: 2025, Premiere: "2025-10-24", Country: "United States"},
		},
		{
			path: "testdata/title_beef.html",
			want: TitleDetails{Name: "BEEF", TitleKind: TitleKindTVShow, Year: 2023, Premiere: "2023-04-06", Country: "United States"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			doc := mustLoadDocument(t, tt.path)
			got, err := parseTitleDetails(doc)
			if err != nil {
				t.Fatalf("parseTitleDetails error: %v", err)
			}
			if !reflect.DeepEqual(*got, tt.want) {
				t.Fatalf("got %+v, want %+v", *got, tt.want)
			}
		})
	}
}

func TestYearFromSlug(t *testing.T) {
	tests := map[string]int{
		"roommates-2026":         2026,
		"raw-1993":               1993,
		"bugonia":                0,
		"180":                    0,
		"2012":                   0,
		"blade-runner-2049-2017": 2017,
	}
	for slug, want := range tests {
		if got := YearFromSlug(slug); got != want {
			t.Errorf("YearFromSlug(%q) = %d, want %d", slug, got, want)
		}
	}
}

func TestSeasonFromName(t *testing.T) {
	tests := []struct {
		in     string
		name   string
		season int
	}{
		{"Wednesday: Season 2", "Wednesday", 2},
		{"Squid Game - Season 3", "Squid Game", 3},
		{"Stranger Things Season 5", "Stranger Things", 5},
		{"The Four Seasons", "The Four Seasons", 0},
		{"Season 2", "Season 2", 0},
		{"Bugonia", "Bugonia", 0},
	}
	for _, tt := range tests {
		name, season := SeasonFromName(tt.in)
		if name != tt.name || season != tt.season {
			t.Errorf("SeasonFromName(%q) = %q, %d; want %q, %d", tt.in, name, season, tt.name, tt.season)
		}
	}
}
