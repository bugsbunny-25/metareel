package service

import (
	"testing"

	"github.com/bugsbunny-25/metareel/internal/repository"
)

// TestClassifyOrigin uses real TMDB networks / production companies and
// Wikidata distributors of titles seen on the charts.
func TestClassifyOrigin(t *testing.T) {
	str := func(s string) *string { return &s }
	tv := func(networks []string, companies ...string) *repository.TmdbMetadata {
		return &repository.TmdbMetadata{Networks: networks, ProductionCompanies: companies}
	}
	film := func(year string, companies, distributors []string) *repository.TmdbMetadata {
		return &repository.TmdbMetadata{ReleaseDate: str(year + "-06-01"), ProductionCompanies: companies, Distributors: distributors}
	}
	cases := []struct {
		name, provider string
		movie          bool
		m              *repository.TmdbMetadata
		want, owner    string
	}{
		{"no metadata", "netflix", false, nil, OriginUnknown, ""},
		{"no companies at all", "netflix", true, film("2026", nil, nil), OriginUnknown, ""},
		{"unknown provider", "zee5", false, tv([]string{"HBO"}), OriginUnknown, "wbd"},

		// TV: the service's own network.
		{"Netflix series made by WBTV", "netflix", false, tv([]string{"Netflix"}, "Kaling International", "Warner Bros. Television"), OriginOriginal, "netflix"},
		{"HBO on HBO Max", "hbo-max", false, tv([]string{"HBO"}, "HBO", "Bastard Sword"), OriginOriginal, "wbd"},
		{"Max original", "hbo-max", false, tv([]string{"Max", "HBO Max"}, "Warner Bros. Television"), OriginOriginal, "wbd"},
		{"Apple series", "apple-tv", false, tv([]string{"Apple TV"}, "Doozer", "Warner Bros. Television"), OriginOriginal, "apple"},
		{"Prime series on two networks", "amazon-prime", false, tv([]string{"Netflix", "Prime Video"}), OriginOriginal, "amazon"},
		{"Peacock series", "peacock", false, tv([]string{"Peacock"}, "Media Res"), OriginOriginal, "nbcu"},
		{"Disney+ Marvel series", "disney", false, tv([]string{"Disney+"}, "Marvel Studios"), OriginOriginal, "disney"},

		// TV: the parent's channels.
		{"TLC on HBO Max", "hbo-max", false, tv([]string{"TLC", "discovery+"}, "Sharp Entertainment"), OriginInHouse, "wbd"},
		{"CBS on Paramount+", "paramount-plus", false, tv([]string{"CBS"}, "CBS Studios"), OriginInHouse, "paramount"},
		{"NBC on Peacock", "peacock", false, tv([]string{"NBC"}, "Universal Television"), OriginInHouse, "nbcu"},
		{"Nat Geo on Disney+", "disney", false, tv([]string{"National Geographic", "Nat Geo Wild"}), OriginInHouse, "disney"},
		{"Hulu show on Disney+", "disney", false, tv([]string{"Hulu"}), OriginInHouse, "disney"},

		// TV: licensed.
		{"CBS sitcom on Peacock", "peacock", false, tv([]string{"CBS"}, "Worldwide Pants", "HBO Independent Productions"), OriginLicensed, "paramount"},
		{"BBC on HBO Max", "hbo-max", false, tv([]string{"BBC One"}, "Mam Tor Productions"), OriginLicensed, ""},
		{"Korean drama on Netflix", "netflix", false, tv([]string{"SBS"}, "Studio S"), OriginLicensed, ""},

		// Films: the service's studio.
		{"Amazon MGM film", "amazon-prime", true, film("2026", []string{"Atlas Entertainment", "Amazon MGM Studios"}, []string{"Amazon MGM Studios", "Eagle Pictures"}), OriginOriginal, "amazon"},
		{"Apple Studios theatrical", "apple-tv", true, film("2025", []string{"Plan B Entertainment", "Apple Studios"}, []string{"Apple TV", "InterCom"}), OriginOriginal, "apple"},

		// Films: released by the service.
		{"Netflix film, indie producer", "netflix", true, film("2026", []string{"Happy Madison Productions"}, []string{"Netflix"}), OriginOriginal, "netflix"},
		{"Netflix film made by Sony Animation", "netflix", true, film("2025", []string{"Sony Pictures Animation"}, []string{"Netflix"}), OriginOriginal, "netflix"},
		{"Apple film with A24", "apple-tv", true, film("2025", []string{"A24", "40 Acres and a Mule Filmworks"}, []string{"A24", "Apple TV"}), OriginOriginal, "apple"},
		{"HBO Films", "hbo-max", true, film("2023", []string{"Seaview Productions"}, []string{"HBO Films", "Metropolitan Filmexport"}), OriginOriginal, "wbd"},

		// Films: a streamer among the distributors is not enough.
		{"Sony film Netflix carries", "netflix", true, film("2017", []string{"Matt Tolmach Productions", "Columbia Pictures"}, []string{"Columbia Pictures", "InterCom", "Netflix"}), OriginLicensed, "sony"},
		{"old film Netflix carries", "netflix", true, film("2005", []string{"Revolution Studios"}, []string{"MOKÉP", "Netflix"}), OriginLicensed, ""},
		{"Disney film, Netflix and Disney+ listed", "netflix", true, film("2004", []string{"BrownHouse Productions", "Walt Disney Pictures"}, []string{"Disney+", "InterCom", "Netflix"}), OriginLicensed, "disney"},
		{"Disney film on Disney+", "disney", true, film("2014", []string{"Walt Disney Pictures", "Roth Films"}, []string{"Disney+", "Fórum Hungary", "Netflix", "Walt Disney Studios Motion Pictures"}), OriginInHouse, "disney"},
		{"Universal film on Netflix", "netflix", true, film("2022", []string{"Universal Pictures", "Illumination"}, []string{"UIP-Dunafilm", "Universal Pictures"}), OriginLicensed, "nbcu"},

		// Films: the parent's studios.
		{"DreamWorks on Peacock", "peacock", true, film("2008", []string{"DreamWorks Animation"}, []string{"Netflix", "Paramount Pictures", "UIP-Dunafilm"}), OriginInHouse, "nbcu"},
		{"WB film by distributor", "hbo-max", true, film("2025", []string{"Clear Horizon Entertainment"}, []string{"Warner Bros. Pictures"}), OriginInHouse, "wbd"},
		{"Fox film on HBO Max", "hbo-max", true, film("2006", []string{"Fox 2000 Pictures", "Dune Entertainment"}, []string{"InterCom", "Netflix"}), OriginLicensed, "disney"},
		{"Paramount film on Paramount+", "paramount-plus", true, film("2022", []string{"Skydance Media", "Paramount Pictures"}, []string{"Paramount Pictures", "Paramount+"}), OriginInHouse, "paramount"},

		// No evidence: unknown rather than licensed.
		{"film without distributors or known studio", "netflix", true, film("2025", []string{"Propagate Content"}, nil), OriginUnknown, ""},
		{"Skydance is not an owner", "amazon-prime", true, film("2026", []string{"Skydance Media", "Reese Wernick Productions"}, nil), OriginUnknown, ""},
		{"show without network or known studio", "disney", false, tv(nil, "Evermotion"), OriginUnknown, ""},
		{"show without network, known studio", "disney", false, tv(nil, "Lucasfilm Animation"), OriginInHouse, "disney"},
		{"film without distributors, known studio", "netflix", true, film("2025", []string{"Columbia Pictures"}, nil), OriginLicensed, "sony"},
		{"Lionsgate film on Peacock", "peacock", true, film("2013", []string{"Summit Entertainment"}, []string{"Netflix", "Summit Entertainment"}), OriginLicensed, "lionsgate"},
		// Owner by distributor skips streamers: Universal, not Netflix.
		{"Universal film by distributor", "hbo-max", true, film("1999", []string{"Alphaville Films"}, []string{"Ivi.ru", "Netflix", "Universal Pictures"}), OriginLicensed, "nbcu"},
	}
	for _, c := range cases {
		got := ClassifyOrigin(c.provider, c.movie, c.m)
		if got.Origin != c.want || got.Owner != c.owner {
			t.Errorf("%s: got %+v, want origin %s owner %q", c.name, got, c.want, c.owner)
		}
		if got.Origin != OriginUnknown && got.Owner != "" && got.Reason == "" {
			t.Errorf("%s: no reason in %+v", c.name, got)
		}
	}
}

func TestBrandMatches(t *testing.T) {
	for _, c := range []struct {
		brand, name string
		want        bool
	}{
		{"Netflix*", "Netflix Animation", true},
		{"Netflix*", "netflix", true},
		{"Netflix*", "Net", false},
		{"ABC", "ABC Signature", false},
		{"ABC", " abc ", true},
		{"Universal Pictures*", "Universal Remote", false},
	} {
		if got := brandMatches(c.brand, c.name); got != c.want {
			t.Errorf("brandMatches(%q, %q) = %v", c.brand, c.name, got)
		}
	}
}
