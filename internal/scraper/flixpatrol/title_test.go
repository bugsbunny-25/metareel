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
			want: TitleDetails{Name: "Bugonia", TitleKind: TitleKindMovie, Year: 2025, Country: "United States"},
		},
		{
			path: "testdata/title_beef.html",
			want: TitleDetails{Name: "BEEF", TitleKind: TitleKindTVShow, Year: 2023, Country: "United States"},
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
