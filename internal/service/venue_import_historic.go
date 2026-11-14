package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/bugsbunny-25/metareel/internal/client"
	"github.com/bugsbunny-25/metareel/internal/repository"
)

// Historic cinemas: the closed and demolished cinemas Wikidata records
// (largely from Cinema Treasures), so a watch decades ago can be logged at
// the cinema it happened in. They are imported closed, after the current
// cinemas of the country, matched the same way, and never reopen or replace
// a cinema OSM shows as open.

type historicSource interface {
	CountryQID(ctx context.Context, iso string) (string, error)
	ClosedCinemas(ctx context.Context, countryQID string) ([]client.WikidataCinema, string, error)
}

// WithHistoric adds closed cinemas from Wikidata to each country's import.
func (s *VenueImporter) WithHistoric(src historicSource) *VenueImporter {
	s.historic = src
	return s
}

// VenueImportHistoric is one country's historic-cinema outcome.
type VenueImportHistoric struct {
	Fetched int    `json:"fetched"` // closed cinemas on Wikidata
	Skipped int    `json:"skipped"` // no name or no coordinates
	Created int    `json:"created"`
	Linked  int    `json:"linked"` // matched to a venue by name nearby, now carrying the Wikidata ID
	Known   int    `json:"known"`  // already had the Wikidata ID
	Filled  int    `json:"filled"`
	Note    string `json:"note,omitempty"`
	Error   string `json:"error,omitempty"`
}

func (s *VenueImporter) importHistoric(ctx context.Context, cc string, towns *townIndex, runLog func(level, msg string)) (*VenueImportHistoric, error) {
	qid, err := s.historic.CountryQID(ctx, cc)
	if err != nil {
		return nil, err
	}
	items, note, err := s.historic.ClosedCinemas(ctx, qid)
	if err != nil {
		return nil, err
	}
	out := &VenueImportHistoric{Fetched: len(items), Note: note}
	existing, err := s.store.VenuesForImport(ctx, cc)
	if err != nil {
		return nil, err
	}
	creates, fills := s.planHistoric(cc, items, existing, towns, out)
	res, err := s.store.ApplyVenueImport(ctx, creates, fills)
	if err != nil {
		return nil, err
	}
	out.Created, out.Filled = res.Created, res.Filled
	runLog("info", fmt.Sprintf("%s: %d closed cinemas on Wikidata; %d new, %d matched to venues by name, %d already imported, %d gained missing details, %d skipped (no name or location)",
		cc, out.Fetched, out.Created, out.Linked, out.Known, out.Filled, out.Skipped))
	if note != "" {
		runLog("warn", fmt.Sprintf("%s: %s", cc, note))
	}
	return out, nil
}

func (s *VenueImporter) planHistoric(cc string, items []client.WikidataCinema, existing []repository.ImportVenueRow, towns *townIndex, out *VenueImportHistoric) ([]repository.VenueImportCreate, []repository.VenueImportFill) {
	byWD := map[string]*repository.ImportVenueRow{}
	byName := map[string][]*repository.ImportVenueRow{} // venues without a Wikidata ID
	for i := range existing {
		v := &existing[i]
		if v.WikidataRef != "" {
			byWD[v.WikidataRef] = v
		} else {
			byName[normVenueName(v.Name)] = append(byName[normVenueName(v.Name)], v)
		}
	}
	var creates []repository.VenueImportCreate
	var fills []repository.VenueImportFill
	taken := map[int64]bool{}
	for _, it := range items {
		v, ok := s.venueFromWikidata(cc, it, towns)
		if !ok {
			out.Skipped++
			continue
		}
		ref := []repository.VenueRef{{Source: "wikidata_id", Ref: it.QID}}
		match := byWD[it.QID]
		if match != nil {
			out.Known++
		} else if match = nameMatch(byName[normVenueName(v.Name)], v, taken); match != nil {
			out.Linked++
		}
		if match != nil {
			taken[match.ID] = true
			if match.Status != "closed" {
				// OSM shows it open (rebuilt or reopened): only the opening date and address help.
				v = repository.Venue{Address: v.Address, OpenedOn: v.OpenedOn}
			}
			fills = append(fills, repository.VenueImportFill{ID: match.ID, Venue: v, Refs: newRefs(match, ref)})
			continue
		}
		creates = append(creates, repository.VenueImportCreate{Venue: v, Refs: ref})
	}
	return creates, fills
}

// venueFromWikidata maps a closed cinema; false without a name or position.
func (s *VenueImporter) venueFromWikidata(cc string, it client.WikidataCinema, towns *townIndex) (repository.Venue, bool) {
	name := strings.TrimSpace(it.Name)
	if name == "" || !it.HasCoord || !wikidataQID.MatchString(it.QID) {
		return repository.Venue{}, false
	}
	lat, lon := it.Lat, it.Lon
	v := repository.Venue{Name: clip(name, 200), Country: &cc, Latitude: &lat, Longitude: &lon, Status: "closed"}
	if it.Address != "" {
		a := clip(it.Address, 300)
		v.Address = &a
	}
	if towns != nil {
		v.City = towns.nearest(lat, lon)
	}
	if s.tz != nil {
		if tz := s.tz.GetTimezoneName(lon, lat); tz != "" {
			if _, err := time.LoadLocation(tz); err == nil {
				v.Timezone = &tz
			}
		}
	}
	if it.Opened.Full() {
		d := string(it.Opened)
		v.OpenedOn = &d
	}
	if it.Closed.Full() {
		d := string(it.Closed)
		v.ClosedOn = &d
	}
	// Dates Wikidata only knows to the year (or month) go in the note rather than invent a day.
	var parts []string
	if it.Opened != "" && !it.Opened.Full() {
		parts = append(parts, "opened "+string(it.Opened))
	}
	if it.Closed != "" && !it.Closed.Full() {
		parts = append(parts, "closed "+string(it.Closed))
	}
	if it.Demolished {
		parts = append(parts, "demolished")
	}
	if len(parts) > 0 {
		n := strings.ToUpper(parts[0][:1]) + parts[0][1:]
		if len(parts) > 1 {
			n += ", " + strings.Join(parts[1:], ", ")
		}
		n += " (Wikidata)"
		v.Note = &n
	}
	return v, true
}
