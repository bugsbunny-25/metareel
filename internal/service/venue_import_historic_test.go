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

type fakeHistoric struct {
	cinemas []client.WikidataCinema
	fail    bool
}

func (f *fakeHistoric) CountryQID(_ context.Context, iso string) (string, error) { return "Q145", nil }
func (f *fakeHistoric) ClosedCinemas(_ context.Context, qid string) ([]client.WikidataCinema, string, error) {
	if f.fail {
		return nil, "", errors.New("wikidata api status 400")
	}
	return f.cinemas, "", nil
}

func TestVenueImportHistoric(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	repo := repository.NewTrackingRepository(db)
	// Typed in by hand, without a position: matched by name and town.
	hand, _ := repo.CreateVenue(ctx, repository.Venue{Name: "The Scala", City: str("London"), Country: str("GB"), Status: "closed"}, 0)

	src := &fakeOSM{places: map[string][]client.OSMPlace{"GB": {
		osm("node", 1, 51.5113, -0.1303, "name", "Prince Charles Cinema", "addr:city", "London"),
	}}, towns: map[string][]client.OSMPlace{"GB": {osm("node", 100, 51.5072, -0.1276, "name", "London", "place", "city")}}}
	hist := &fakeHistoric{cinemas: []client.WikidataCinema{
		{QID: "Q1", Name: "The Scala", Lat: 51.5306, Lon: -0.1218, HasCoord: true, Opened: "1920", Closed: "1993-06-01"},
		{QID: "Q2", Name: "Prince Charles Cinema", Lat: 51.5114, Lon: -0.1302, HasCoord: true, Opened: "1962-01-01", Closed: "1990"},
		{QID: "Q3", Name: "Empire Leicester Square", Lat: 51.5108, Lon: -0.1302, HasCoord: true, Address: "5-6 Leicester Square", Opened: "1884-04-17", Closed: "2016-01", Demolished: true},
		{QID: "Q4", Name: "", Lat: 51.5, Lon: -0.1, HasCoord: true},
		{QID: "Q5", Name: "Nowhere Picture House"},
	}}
	imp := NewVenueImporter(slog.New(slog.NewTextHandler(io.Discard, nil)), src, repo, fakeTZ{}, []string{"GB"}).WithHistoric(hist)
	imp.gap, imp.busyWait = 0, 0

	var logs []string
	rep, err := imp.Run(ctx, nil, func(_, m string) { logs = append(logs, m) })
	if err != nil {
		t.Fatal(err)
	}
	h := rep.Countries[0].Historic
	if h == nil || h.Fetched != 5 || h.Skipped != 2 || h.Created != 1 || h.Linked != 2 || h.Known != 0 {
		t.Fatalf("historic = %+v", h)
	}
	if !strings.Contains(strings.Join(logs, "\n"), "GB: 5 closed cinemas on Wikidata; 1 new, 2 matched") {
		t.Fatalf("logs = %q", logs)
	}

	// The open cinema OSM knows stays open and gets no closing note.
	rows, _ := repo.VenuesForImport(ctx, "GB")
	byName := map[string]repository.ImportVenueRow{}
	for _, r := range rows {
		byName[r.Name] = r
	}
	pcc := byName["Prince Charles Cinema"]
	if pv, _ := repo.Venue(ctx, pcc.ID); pv.Status != "open" || pv.ClosedOn != nil || pv.Note != nil || pv.OpenedOn == nil || *pv.OpenedOn != "1962-01-01" || pcc.WikidataRef != "Q2" {
		t.Fatalf("open cinema = %+v (ref %q)", pv, pcc.WikidataRef)
	}
	// The hand-entered one gains position, zone, the full closing date and a note for the year.
	if sv, _ := repo.Venue(ctx, hand); sv.Latitude == nil || *sv.ClosedOn != "1993-06-01" || *sv.Note != "Opened 1920 (Wikidata)" || *sv.Timezone != "Europe/London" {
		t.Fatalf("hand-entered = %+v", sv)
	}
	emp := byName["Empire Leicester Square"]
	ev, _ := repo.Venue(ctx, emp.ID)
	if ev.Status != "closed" || *ev.OpenedOn != "1884-04-17" || ev.ClosedOn != nil || *ev.Note != "Closed 2016-01, demolished (Wikidata)" ||
		*ev.Address != "5-6 Leicester Square" || *ev.City != "London" || emp.WikidataRef != "Q3" {
		t.Fatalf("created = %+v", ev)
	}

	// Re-run: nothing new. Wikidata failing leaves the current cinemas imported.
	rep, _ = imp.Run(ctx, nil, nil)
	if h := rep.Countries[0].Historic; h.Created != 0 || h.Known != 3 || h.Linked != 0 || h.Filled != 0 {
		t.Fatalf("re-run = %+v", h)
	}
	hist.fail = true
	rep, err = imp.Run(ctx, nil, nil)
	if err != nil || rep.Failed != 0 || rep.Countries[0].Historic.Error == "" || rep.Countries[0].Known != 1 {
		t.Fatalf("wikidata down = %+v, %v", rep.Countries[0], err)
	}
}
