package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/bugsbunny-25/metareel/internal/client"
	"github.com/bugsbunny-25/metareel/internal/repository"
)

// Cinema import (venues.import): every amenity=cinema OpenStreetMap has in a
// country becomes a venue, so people pick their cinema instead of typing it.
// A venue is matched by its OSM ref, then its Wikidata ID, then by name
// nearby (adopting cinemas entered by hand). Re-imports only fill fields that
// are still empty, never overwrite one, and never delete: a cinema that left
// OSM may still have watches.

// DefaultVenueCountries are imported when a run names none.
var DefaultVenueCountries = []string{"US", "GB", "IN"}

const (
	venueMatchMeters     = 300 // same name within this distance is the same cinema
	venueDuplicateMeters = 150 // one cinema mapped twice in OSM (node and building)
	venueBusyRetries     = 2
	venueBusyWait        = 30 * time.Second
	venueCountryGap      = 5 * time.Second
)

type cinemaSource interface {
	Cinemas(ctx context.Context, country string) ([]client.OSMPlace, error)
	Towns(ctx context.Context, country string) ([]client.OSMPlace, error)
}

// venueTownKm: a cinema without addr:city takes the nearest city or town
// node this close.
const venueTownKm = 20

// TimezoneFinder names the IANA zone at a point ("" if none).
type TimezoneFinder interface {
	GetTimezoneName(lng, lat float64) string
}

type venueImportStore interface {
	VenuesForImport(ctx context.Context, country string) ([]repository.ImportVenueRow, error)
	ApplyVenueImport(ctx context.Context, creates []repository.VenueImportCreate, fills []repository.VenueImportFill) (repository.VenueImportResult, error)
}

// VenueImporter imports cinemas from OpenStreetMap.
type VenueImporter struct {
	log       *slog.Logger
	src       cinemaSource
	store     venueImportStore
	tz        TimezoneFinder
	countries []string
	gap       time.Duration
	busyWait  time.Duration
	historic  historicSource // closed cinemas (Wikidata); nil = none
}

func NewVenueImporter(log *slog.Logger, src cinemaSource, store venueImportStore, tz TimezoneFinder, countries []string) *VenueImporter {
	if len(countries) == 0 {
		countries = DefaultVenueCountries
	}
	return &VenueImporter{log: log, src: src, store: store, tz: tz, countries: countries, gap: venueCountryGap, busyWait: venueBusyWait}
}

// VenueImportCountry is one country's outcome.
type VenueImportCountry struct {
	Country    string               `json:"country"`
	Fetched    int                  `json:"fetched"`    // cinemas OSM returned
	Skipped    int                  `json:"skipped"`    // unnamed or private
	Duplicates int                  `json:"duplicates"` // mapped twice in OSM, merged
	Created    int                  `json:"created"`
	Linked     int                  `json:"linked"` // existing venues matched by Wikidata ID or name, now carrying the OSM ref
	Filled     int                  `json:"filled"` // existing venues that gained an empty field
	Known      int                  `json:"known"`  // imported before and still in OSM
	Gone       int                  `json:"gone"`   // imported before, no longer in OSM (kept)
	Historic   *VenueImportHistoric `json:"historic,omitempty"`
	Error      string               `json:"error,omitempty"`
}

type VenueImportReport struct {
	Countries []VenueImportCountry `json:"countries"`
	Created   int                  `json:"created"`
	Linked    int                  `json:"linked"`
	Failed    int                  `json:"failed"` // countries
}

// Run imports each country (the configured ones when countries is empty).
// A country that fails is reported and the rest still run; the run fails
// only when every country failed.
func (s *VenueImporter) Run(ctx context.Context, countries []string, runLog func(level, msg string)) (*VenueImportReport, error) {
	if runLog == nil {
		runLog = func(string, string) {}
	}
	if len(countries) == 0 {
		countries = s.countries
	}
	rep := &VenueImportReport{Countries: []VenueImportCountry{}}
	var lastErr error
	for i, cc := range countries {
		if i > 0 && sleepCtx(ctx, s.gap) != nil {
			return rep, ctx.Err()
		}
		c, err := s.importCountry(ctx, strings.ToUpper(strings.TrimSpace(cc)), runLog)
		if err != nil {
			if ctx.Err() != nil {
				return rep, ctx.Err()
			}
			lastErr = err
			c.Error = err.Error()
			rep.Failed++
			runLog("error", fmt.Sprintf("%s: %v", c.Country, err))
			s.log.Warn("venue import failed", slog.String("country", c.Country), slog.Any("error", err))
		}
		rep.Countries = append(rep.Countries, c)
		rep.Created += c.Created
		rep.Linked += c.Linked
	}
	if rep.Failed == len(countries) && lastErr != nil {
		return rep, lastErr
	}
	return rep, nil
}

func (s *VenueImporter) importCountry(ctx context.Context, cc string, runLog func(level, msg string)) (VenueImportCountry, error) {
	out := VenueImportCountry{Country: cc}
	places, err := s.fetch(ctx, cc, runLog)
	if err != nil {
		return out, err
	}
	out.Fetched = len(places)
	var towns *townIndex
	if missingCity(places) || s.historic != nil {
		if s.gap > 0 && sleepCtx(ctx, s.gap) != nil {
			return out, ctx.Err()
		}
		if t, err := s.src.Towns(ctx, cc); err != nil {
			runLog("warn", fmt.Sprintf("%s: towns didn't load (%v); cinemas without an address get no city", cc, err))
		} else {
			towns = newTownIndex(t)
		}
	}
	existing, err := s.store.VenuesForImport(ctx, cc)
	if err != nil {
		return out, err
	}
	creates, fills, plan := s.plan(cc, places, existing, towns)
	out.Skipped, out.Duplicates, out.Linked, out.Known, out.Gone = plan.skipped, plan.duplicates, plan.linked, plan.known, plan.gone
	res, err := s.store.ApplyVenueImport(ctx, creates, fills)
	if err != nil {
		return out, err
	}
	out.Created, out.Filled = res.Created, res.Filled
	runLog("info", fmt.Sprintf("%s: %d cinemas in OpenStreetMap; %d new, %d matched to venues entered before, %d already imported, %d gained missing details, %d skipped (unnamed or private), %d duplicates merged, %d no longer in OSM (kept)",
		cc, out.Fetched, out.Created, out.Linked, out.Known, out.Filled, out.Skipped, out.Duplicates, out.Gone))
	if res.RefsSkipped > 0 {
		runLog("warn", fmt.Sprintf("%s: %d OSM or Wikidata IDs already belong to another venue and were not moved", cc, res.RefsSkipped))
	}
	if s.historic != nil {
		// Historic cinemas are extra: a failure leaves the current ones imported.
		if h, err := s.importHistoric(ctx, cc, towns, runLog); err != nil {
			if ctx.Err() != nil {
				return out, ctx.Err()
			}
			runLog("warn", fmt.Sprintf("%s: historic cinemas didn't import: %v", cc, err))
			out.Historic = &VenueImportHistoric{Error: err.Error()}
		} else {
			out.Historic = h
		}
	}
	return out, nil
}

// fetch asks Overpass, waiting and retrying while the instance is busy.
func (s *VenueImporter) fetch(ctx context.Context, cc string, runLog func(level, msg string)) ([]client.OSMPlace, error) {
	for attempt := 0; ; attempt++ {
		places, err := s.src.Cinemas(ctx, cc)
		if err == nil || !errors.Is(err, client.ErrOverpassBusy) || attempt == venueBusyRetries {
			return places, err
		}
		runLog("warn", fmt.Sprintf("%s: Overpass is busy (%v); retrying in %s", cc, err, s.busyWait))
		if sleepCtx(ctx, s.busyWait) != nil {
			return nil, ctx.Err()
		}
	}
}

type importPlan struct{ skipped, duplicates, linked, known, gone int }

// plan turns OSM places into creates and fills against the country's venues.
func (s *VenueImporter) plan(cc string, places []client.OSMPlace, existing []repository.ImportVenueRow, towns *townIndex) ([]repository.VenueImportCreate, []repository.VenueImportFill, importPlan) {
	var p importPlan
	// A Wikidata ID on several cinemas is the chain's, not a cinema's.
	qids := map[string]int{}
	for _, pl := range places {
		if q := wikidataTag(pl.Tags); q != "" {
			qids[q]++
		}
	}
	byOSM := map[string]*repository.ImportVenueRow{}
	byWikidata := map[string]*repository.ImportVenueRow{}
	byName := map[string][]*repository.ImportVenueRow{} // venues without an OSM ref, by normalised name
	for i := range existing {
		v := &existing[i]
		if v.OSMRef != "" {
			byOSM[v.OSMRef] = v
		} else {
			byName[normVenueName(v.Name)] = append(byName[normVenueName(v.Name)], v)
		}
		if v.WikidataRef != "" {
			byWikidata[v.WikidataRef] = v
		}
	}

	var creates []repository.VenueImportCreate
	var fills []repository.VenueImportFill
	seen := map[string]bool{} // OSM refs in this import
	taken := map[int64]bool{} // venues already matched in this run
	kept := map[string][]client.OSMPlace{}
	for _, pl := range places {
		v, ok := s.venueFromOSM(cc, pl)
		if !ok {
			p.skipped++
			continue
		}
		if v.City == nil && towns != nil {
			v.City = towns.nearest(pl.Lat, pl.Lon)
		}
		key := normVenueName(v.Name)
		if dupOf(kept[key], pl) {
			p.duplicates++
			continue
		}
		kept[key] = append(kept[key], pl)
		ref := pl.Ref()
		seen[ref] = true
		refs := []repository.VenueRef{{Source: "osm_id", Ref: ref}}
		wd := wikidataTag(pl.Tags)
		if qids[wd] > 1 || wd == strings.TrimSpace(pl.Tags["brand:wikidata"]) {
			wd = ""
		}
		if wd != "" {
			refs = append(refs, repository.VenueRef{Source: "wikidata_id", Ref: wd})
		}

		if ex := byOSM[ref]; ex != nil {
			taken[ex.ID] = true
			p.known++
			fills = append(fills, repository.VenueImportFill{ID: ex.ID, Venue: v, Refs: newRefs(ex, refs)})
			continue
		}
		var match *repository.ImportVenueRow
		if ex := byWikidata[wd]; wd != "" && ex != nil && ex.OSMRef == "" && !taken[ex.ID] {
			match = ex
		} else {
			match = nameMatch(byName[key], v, taken)
		}
		if match != nil {
			taken[match.ID] = true
			p.linked++
			fills = append(fills, repository.VenueImportFill{ID: match.ID, Venue: v, Refs: newRefs(match, refs)})
			continue
		}
		creates = append(creates, repository.VenueImportCreate{Venue: v, Refs: refs})
	}
	for ref := range byOSM {
		if !seen[ref] {
			p.gone++
		}
	}
	return creates, fills, p
}

func missingCity(places []client.OSMPlace) bool {
	for _, p := range places {
		if strings.TrimSpace(p.Tags["addr:city"]) == "" {
			return true
		}
	}
	return false
}

// townIndex finds the nearest city or town node, bucketed by half-degree cells.
type townIndex struct {
	cells map[[2]int][]town
}

type town struct {
	lat, lon float64
	name     string
	city     bool // place=city
}

// venueCityKm: a city this close wins over a nearer town (a cinema in Boston
// is nearer Brookline's town marker than Boston's city one).
const venueCityKm = 10

func townCell(lat, lon float64) [2]int {
	return [2]int{int(math.Floor(lat * 2)), int(math.Floor(lon * 2))}
}

func newTownIndex(places []client.OSMPlace) *townIndex {
	ix := &townIndex{cells: map[[2]int][]town{}}
	for _, p := range places {
		name := strings.TrimSpace(p.Tags["name"])
		if en := strings.TrimSpace(p.Tags["name:en"]); en != "" && !hasLatin(name) {
			name = en
		}
		if name == "" {
			continue
		}
		c := townCell(p.Lat, p.Lon)
		ix.cells[c] = append(ix.cells[c], town{p.Lat, p.Lon, clip(name, 200), p.Tags["place"] == "city"})
	}
	return ix
}

// nearest returns the closest city within venueCityKm, else the closest
// city or town within venueTownKm, or nil.
func (ix *townIndex) nearest(lat, lon float64) *string {
	c := townCell(lat, lon)
	city, cityM := "", float64(venueCityKm*1000)
	best, bestM := "", float64(venueTownKm*1000)
	for dy := -1; dy <= 1; dy++ {
		for dx := -1; dx <= 1; dx++ {
			for _, t := range ix.cells[[2]int{c[0] + dy, c[1] + dx}] {
				d := haversineM(lat, lon, t.lat, t.lon)
				if t.city && d <= cityM {
					city, cityM = t.name, d
				}
				if d <= bestM {
					best, bestM = t.name, d
				}
			}
		}
	}
	if city != "" {
		return &city
	}
	if best == "" {
		return nil
	}
	return &best
}

// newRefs drops the refs a venue already has.
func newRefs(ex *repository.ImportVenueRow, refs []repository.VenueRef) []repository.VenueRef {
	var out []repository.VenueRef
	for _, r := range refs {
		if (r.Source == "osm_id" && ex.OSMRef == r.Ref) || (r.Source == "wikidata_id" && ex.WikidataRef == r.Ref) {
			continue
		}
		out = append(out, r)
	}
	return out
}

// venueFromOSM maps an OSM cinema to a venue; false for one to skip.
func (s *VenueImporter) venueFromOSM(cc string, pl client.OSMPlace) (repository.Venue, bool) {
	t := pl.Tags
	if t["access"] == "private" || t["access"] == "no" {
		return repository.Venue{}, false
	}
	name := strings.TrimSpace(t["name"])
	// A name in a script other than Latin reads better in English, when OSM has one.
	if en := strings.TrimSpace(t["name:en"]); en != "" && !hasLatin(name) {
		name = en
	}
	if name == "" {
		return repository.Venue{}, false
	}
	lat, lon := pl.Lat, pl.Lon
	v := repository.Venue{Name: clip(name, 200), Country: &cc, Latitude: &lat, Longitude: &lon, Status: "open"}
	v.Chain = tagPtr(t, "brand")
	if addr := strings.TrimSpace(t["addr:full"]); addr != "" {
		v.Address = &addr
	} else if street := strings.TrimSpace(t["addr:street"]); street != "" {
		addr := strings.TrimSpace(strings.TrimSpace(t["addr:housenumber"]) + " " + street)
		v.Address = &addr
	}
	v.City = tagPtr(t, "addr:city")
	if v.Region = tagPtr(t, "addr:state"); v.Region == nil {
		v.Region = tagPtr(t, "addr:province")
	}
	if s.tz != nil {
		if tz := s.tz.GetTimezoneName(lon, lat); tz != "" {
			if _, err := time.LoadLocation(tz); err == nil {
				v.Timezone = &tz
			}
		}
	}
	for _, k := range []string{"start_date", "opening_date"} {
		if d := strings.TrimSpace(t[k]); fullDate(d) {
			v.OpenedOn = &d
			break
		}
	}
	return v, true
}

func tagPtr(t map[string]string, k string) *string {
	if v := strings.TrimSpace(t[k]); v != "" {
		v = clip(v, 200)
		return &v
	}
	return nil
}

func clip(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n])
	}
	return s
}

var wikidataQID = regexp.MustCompile(`^Q[1-9][0-9]*$`)

func wikidataTag(t map[string]string) string {
	if q := strings.TrimSpace(t["wikidata"]); wikidataQID.MatchString(q) {
		return q
	}
	return ""
}

// fullDate accepts only YYYY-MM-DD: a bare year would invent a day.
func fullDate(d string) bool {
	if len(d) != 10 {
		return false
	}
	_, err := time.Parse("2006-01-02", d)
	return err == nil
}

func hasLatin(s string) bool {
	for _, r := range s {
		if unicode.In(r, unicode.Latin) {
			return true
		}
	}
	return false
}

// normVenueName folds case, accents aside, and drops punctuation and spaces:
// "Prince Charles Cinema" and "prince-charles cinema" match.
func normVenueName(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// nameMatch picks the one venue with this name that is the same place:
// within venueMatchMeters, or, for a venue without coordinates, in the same
// city. Two candidates is ambiguous: no match.
func nameMatch(cands []*repository.ImportVenueRow, v repository.Venue, taken map[int64]bool) *repository.ImportVenueRow {
	var hit *repository.ImportVenueRow
	for _, c := range cands {
		if taken[c.ID] {
			continue
		}
		same := false
		switch {
		case c.Latitude != nil && c.Longitude != nil:
			same = haversineM(*c.Latitude, *c.Longitude, *v.Latitude, *v.Longitude) <= venueMatchMeters
		case c.City != nil && v.City != nil:
			same = normVenueName(*c.City) == normVenueName(*v.City)
		}
		if same {
			if hit != nil {
				return nil
			}
			hit = c
		}
	}
	return hit
}

// dupOf: OSM sometimes maps one cinema as a node and as its building.
func dupOf(kept []client.OSMPlace, pl client.OSMPlace) bool {
	for _, k := range kept {
		if haversineM(k.Lat, k.Lon, pl.Lat, pl.Lon) <= venueDuplicateMeters {
			return true
		}
	}
	return false
}

func haversineM(lat1, lon1, lat2, lon2 float64) float64 {
	const r = 6371000.0
	rad := math.Pi / 180
	dlat, dlon := (lat2-lat1)*rad, (lon2-lon1)*rad
	a := math.Sin(dlat/2)*math.Sin(dlat/2) + math.Cos(lat1*rad)*math.Cos(lat2*rad)*math.Sin(dlon/2)*math.Sin(dlon/2)
	return 2 * r * math.Asin(math.Sqrt(a))
}
