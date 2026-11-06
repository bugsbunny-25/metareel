package service

import (
	"strconv"
	"strings"

	"github.com/bugsbunny-25/metareel/internal/repository"
)

// Origins of a title on a service's chart (computed at read time from TMDB
// networks and production companies and Wikidata distributors, so rule
// changes apply to all stored charts).
const (
	// OriginOriginal: made for the service: its own network (TV), its own
	// studio, or (movies) released by it with no theatrical studio involved.
	OriginOriginal = "original"
	// OriginInHouse: from the service's parent company (its film studios and
	// linear channels), e.g. a Marvel film on Disney+ or a CBS show on
	// Paramount+.
	OriginInHouse = "in_house"
	// OriginLicensed: from another company (or an independent).
	OriginLicensed = "licensed"
	// OriginUnknown: no TMDB metadata yet, a service without rules, or
	// nothing says who made the title (no network / distributor and no known
	// studio).
	OriginUnknown = "unknown"
)

// Origins lists the origin values in display order.
var Origins = []string{OriginOriginal, OriginInHouse, OriginLicensed, OriginUnknown}

// OriginInfo says where a title on a service's chart comes from.
type OriginInfo struct {
	Origin string `json:"origin"`
	// Owner is the media group the title belongs to (netflix, amazon, apple,
	// disney, wbd, nbcu, paramount, sony, lionsgate, a24); "" = independent
	// or not recognised.
	Owner string `json:"owner,omitempty"`
	// Reason is the brand that decided it, e.g. "network: HBO",
	// "studio: Amazon MGM Studios", "distributor: Netflix".
	Reason string `json:"reason,omitempty"`
}

// mediaGroup is a company with its brands. A brand ending in "*" matches
// every name starting with it; others match the whole name (both ignoring
// case).
type mediaGroup struct {
	key, name string
	// originals: each FlixPatrol provider slug of the group with the brands
	// of its own originals (networks, studios, distributor names).
	originals map[string][]string
	// since is the first year a film can be one of the group's streaming
	// originals (for the distributor rule).
	since int
	// studios: the group's other brands (film / TV studios, channels).
	studios []string
	// major: its studios release films theatrically; as distributors they
	// mean a film is not a streaming original.
	major bool
}

var mediaGroups = []mediaGroup{
	{key: "netflix", name: "Netflix", since: 2015,
		originals: map[string][]string{"netflix": {"Netflix*"}}},
	{key: "amazon", name: "Amazon", since: 2015,
		originals: map[string][]string{"amazon-prime": {"Prime Video", "Amazon*", "Freevee", "IMDb TV"}},
		studios: []string{"Metro-Goldwyn-Mayer*", "MGM", "MGM Television", "MGM+*", "Epix", "United Artists*",
			"Orion Pictures", "American International Pictures"}, major: true},
	{key: "apple", name: "Apple", since: 2019,
		originals: map[string][]string{"apple-tv": {"Apple TV", "Apple TV+", "Apple Studios", "Apple Original Films", "Apple"}}},
	{key: "disney", name: "Disney", since: 2019,
		originals: map[string][]string{"disney": {"Disney+"}, "hulu": {"Hulu", "FX on Hulu"}},
		studios: []string{"Walt Disney*", "Disney Channel", "Disney Junior", "Disney XD", "Disney Television Animation",
			"Disneynature", "Pixar*", "Marvel*", "Lucasfilm*", "20th Century*", "Twentieth Century*", "20th Television*",
			"Searchlight Pictures", "Fox Searchlight Pictures", "Fox 2000 Pictures", "Touchstone*", "Hollywood Pictures",
			"Blue Sky Studios", "ABC", "ABC Signature", "ABC Studios", "FX", "FXX", "FX Productions", "Freeform",
			"National Geographic*", "Nat Geo*", "ESPN*"}, major: true},
	{key: "wbd", name: "Warner Bros. Discovery", since: 2020,
		originals: map[string][]string{"hbo-max": {"HBO*", "Max", "Max Original"}},
		studios: []string{"Warner Bros*", "New Line Cinema", "DC Studios", "DC Entertainment", "DC Films",
			"Castle Rock Entertainment", "Turner*", "TNT", "TBS", "truTV", "CNN*", "Cartoon Network*", "Adult Swim",
			"Williams Street", "Hanna-Barbera*", "Discovery*", "discovery+", "TLC", "HGTV", "Food Network",
			"Investigation Discovery", "Animal Planet", "Travel Channel", "OWN", "Magnolia Network"}, major: true},
	{key: "nbcu", name: "NBCUniversal", since: 2020,
		originals: map[string][]string{"peacock": {"Peacock"}},
		studios: []string{"NBC", "NBC News", "NBCUniversal*", "Bravo", "USA Network", "Syfy", "E!", "Oxygen",
			"Telemundo*", "CNBC", "MSNBC", "Universal Pictures*", "Universal Television", "Universal Studios*",
			"Universal Media Studios", "Universal Content Productions", "Universal Animation Studios",
			"Universal International Studios", "Focus Features", "Illumination*", "DreamWorks Animation*",
			"Working Title*", "UIP*", "United International Pictures"}, major: true},
	{key: "paramount", name: "Paramount Skydance", since: 2014,
		originals: map[string][]string{"paramount-plus": {"Paramount+*", "CBS All Access"}},
		studios: []string{"Paramount*", "CBS", "CBS Studios", "CBS Television Studios", "CBS Films", "CBS News",
			"CBS Productions", "Showtime*", "Nickelodeon*", "Nick Jr.", "MTV*", "Comedy Central", "BET*", "TV Land", "CMT",
			"Pluto TV", "Smithsonian Channel", "Spike"}, major: true},
	{key: "sony", name: "Sony", since: 2017,
		originals: map[string][]string{"crunchyroll": {"Crunchyroll"}},
		studios: []string{"Sony Pictures*", "Columbia Pictures", "TriStar Pictures", "Screen Gems", "Affirm Films",
			"Stage 6 Films", "Destination Films", "Aniplex", "Funimation"}, major: true},
	{key: "lionsgate", name: "Lionsgate", major: true,
		studios: []string{"Lionsgate*", "Lions Gate*", "Summit Entertainment"}},
	{key: "a24", name: "A24", studios: []string{"A24"}},
}

// MediaGroupName returns a group's display name ("" for unknown keys).
func MediaGroupName(key string) string {
	for _, g := range mediaGroups {
		if g.key == key {
			return g.name
		}
	}
	return ""
}

func brandMatches(brand, name string) bool {
	name = strings.TrimSpace(name)
	if p, ok := strings.CutSuffix(brand, "*"); ok {
		return len(name) >= len(p) && strings.EqualFold(name[:len(p)], p)
	}
	return strings.EqualFold(name, brand)
}

func anyBrand(brands []string, name string) bool {
	for _, b := range brands {
		if brandMatches(b, name) {
			return true
		}
	}
	return false
}

// providerGroup returns the group of a FlixPatrol provider and its own
// original brands.
func providerGroup(provider string) (*mediaGroup, []string) {
	for i := range mediaGroups {
		if brands, ok := mediaGroups[i].originals[provider]; ok {
			return &mediaGroups[i], brands
		}
	}
	return nil, nil
}

// ownerOf returns the group a name belongs to: with streaming = true any of
// its brands (originals included), else only its studios and channels.
func ownerOf(name string, streaming bool) *mediaGroup {
	for i := range mediaGroups {
		g := &mediaGroups[i]
		if anyBrand(g.studios, name) {
			return g
		}
		if streaming {
			for _, brands := range g.originals {
				if anyBrand(brands, name) {
					return g
				}
			}
		}
	}
	return nil
}

// ClassifyOrigin tells where a title on a provider's chart comes from. m is
// its TMDB metadata (nil = unknown); movie selects the rules for films.
func ClassifyOrigin(provider string, movie bool, m *repository.TmdbMetadata) OriginInfo {
	netflixOriginal := m != nil && m.NetflixPremiere != nil && *m.NetflixPremiere != ""
	if m == nil || (len(m.Networks)+len(m.ProductionCompanies)+len(m.Distributors) == 0 && !netflixOriginal) {
		return OriginInfo{Origin: OriginUnknown}
	}
	group, own := providerGroup(provider)

	// Netflix's own engagement reports date the premiere of its originals.
	if netflixOriginal && group != nil && group.key == "netflix" {
		return OriginInfo{Origin: OriginOriginal, Owner: "netflix", Reason: "Netflix engagement report: premiered " + *m.NetflixPremiere}
	}

	if group != nil {
		// Made for the service: its network (TV), else its studio.
		if !movie {
			for _, n := range m.Networks {
				if anyBrand(own, n) {
					return OriginInfo{Origin: OriginOriginal, Owner: group.key, Reason: "network: " + n}
				}
			}
		}
		if movie || len(m.Networks) == 0 {
			for _, c := range m.ProductionCompanies {
				if anyBrand(own, c) {
					return OriginInfo{Origin: OriginOriginal, Owner: group.key, Reason: "studio: " + c}
				}
			}
		}
		// Films the service released itself: it is a distributor, no
		// theatrical studio or other streamer is, and the film is not older
		// than the service's originals (Wikidata lists streamers that carry
		// licensed films as distributors too).
		if movie && releaseYear(m) >= group.since {
			if d, ok := ownDistributor(m.Distributors, own); ok {
				return OriginInfo{Origin: OriginOriginal, Owner: group.key, Reason: "distributor: " + d}
			}
		}
	}

	owner, reason := titleOwner(movie, m)
	if owner == nil && netflixOriginal {
		owner, reason = netflixGroup(), "Netflix engagement report: premiered "+*m.NetflixPremiere
	}
	switch {
	case owner == nil && !hasSource(movie, m):
		// Nothing says who made or carries it (a film Wikidata has no
		// distributors for, a show without a network): don't guess licensed.
		return OriginInfo{Origin: OriginUnknown}
	case group == nil:
		return OriginInfo{Origin: OriginUnknown, Owner: ownerKey(owner), Reason: reason}
	case owner == group:
		return OriginInfo{Origin: OriginInHouse, Owner: owner.key, Reason: reason}
	default:
		return OriginInfo{Origin: OriginLicensed, Owner: ownerKey(owner), Reason: reason}
	}
}

// hasSource reports whether the title has its decisive source: a network
// (TV) or Wikidata distributors (films).
func hasSource(movie bool, m *repository.TmdbMetadata) bool {
	if movie {
		return len(m.Distributors) > 0
	}
	return len(m.Networks) > 0
}

func netflixGroup() *mediaGroup {
	for i := range mediaGroups {
		if mediaGroups[i].key == "netflix" {
			return &mediaGroups[i]
		}
	}
	return nil
}

func ownerKey(g *mediaGroup) string {
	if g == nil {
		return ""
	}
	return g.key
}

// ownDistributor returns the service's brand among a film's distributors
// when none of the others is a major studio or another streamer.
func ownDistributor(distributors, own []string) (string, bool) {
	found := ""
	for _, d := range distributors {
		if anyBrand(own, d) {
			if found == "" {
				found = d
			}
			continue
		}
		if g := ownerOf(d, true); g != nil && (g.major || len(g.originals) > 0) {
			return "", false
		}
	}
	return found, found != ""
}

// titleOwner finds the group a title belongs to: TV by network then
// studio; films by studio, then distributor (studios only: streamers that
// carry a film are not its owner).
func titleOwner(movie bool, m *repository.TmdbMetadata) (*mediaGroup, string) {
	if !movie {
		for _, n := range m.Networks {
			if g := ownerOf(n, true); g != nil {
				return g, "network: " + n
			}
		}
	}
	for _, c := range m.ProductionCompanies {
		if g := ownerOf(c, true); g != nil {
			return g, "studio: " + c
		}
	}
	for _, d := range m.Distributors {
		if g := ownerOf(d, false); g != nil {
			return g, "distributor: " + d
		}
	}
	return nil, ""
}

func releaseYear(m *repository.TmdbMetadata) int {
	if m.ReleaseDate == nil || len(*m.ReleaseDate) < 4 {
		return 0
	}
	y, _ := strconv.Atoi((*m.ReleaseDate)[:4])
	return y
}
