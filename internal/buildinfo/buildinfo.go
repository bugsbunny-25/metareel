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
}

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
	return Info{Version: version, Revision: rev, Go: runtime.Version()}
}
