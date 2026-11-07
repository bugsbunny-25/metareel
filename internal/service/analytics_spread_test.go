package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/bugsbunny-25/metareel/internal/repository"
)

func TestComputeSpread(t *testing.T) {
	var rows []repository.ChartEntryRow
	add := func(slug, date, country string, rank int64) {
		var tmdb *string
		if slug == "a" {
			id := "1"
			tmdb = &id
		}
		rows = append(rows, repository.ChartEntryRow{Date: date, Country: country, Provider: "netflix", Category: "tv_shows", Rank: rank,
			TitleID: int64(slug[0]), Slug: slug, Name: strings.ToUpper(slug), Kind: "tv_show", TmdbID: tmdb})
	}
	// daily stores filler charts (title z) every day from..to.
	daily := func(country, from, to string) {
		for d := day(from); !d.After(day(to)); d = d.AddDate(0, 0, 1) {
			add("z", repository.FormatDate(d), country, 10)
		}
	}
	// Stored ranges: US and GB from 10-01 to 11-29, IN from 11-10.
	daily("US", "2026-10-01", "2026-11-29")
	daily("GB", "2026-10-01", "2026-11-29")
	daily("IN", "2026-11-10", "2026-11-29")
	add("y", "2026-11-10", "IN", 1)
	// DE: stored from 10-28, under 30 days before every debut: unknown.
	daily("DE", "2026-10-28", "2026-11-29")
	// FR: a gap from 10-21 to 10-31 (19 of the 30 days before 11-05 stored).
	daily("FR", "2026-10-01", "2026-10-20")
	daily("FR", "2026-11-01", "2026-11-29")
	add("i", "2026-11-05", "FR", 1) // debut in FR: dropped
	add("i", "2026-11-07", "US", 1)
	// ES: stored before every debut, but not during c's window (11-03..11-10).
	daily("ES", "2026-09-01", "2026-11-02")
	daily("ES", "2026-11-11", "2026-11-29")
	add("h", "2026-11-05", "DE", 1) // debut in DE: dropped
	add("h", "2026-11-06", "US", 1)
	add("a", "2026-11-06", "DE", 1)
	add("a", "2026-11-05", "US", 3) // debut in US
	add("a", "2026-11-08", "GB", 5) // 3 days later
	add("a", "2026-11-07", "GB", 9) // earlier still: lag 2, rank 9
	add("a", "2026-11-12", "IN", 1) // IN not stored before the debut: unknown
	add("b", "2026-11-06", "GB", 2) // simultaneous debut
	add("b", "2026-11-06", "US", 4)
	add("c", "2026-11-03", "US", 1) // GB never: not reached within 7 days
	add("d", "2026-10-20", "US", 1) // charted in the lookback: no debut in range
	add("d", "2026-11-10", "GB", 1)
	add("e", "2026-11-10", "IN", 1) // IN's charts start that day: censored
	add("f", "2026-11-28", "US", 1) // GB window not over yet: pending
	add("g", "2026-11-04", "US", 2) // GB after the window: not reached
	add("g", "2026-11-20", "GB", 2)

	f := analyticsFilter{from: day("2026-11-01"), to: day("2026-11-30")}
	ctx := context.Background()
	out, err := computeSpread(ctx, rows, f, 7)
	if err != nil {
		t.Fatal(err)
	}
	if out.Titles != 5 || out.MultiCountry != 2 || out.Simultaneous != 1 {
		t.Fatalf("titles %d, multi %d, simultaneous %d", out.Titles, out.MultiCountry, out.Simultaneous)
	}
	// Lags after the first country: a GB 2, b's second origin 0.
	if out.Summary.Titles != 2 || *out.Summary.Median != 1 {
		t.Fatalf("summary = %+v", out.Summary)
	}
	if len(out.Items) != 2 || out.Items[0].Name != "B" || out.Items[1].Name != "A" {
		t.Fatalf("items = %+v", out.Items)
	}
	a := out.Items[1]
	if strings.Join(a.Origins, ",") != "US" || len(a.Arrivals) != 2 || a.Arrivals[1].Country != "GB" || a.Arrivals[1].LagDays != 2 ||
		a.Arrivals[1].Rank != 9 || a.Arrivals[1].FirstDate != "2026-11-07" || len(a.NotReached) != 0 {
		t.Fatalf("a = %+v", a)
	}
	if b := out.Items[0]; strings.Join(b.Origins, ",") != "GB,US" || b.Arrivals[0].LagDays != 0 || b.Arrivals[1].LagDays != 0 {
		t.Fatalf("b = %+v", b)
	}
	countries := map[string]SpreadCountry{}
	for _, c := range out.Countries {
		countries[c.Country] = c
	}
	us, gb := countries["US"], countries["GB"]
	if us.Origin != 5 || us.Arrived != 0 || gb.Origin != 1 || gb.Arrived != 1 || gb.NotReached != 2 || *gb.ArrivalLag.Median != 2 {
		t.Fatalf("countries = %+v", out.Countries)
	}
	if _, ok := countries["IN"]; ok {
		t.Fatalf("IN has no followed titles: %+v", out.Countries)
	}
	for _, c := range []string{"DE", "FR"} {
		if _, ok := countries[c]; ok {
			t.Fatalf("%s lacks 30 days of history: %+v", c, out.Countries)
		}
	}
	// ES: g (11-04, window to 11-11) and c (11-03, to 11-10) have gaps there;
	// a, b, f are not reached / pending.
	if es := countries["ES"]; es.NotReached != 0 {
		t.Fatalf("ES = %+v", es)
	}
	if len(out.Pairs) != 1 || out.Pairs[0].From != "US" || out.Pairs[0].To != "GB" || out.Pairs[0].Reached != 1 || out.Pairs[0].NotReached != 2 {
		t.Fatalf("pairs = %+v", out.Pairs)
	}
	// Only titles that debuted in GB.
	f.country = "GB"
	if out, _ = computeSpread(ctx, rows, f, 7); out.Titles != 1 || out.Items[0].Name != "B" {
		t.Fatalf("country=GB: %+v", out)
	}

	var v *ValidationError
	for _, w := range []string{"6", "91", "x"} {
		if _, err := parseSpreadWindow(w); !errors.As(err, &v) {
			t.Errorf("window %q: %v", w, err)
		}
	}
	if w, err := parseSpreadWindow(""); err != nil || w != 30 {
		t.Fatalf("default window = %d, %v", w, err)
	}
}

func TestSpreadLagLimits(t *testing.T) {
	db := newTestDB(t)
	svc := NewAnalyticsService(repository.NewAnalyticsRepository(db), repository.NewMetadataRepository(db), nil, nil)
	from, to := day("2026-01-01"), day("2026-06-30") // 181 days
	var v *ValidationError
	if _, err := svc.SpreadLag(context.Background(), AnalyticsQuery{From: &from, To: &to}); !errors.As(err, &v) {
		t.Fatalf("181 days: err = %v", err)
	}
	to = day("2026-06-29")
	if out, err := svc.SpreadLag(context.Background(), AnalyticsQuery{From: &from, To: &to}); err != nil || out.Titles != 0 || out.Window != 30 {
		t.Fatalf("180 days: %+v, %v", out, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	rows := []repository.ChartEntryRow{{Date: "2026-03-01", Country: "US", Provider: "netflix", Category: "movies", Rank: 1, TitleID: 1, Kind: "movie"}}
	if _, err := computeSpread(ctx, rows, analyticsFilter{from: from, to: to}, 30); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled: %v", err)
	}
}
