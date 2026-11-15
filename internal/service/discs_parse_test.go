package service

import (
	"reflect"
	"testing"
)

func TestParseProductTitle(t *testing.T) {
	tests := []struct {
		in        string
		title     string
		format    string
		packaging string
		edition   string
		boxSet    bool
		slip      bool
		label     string
		seasons   []int
	}{
		{in: "The Dark Knight (4K Ultra HD + Blu-ray + Digital)", title: "The Dark Knight", format: "uhd_bluray"},
		{in: "Dune: Part Two [4K UHD] Steelbook", title: "Dune: Part Two", format: "uhd_bluray", packaging: "steelbook", edition: "Steelbook"},
		{in: "The Dark Knight Trilogy Ultimate Collector's Edition (Blu-ray)", title: "The Dark Knight Trilogy", format: "bluray", packaging: "box_set", edition: "Ultimate Collector's Edition", boxSet: true},
		{in: "Breaking Bad: The Complete Series [Blu-ray]", title: "Breaking Bad", format: "bluray", packaging: "box_set", edition: "The Complete Series", boxSet: true},
		{in: "Game of Thrones: Season 3 (DVD)", title: "Game of Thrones", format: "dvd", edition: "Season 3", seasons: []int{3}},
		{in: "The Sopranos Seasons 1-6 Blu-ray", title: "The Sopranos", format: "bluray", packaging: "box_set", edition: "Seasons 1-6", boxSet: true, seasons: []int{1, 2, 3, 4, 5, 6}},
		{in: "Avatar 3D Blu-ray with Slipcover", title: "Avatar", format: "bluray_3d", slip: true},
		{in: "Seven Samurai (The Criterion Collection) [Blu-ray]", title: "Seven Samurai", format: "bluray", packaging: "box_set", boxSet: true, label: "criterion"},
		{in: "Blade Runner 2049", title: "Blade Runner 2049"},
		{in: "Dune 2-Film Collection (Blu-ray)  Action  Warner Bros.", title: "Dune 2-Film Collection", format: "bluray", packaging: "box_set", boxSet: true},
		{in: "Aliens Director's Cut DVD Widescreen", title: "Aliens", format: "dvd", edition: "Director's Cut"},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			sg := parseProductTitle(tt.in)
			if sg.Title != tt.title || sg.MediaFormat != tt.format || sg.Packaging != tt.packaging ||
				sg.Edition != tt.edition || sg.BoxSet != tt.boxSet || sg.HasSlipcover != tt.slip || sg.Label != tt.label {
				t.Errorf("got title=%q format=%q packaging=%q edition=%q box=%v slip=%v label=%q",
					sg.Title, sg.MediaFormat, sg.Packaging, sg.Edition, sg.BoxSet, sg.HasSlipcover, sg.Label)
			}
			want := tt.seasons
			if want == nil {
				want = []int{}
			}
			if !reflect.DeepEqual(sg.Seasons, want) {
				t.Errorf("seasons = %v, want %v", sg.Seasons, want)
			}
		})
	}
}

func TestNormalizeBarcode(t *testing.T) {
	for in, want := range map[string]string{
		"883929640010":    "0883929640010", // UPC-A gains its leading 0
		"0883929640010":   "0883929640010",
		"5051892230841":   "5051892230841",
		"5 051892 230841": "5051892230841",
		"00883929640010":  "0883929640010",
	} {
		got, err := NormalizeBarcode(in)
		if err != nil || got != want {
			t.Errorf("NormalizeBarcode(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, bad := range []string{"", "abc", "883929640017", "12345"} {
		if _, err := NormalizeBarcode(bad); err == nil {
			t.Errorf("NormalizeBarcode(%q) accepted", bad)
		}
	}
}
