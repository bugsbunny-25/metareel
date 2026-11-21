// Package buildinfo holds the running server's version, stamped at build
// time into cmd/server's main.version and main.revision (Dockerfile, CI,
// Makefile) and handed over here at startup.
package buildinfo

import (
	"runtime"
	"runtime/debug"
)

var (
	version  = "dev"
	revision = ""
)

// Info is what GET /api/v1/version returns.
type Info struct {
	Version  string `json:"version"`
	Revision string `json:"revision,omitempty"`
	Go       string `json:"go"`
	// Features lists optional public API capabilities this server has, so
	// clients can tell what it supports without guessing from missing fields.
	Features []string `json:"features"`
}

// Features are the public API capabilities added over time (append only):
//   - chart_title_facts: chart rows carry tmdb_kind, backdrop_path, overview,
//     release_date, year, runtime, season_count and genres
//   - chart_ratings: chart endpoints accept include=ratings
//   - overview_sections: GET /titles/tmdb/{kind}/{id} accepts
//     include=rankings,timeline,releases and countries=
//   - list_overview: Trending and In cinemas items carry overview, and known
//     shows season_count
//   - title_logos: logo_path on chart rows, lists and TMDB metadata
//   - certifications: certification on chart rows (the chart's country) and
//     In cinemas, certifications per country in TMDB metadata
//   - provider_ids: tmdb_provider_id, chart_provider and logo_path on
//     services, chart providers, offers, offer events and rotation services
//   - person_seasons: GET /people/{id}?seasons=true adds filmography_seasons
var Features = []string{"chart_title_facts", "chart_ratings", "overview_sections", "list_overview",
	"title_logos", "certifications", "provider_ids", "person_seasons"}

// Set records the stamped version and revision; empty values keep the
// defaults ("dev", and the VCS revision Go embeds when built from a checkout).
func Set(v, rev string) {
	if v != "" {
		version = v
	}
	if rev != "" {
		revision = rev
	}
}

// Get returns the version, a 12-character revision, and the Go release.
func Get() Info {
	rev := revision
	if rev == "" {
		if bi, ok := debug.ReadBuildInfo(); ok {
			for _, s := range bi.Settings {
				if s.Key == "vcs.revision" {
					rev = s.Value
				}
			}
		}
	}
	if len(rev) > 12 {
		rev = rev[:12]
	}
	return Info{Version: version, Revision: rev, Go: runtime.Version(), Features: append([]string(nil), Features...)}
}
