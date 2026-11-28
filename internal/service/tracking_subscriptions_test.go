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
	usd, gbp := "USD", "GBP"
	sub := repository.CinemaSubscription{ID: 1, Plan: "AMC Stubs A-List", Chain: new("AMC"), PriceMinor: 2399, Currency: "USD", Period: "month", StartedOn: "2026-09-10"}
	covered := new(int64(1))
	watches := []repository.CinemaWatchCost{
		// Paid tickets: price the covered watches without a face value (median at AMC: 1650).
		{ID: 1, Day: "2026-05-01", PriceMinor: new(int64(1600)), Currency: &usd, Tickets: 1, Chain: "AMC Theatres"},
		{ID: 2, Day: "2026-06-01", PriceMinor: new(int64(3400)), Currency: &usd, Tickets: 2, Chain: "AMC Theatres"},
		{ID: 3, Day: "2026-06-02", PriceMinor: new(int64(900)), Currency: &usd, Tickets: 1, Chain: "Regal"},
		// Covered.
		{ID: 4, Day: "2026-09-12", SubscriptionID: covered, FaceValueMinor: new(int64(2200)), PriceMinor: new(int64(500)), Currency: &usd, Tickets: 1, Chain: "AMC Theatres"}, // IMAX upcharge
		{ID: 5, Day: "2026-09-20", SubscriptionID: covered, Tickets: 1, Chain: "AMC Theatres"},
		{ID: 6, Day: "2026-10-03", SubscriptionID: covered, FaceValueMinor: new(int64(1800)), Currency: &usd, Tickets: 1},
		{ID: 7, Day: "2026-10-04", SubscriptionID: covered, FaceValueMinor: new(int64(1500)), Currency: &gbp, Tickets: 1},
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
	sub.EndedOn = new("2026-10-20")
	if r := planSavings(sub, watches, "2026-11-13"); r.Bills != 2 || r.Active {
		t.Fatalf("ended = %+v", r)
	}

	// What-if: AMC paid tickets in the last year, one ticket per watch.
	w := whatIf(nil, watches, "2026-11-13")
	if len(w) != 2 || w[0].Plan != "AMC Stubs A-List" || w[0].Watches != 2 || w[0].SpentMinor != 3300 || w[0].PerMonthMinor != 275 || w[1].Plan != "Regal Unlimited" {
		t.Fatalf("what-if = %+v", w)
	}
}

func TestPlanBillsAcrossGapsAndPriceChanges(t *testing.T) {
	// Subscribed 10 Jan at 20.00, cancelled 15 Mar, back 1 Jun; the price went
	// up to 25.00 from 1 Jul. Bills: 10 Jan, 10 Feb, 10 Mar (run 1); 1 Jun,
	// 1 Jul, 1 Aug (run 2, the last two at the new price). Nothing in Apr/May.
	sub := repository.CinemaSubscription{ID: 1, Plan: "ODEON Limitless", Currency: "GBP", Period: "month",
		Periods: []repository.SubscriptionPeriod{{StartedOn: "2026-01-10", EndedOn: new("2026-03-15")}, {StartedOn: "2026-06-01"}},
		Prices:  []repository.SubscriptionPrice{{From: "2026-01-10", PriceMinor: 2000}, {From: "2026-07-01", PriceMinor: 2500}}}
	got := planBills(sub, "2026-08-20")
	want := []Bill{{"2026-01-10", 2000}, {"2026-02-10", 2000}, {"2026-03-10", 2000}, {"2026-06-01", 2000}, {"2026-07-01", 2500}, {"2026-08-01", 2500}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("bills = %v, want %v", got, want)
	}
	r := planSavings(sub, nil, "2026-08-20")
	if r.Bills != 6 || r.PaidMinor != 4*2000+2*2500 || !r.Active {
		t.Fatalf("savings = %+v", r)
	}
	for _, m := range r.Months {
		if m.Month == "2026-04" || m.Month == "2026-05" {
			t.Fatalf("billed in the gap: %+v", m)
		}
	}
	// Cancelled again: inactive, and no bill after the end.
	sub.Periods[1].EndedOn = new("2026-07-15")
	if r := planBills(sub, "2026-08-20"); len(r) != 5 {
		t.Fatalf("bills after second cancel = %v", r)
	}
}

func TestSubscriptionFromHistory(t *testing.T) {
	s := &TrackingService{}
	base := SubscriptionInput{Plan: "Regal Unlimited", Currency: "usd", Period: "month"}

	// Periods given out of order with a gap, a price change; summary follows them.
	in := base
	in.Periods = &[]SubscriptionPeriodInput{{StartedOn: "2026-06-01"}, {StartedOn: "2026-01-10", EndedOn: new("2026-03-15")}}
	in.Prices = &[]SubscriptionPriceInput{{From: "2099-01-01", PriceMinor: new(int64(2500))}, {From: "2026-02-01", PriceMinor: new(int64(1999))}}
	got, err := s.subscriptionFrom(in, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.StartedOn != "2026-01-10" || got.EndedOn != nil || got.Periods[0].StartedOn != "2026-01-10" || got.Currency != "USD" {
		t.Fatalf("periods = %+v", got)
	}
	// The first price is moved back to the plan's start; today's price is the summary.
	if got.Prices[0].From != "2026-01-10" || got.PriceMinor != 1999 {
		t.Fatalf("prices = %+v, price %d", got.Prices, got.PriceMinor)
	}

	bad := func(name string, mod func(*SubscriptionInput)) {
		t.Helper()
		in := base
		in.Periods = &[]SubscriptionPeriodInput{{StartedOn: "2026-01-10", EndedOn: new("2026-03-15")}, {StartedOn: "2026-06-01"}}
		in.PriceMinor = new(int64(2000))
		mod(&in)
		if _, err := s.subscriptionFrom(in, nil); err == nil {
			t.Errorf("%s: no error", name)
		}
	}
	bad("overlap", func(in *SubscriptionInput) { (*in.Periods)[1].StartedOn = "2026-03-15" })
	bad("open period before another", func(in *SubscriptionInput) { (*in.Periods)[0].EndedOn = nil })
	bad("end before start", func(in *SubscriptionInput) { (*in.Periods)[0].EndedOn = new("2026-01-01") })
	bad("no periods", func(in *SubscriptionInput) { in.Periods = &[]SubscriptionPeriodInput{} })
	bad("duplicate price date", func(in *SubscriptionInput) {
		in.Prices = &[]SubscriptionPriceInput{{From: "2026-02-01", PriceMinor: new(int64(1))}, {From: "2026-02-01", PriceMinor: new(int64(2))}}
	})
	bad("missing price", func(in *SubscriptionInput) { in.PriceMinor = nil })

	// Update without periods or prices (an older client): runs and prices are
	// kept, the last end and today's price replaced.
	prev := got
	upd := base
	upd.StartedOn, upd.EndedOn, upd.PriceMinor = "2026-01-10", new("2026-09-30"), new(int64(2100))
	got2, err := s.subscriptionFrom(upd, &prev)
	if err != nil {
		t.Fatal(err)
	}
	if len(got2.Periods) != 2 || got2.Periods[1].EndedOn == nil || *got2.Periods[1].EndedOn != "2026-09-30" || *got2.EndedOn != "2026-09-30" {
		t.Fatalf("kept periods = %+v", got2.Periods)
	}
	// Stored: 19.99 from the start, 25.00 from 2099; today's (the first) becomes 21.00.
	if len(got2.Prices) != 2 || got2.Prices[0].PriceMinor != 2100 || got2.Prices[1].PriceMinor != 2500 || got2.PriceMinor != 2100 {
		t.Fatalf("kept prices = %+v", got2.Prices)
	}
}
