package service

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/bugsbunny-25/metareel/internal/client"
	"github.com/bugsbunny-25/metareel/internal/repository"
)

type fakeOSM struct {
	places map[string][]client.OSMPlace
	towns  map[string][]client.OSMPlace
	busy   int // answer ErrOverpassBusy this many times first
	fail   map[string]bool
	calls  int
}

func (f *fakeOSM) Towns(_ context.Context, cc string) ([]client.OSMPlace, error) {
	return f.towns[cc], nil
}

func (f *fakeOSM) Cinemas(_ context.Context, cc string) ([]client.OSMPlace, error) {
	f.calls++
	if f.busy > 0 {
		f.busy--
		return nil, client.ErrOverpassBusy
	}
	if f.fail[cc] {
		return nil, errors.New("overpass status 400")
	}
	return f.places[cc], nil
}

type fakeTZ map[string]string // country by rough longitude sign is enough here

func (fakeTZ) GetTimezoneName(lng, lat float64) string {
	switch {
	case lng > 60:
		return "Asia/Kolkata"
	case lng > -20:
		return "Europe/London"
	case lng < -100:
		return "America/Los_Angeles"
	}
	return ""
}

func osm(typ string, id int64, lat, lon float64, tags ...string) client.OSMPlace {
	t := map[string]string{"amenity": "cinema"}
	for i := 0; i+1 < len(tags); i += 2 {
		t[tags[i]] = tags[i+1]
	}
	return client.OSMPlace{Type: typ, ID: id, Lat: lat, Lon: lon, Tags: t}
}

func TestVenueImport(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	repo := repository.NewTrackingRepository(db)

	// Entered by hand before any import: two to adopt, two that share a name.
	pcc, _ := repo.CreateVenue(ctx, repository.Venue{Name: "Prince Charles Cinema", City: str("London"), Country: str("GB"), Timezone: str("Europe/London"), Status: "open"}, 0)
	bfi, _ := repo.CreateVenue(ctx, repository.Venue{Name: "BFI Southbank (NFT)", Country: str("GB"), Status: "open"}, 0)
	if err := repo.SaveVenueRef(ctx, bfi, repository.VenueRef{Source: "wikidata_id", Ref: "Q4835803"}); err != nil {
		t.Fatal(err)
	}
	odeonA, _ := repo.CreateVenue(ctx, repository.Venue{Name: "Odeon", City: str("Leeds"), Country: str("GB"), Status: "open"}, 0)
	odeonB, _ := repo.CreateVenue(ctx, repository.Venue{Name: "ODEON", City: str("Leeds"), Country: str("GB"), Status: "open"}, 0)

	src := &fakeOSM{places: map[string][]client.OSMPlace{
		"GB": {
			osm("node", 1, 51.5113, -0.1303, "name", "Prince Charles Cinema", "addr:city", "London", "addr:street", "Leicester Place", "addr:housenumber", "7", "start_date", "1991"),
			osm("way", 2, 51.5065, -0.1155, "name", "BFI Southbank", "wikidata", "Q4835803", "operator", "BFI"),
			osm("node", 3, 53.7997, -1.5492, "name", "Odeon", "addr:city", "Leeds", "brand", "Odeon"),
			osm("node", 4, 51.5, -0.12, "amenity", "cinema"),                            // unnamed
			osm("node", 5, 51.52, -0.10, "name", "Screening Room", "access", "private"), // private
			osm("node", 6, 51.4613, -0.1156, "name", "Ritzy Picturehouse", "brand", "Picturehouse", "opening_date", "1911-05-01", "addr:city", "London"),
			osm("way", 7, 51.4614, -0.1157, "name", "Ritzy Picturehouse"), // same cinema as its building
			// A chain's Wikidata ID tagged on its branches is not either branch's.
			osm("node", 8, 52.4797, -1.8970, "name", "Vue Birmingham", "wikidata", "Q7943543", "addr:city", "Birmingham"),
			osm("node", 9, 53.4808, -2.2426, "name", "Vue Manchester", "wikidata", "Q7943543", "addr:city", "Manchester"),
			osm("node", 12, 51.38, -2.36, "name", "Avenue Cinema", "addr:city", "Bath"),
			osm("node", 11, 55.9533, -3.1883, "name", "Cameo", "wikidata", "Q5025910", "brand:wikidata", "Q5025910", "addr:city", "Edinburgh"),
		},
		"IN": {
			osm("node", 10, 19.0, 72.83, "name", "पीवीआर", "name:en", "PVR Phoenix", "brand", "PVR", "addr:state", "Maharashtra"),
		},
	}, towns: map[string][]client.OSMPlace{
		"GB": {osm("node", 100, 51.5072, -0.1276, "name", "London", "place", "city"), osm("node", 101, 53.8008, -1.5491, "name", "Leeds", "place", "city")},
		"IN": {osm("node", 102, 19.0760, 72.8777, "name", "मुंबई", "name:en", "Mumbai", "place", "city")},
	}}
	imp := NewVenueImporter(slog.New(slog.NewTextHandler(io.Discard, nil)), src, repo, fakeTZ{}, []string{"GB", "IN"})
	imp.gap, imp.busyWait = 0, 0

	var logs []string
	rep, err := imp.Run(ctx, nil, func(_, m string) { logs = append(logs, m) })
	if err != nil {
		t.Fatal(err)
	}
	gb := rep.Countries[0]
	// Created: Odeon (ambiguous by name), Ritzy. Linked: Prince Charles (name + city), BFI (Wikidata).
	if gb.Fetched != 11 || gb.Skipped != 2 || gb.Duplicates != 1 || gb.Created != 6 || gb.Linked != 2 || gb.Known != 0 {
		t.Fatalf("GB = %+v", gb)
	}
	if in := rep.Countries[1]; in.Created != 1 || rep.Created != 7 || rep.Linked != 2 {
		t.Fatalf("IN = %+v, report %+v", in, rep)
	}
	if len(logs) != 2 || !strings.Contains(logs[0], "GB: 11 cinemas in OpenStreetMap; 6 new, 2 matched") {
		t.Fatalf("logs = %q", logs)
	}

	// The hand-entered cinema keeps its name and zone and gains OSM's details.
	v, err := repo.Venue(ctx, pcc)
	if err != nil {
		t.Fatal(err)
	}
	if v.Name != "Prince Charles Cinema" || *v.Address != "7 Leicester Place" || *v.Latitude != 51.5113 || *v.Timezone != "Europe/London" || v.OpenedOn != nil {
		t.Fatalf("adopted = %+v", v)
	}
	refs, _ := repo.VenueRefs(ctx, pcc)
	if len(refs) != 1 || refs[0] != (repository.VenueRef{Source: "osm_id", Ref: "node/1"}) {
		t.Fatalf("refs = %+v", refs)
	}
	// No addr:city in OSM: the nearest town.
	if v, _ := repo.Venue(ctx, bfi); v.Name != "BFI Southbank (NFT)" || v.Latitude == nil || v.City == nil || *v.City != "London" {
		t.Fatalf("bfi = %+v", v)
	}
	for _, id := range []int64{odeonA, odeonB} {
		if refs, _ := repo.VenueRefs(ctx, id); len(refs) != 0 {
			t.Fatalf("ambiguous Odeon %d linked: %+v", id, refs)
		}
	}
	all, _ := repo.VenuesForImport(ctx, "GB")
	var ritzy *repository.ImportVenueRow
	for i := range all {
		if all[i].Name == "Ritzy Picturehouse" {
			ritzy = &all[i]
		}
	}
	if ritzy == nil || ritzy.OSMRef != "node/6" {
		t.Fatalf("ritzy = %+v", ritzy)
	}
	rv, _ := repo.Venue(ctx, ritzy.ID)
	if *rv.Chain != "Picturehouse" || *rv.OpenedOn != "1911-05-01" || *rv.Timezone != "Europe/London" || *rv.City != "London" || *rv.Country != "GB" || rv.Status != "open" {
		t.Fatalf("ritzy venue = %+v", rv)
	}
	inRows, _ := repo.VenuesForImport(ctx, "IN")
	if len(inRows) != 1 || inRows[0].Name != "PVR Phoenix" {
		t.Fatalf("IN rows = %+v", inRows)
	}
	if pv, _ := repo.Venue(ctx, inRows[0].ID); *pv.Timezone != "Asia/Kolkata" || *pv.Region != "Maharashtra" || *pv.City != "Mumbai" {
		t.Fatalf("pvr = %+v", pv)
	}

	for _, name := range []string{"Vue Birmingham", "Vue Manchester", "Cameo"} {
		for _, r := range all {
			if r.Name == name && r.WikidataRef != "" {
				t.Fatalf("%s took the chain's Wikidata ID %s", name, r.WikidataRef)
			}
		}
	}

	// An admin edits the Ritzy; a re-run keeps the edit, adds nothing and
	// counts the cinema OSM dropped.
	rv.Chain = str("Picturehouse Cinemas")
	if _, err := repo.UpdateVenue(ctx, rv); err != nil {
		t.Fatal(err)
	}
	var kept []client.OSMPlace // without the Odeon (node 3) and the duplicate building (way 7)
	for _, pl := range src.places["GB"] {
		if pl.Ref() != "node/3" && pl.Ref() != "way/7" {
			kept = append(kept, pl)
		}
	}
	src.places["GB"] = kept
	rep, err = imp.Run(ctx, []string{"gb"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if gb := rep.Countries[0]; gb.Created != 0 || gb.Linked != 0 || gb.Known != 7 || gb.Filled != 0 || gb.Gone != 1 {
		t.Fatalf("re-run = %+v", gb)
	}
	if rv, _ := repo.Venue(ctx, ritzy.ID); *rv.Chain != "Picturehouse Cinemas" {
		t.Fatalf("admin edit overwritten: %+v", rv)
	}

	// Search: every word in the name, chain, city or address; names that
	// start with the first word lead.
	found, err := repo.ListVenues(ctx, repository.VenueFilter{Query: "picturehouse, london", Limit: 10})
	if err != nil || len(found) != 1 || found[0].Name != "Ritzy Picturehouse" {
		t.Fatalf("multi-word search = %+v, %v", found, err)
	}
	if found, _ := repo.ListVenues(ctx, repository.VenueFilter{Query: "pvr mumbai", Limit: 10}); len(found) != 1 || found[0].Name != "PVR Phoenix" {
		t.Fatalf("name + town search = %+v", found)
	}
	// "vue" is in "Vue Birmingham" and inside "Avenue": the name that starts with it leads.
	if found, _ := repo.ListVenues(ctx, repository.VenueFilter{Query: "vue", Limit: 10}); len(found) != 2 || found[0].Name != "Vue Birmingham" {
		t.Fatalf("prefix first = %+v", found)
	}

	// Busy instance: retried. One failing country doesn't stop the others;
	// every country failing fails the run.
	src.busy = 1
	src.fail = map[string]bool{"US": true}
	rep, err = imp.Run(ctx, []string{"GB", "US"}, nil)
	if err != nil || rep.Failed != 1 || rep.Countries[1].Error == "" || rep.Countries[0].Error != "" {
		t.Fatalf("partial failure = %+v, %v", rep, err)
	}
	if _, err := imp.Run(ctx, []string{"US"}, nil); err == nil {
		t.Fatal("all countries failing should fail the run")
	}
	src.busy = venueBusyRetries + 1
	if rep, _ := imp.Run(ctx, []string{"GB"}, nil); rep.Failed != 1 || !strings.Contains(rep.Countries[0].Error, "busy") {
		t.Fatalf("busy too long = %+v", rep)
	}
}

func TestVenueFromOSM(t *testing.T) {
	imp := &VenueImporter{}
	for _, tc := range []struct {
		name string
		pl   client.OSMPlace
		want string // name, or "" for skipped
	}{
		{"latin name kept over name:en", osm("node", 1, 0, 0, "name", "Cinéma Le Champo", "name:en", "Champo Cinema"), "Cinéma Le Champo"},
		{"non-latin uses name:en", osm("node", 1, 0, 0, "name", "シネマ", "name:en", "Cinema"), "Cinema"},
		{"non-latin without name:en kept", osm("node", 1, 0, 0, "name", "シネマ"), "シネマ"},
		{"access no", osm("node", 1, 0, 0, "name", "X", "access", "no"), ""},
		{"blank name", osm("node", 1, 0, 0, "name", "  "), ""},
	} {
		v, ok := imp.venueFromOSM("JP", tc.pl)
		if got := map[bool]string{true: v.Name, false: ""}[ok]; got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.name, got, tc.want)
		}
	}
	v, _ := imp.venueFromOSM("US", osm("node", 1, 0, 0, "name", "A", "start_date", "1950-13-01", "addr:full", "1 Main St", "addr:province", "ON", "wikidata", "not-a-qid"))
	if v.OpenedOn != nil || *v.Address != "1 Main St" || *v.Region != "ON" || v.Timezone != nil {
		t.Fatalf("venue = %+v", v)
	}
	if wikidataTag(map[string]string{"wikidata": "Q12"}) != "Q12" || wikidataTag(map[string]string{"wikidata": "Q0"}) != "" {
		t.Fatal("wikidata tag")
	}
	ix := newTownIndex([]client.OSMPlace{osm("node", 1, 19.0760, 72.8777, "name", "Mumbai"), osm("node", 2, 19.2183, 72.9781, "name", "Thane")})
	if got := ix.nearest(19.0, 72.83); got == nil || *got != "Mumbai" {
		t.Fatalf("nearest town = %v", got)
	}
	// A city within 10 km beats a nearer town.
	boston := newTownIndex([]client.OSMPlace{osm("node", 1, 42.3555, -71.0565, "name", "Boston", "place", "city"), osm("node", 2, 42.3318, -71.1212, "name", "Brookline", "place", "town")})
	if got := boston.nearest(42.3405, -71.0894); got == nil || *got != "Boston" {
		t.Fatalf("city over town = %v", got)
	}
	if got := ix.nearest(20.5, 74.0); got != nil {
		t.Fatalf("town 200 km away = %q", *got)
	}
	if d := haversineM(51.5113, -0.1303, 51.5113, -0.1260); d < 250 || d > 350 {
		t.Fatalf("haversine = %f", d)
	}
}
