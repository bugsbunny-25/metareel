package service

import (
	"reflect"
	"testing"

	"github.com/bugsbunny-25/metareel/internal/repository"
)

func TestBillingDays(t *testing.T) {
	for _, tc := range []struct {
		start, end, period string
		want               []string
	}{
		{"2026-01-31", "2026-04-30", "month", []string{"2026-01-31", "2026-02-28", "2026-03-31", "2026-04-30"}},
		{"2026-01-15", "2026-03-14", "month", []string{"2026-01-15", "2026-02-15"}},
		{"2024-02-29", "2026-03-01", "year", []string{"2024-02-29", "2025-02-28", "2026-02-28"}},
		{"2026-05-01", "2026-04-01", "month", nil},
		{"2025-11-20", "2026-01-20", "month", []string{"2025-11-20", "2025-12-20", "2026-01-20"}},
	} {
		if got := billingDays(tc.start, tc.end, tc.period); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("billingDays(%s, %s, %s) = %v, want %v", tc.start, tc.end, tc.period, got, tc.want)
		}
	}
}

func TestPlanSavings(t *testing.T) {
	i64 := func(v int64) *int64 { return &v }
	usd, gbp := "USD", "GBP"
	sub := repository.CinemaSubscription{ID: 1, Plan: "AMC Stubs A-List", Chain: str("AMC"), PriceMinor: 2399, Currency: "USD", Period: "month", StartedOn: "2026-09-10"}
	covered := i64(1)
	watches := []repository.CinemaWatchCost{
		// Paid tickets: price the covered watches without a face value (median at AMC: 1650).
		{ID: 1, Day: "2026-05-01", PriceMinor: i64(1600), Currency: &usd, Tickets: 1, Chain: "AMC Theatres"},
		{ID: 2, Day: "2026-06-01", PriceMinor: i64(3400), Currency: &usd, Tickets: 2, Chain: "AMC Theatres"},
		{ID: 3, Day: "2026-06-02", PriceMinor: i64(900), Currency: &usd, Tickets: 1, Chain: "Regal"},
		// Covered.
		{ID: 4, Day: "2026-09-12", SubscriptionID: covered, FaceValueMinor: i64(2200), PriceMinor: i64(500), Currency: &usd, Tickets: 1, Chain: "AMC Theatres"}, // IMAX upcharge
		{ID: 5, Day: "2026-09-20", SubscriptionID: covered, Tickets: 1, Chain: "AMC Theatres"},
		{ID: 6, Day: "2026-10-03", SubscriptionID: covered, FaceValueMinor: i64(1800), Currency: &usd, Tickets: 1},
		{ID: 7, Day: "2026-10-04", SubscriptionID: covered, FaceValueMinor: i64(1500), Currency: &gbp, Tickets: 1},
	}
	r := planSavings(sub, watches, "2026-11-13")
	// Bills: Sep 10, Oct 10, Nov 10 = 3 x 2399.
	if r.Bills != 3 || r.PaidMinor != 7197 || !r.Active {
		t.Fatalf("bills = %+v", r)
	}
	// Value: 2200 + 1650 (estimated) + 1800; extras 500; GBP watch left out.
	if r.Watches != 3 || r.Estimated != 1 || r.OtherCurrency != 1 || r.ValueMinor != 5650 || r.ExtrasMinor != 500 || r.SavedMinor != 5650-7197-500 {
		t.Fatalf("savings = %+v", r)
	}
	if r.UsualTicket == nil || *r.UsualTicket != 1650 {
		t.Fatalf("usual ticket = %v", r.UsualTicket)
	}
	if r.AvgTicket == nil || *r.AvgTicket != 1883 || r.BreakEven == nil || *r.BreakEven != 2 || r.PerPeriod != 1 {
		t.Fatalf("break-even = avg %v, %v, per period %v", r.AvgTicket, r.BreakEven, r.PerPeriod)
	}
	if len(r.Months) != 3 || r.Months[0] != (SavingsMonth{Month: "2026-09", Watches: 2, ValueMinor: 3850, CostMinor: 2899, NetMinor: 951}) {
		t.Fatalf("months = %+v", r.Months)
	}

	// Ended in October: two bills, inactive.
	sub.EndedOn = str("2026-10-20")
	if r := planSavings(sub, watches, "2026-11-13"); r.Bills != 2 || r.Active {
		t.Fatalf("ended = %+v", r)
	}

	// What-if: AMC paid tickets in the last year, one ticket per watch.
	w := whatIf(nil, watches, "2026-11-13")
	if len(w) != 2 || w[0].Plan != "AMC Stubs A-List" || w[0].Watches != 2 || w[0].SpentMinor != 3300 || w[0].PerMonthMinor != 275 || w[1].Plan != "Regal Unlimited" {
		t.Fatalf("what-if = %+v", w)
	}
}
