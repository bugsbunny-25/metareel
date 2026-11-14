package service

import (
	"context"
	"database/sql"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/bugsbunny-25/metareel/internal/repository"
)

// Cinema subscriptions: plans like AMC Stubs A-List or ODEON Limitless that
// a user pays for monthly, the watches each covered, and whether it paid off.

// SubscriptionPlan is a well-known plan offered when adding one. Prices vary
// by region and change, so none is suggested: the user enters theirs.
type SubscriptionPlan struct {
	Plan     string `json:"plan"`
	Chain    string `json:"chain"` // matched against a venue's chain or name
	Country  string `json:"country"`
	Currency string `json:"currency"`
}

var SubscriptionPlans = []SubscriptionPlan{
	{"AMC Stubs A-List", "AMC", "US", "USD"},
	{"Regal Unlimited", "Regal", "US", "USD"},
	{"Cinemark Movie Club", "Cinemark", "US", "USD"},
	{"Alamo Drafthouse Season Pass", "Alamo Drafthouse", "US", "USD"},
	{"Cineworld Unlimited", "Cineworld", "GB", "GBP"},
	{"ODEON Limitless", "Odeon", "GB", "GBP"},
	{"PVR Passport", "PVR", "IN", "INR"},
}

// SubscriptionInput creates or replaces a plan.
type SubscriptionInput struct {
	Plan       string  `json:"plan"`
	Chain      *string `json:"chain"`
	PriceMinor *int64  `json:"price_minor"`
	Currency   string  `json:"currency"`
	Period     string  `json:"period"` // month (default) | year
	StartedOn  string  `json:"started_on"`
	EndedOn    *string `json:"ended_on"`
	Note       *string `json:"note"`
}

func (s *TrackingService) subscriptionFrom(in SubscriptionInput) (repository.CinemaSubscription, error) {
	out := repository.CinemaSubscription{Plan: strings.TrimSpace(in.Plan), Chain: trimmedPtr(in.Chain), Note: trimmedPtr(in.Note),
		Currency: strings.ToUpper(strings.TrimSpace(in.Currency)), Period: strings.TrimSpace(in.Period), StartedOn: strings.TrimSpace(in.StartedOn)}
	if n := len([]rune(out.Plan)); n == 0 || n > 100 {
		return out, &ValidationError{Message: "plan is required (at most 100 characters)"}
	}
	if out.Chain != nil && len([]rune(*out.Chain)) > 100 {
		return out, &ValidationError{Message: "chain must be at most 100 characters"}
	}
	if out.Note != nil && len([]rune(*out.Note)) > 500 {
		return out, &ValidationError{Message: "note must be at most 500 characters"}
	}
	if in.PriceMinor == nil || *in.PriceMinor < 0 || *in.PriceMinor > 1e9 {
		return out, &ValidationError{Message: "price_minor is required: what the plan costs per period, in minor units (cents, pence, paise)"}
	}
	out.PriceMinor = *in.PriceMinor
	if !currencyRe.MatchString(out.Currency) {
		return out, &ValidationError{Message: "currency must be a 3-letter ISO 4217 code, e.g. USD"}
	}
	if out.Period == "" {
		out.Period = "month"
	}
	if out.Period != "month" && out.Period != "year" {
		return out, &ValidationError{Message: "period must be month or year"}
	}
	start, err := time.Parse(time.DateOnly, out.StartedOn)
	if err != nil {
		return out, &ValidationError{Message: "started_on must be a date (YYYY-MM-DD)"}
	}
	if end := trimmedPtr(in.EndedOn); end != nil {
		e, err := time.Parse(time.DateOnly, *end)
		if err != nil {
			return out, &ValidationError{Message: "ended_on must be a date (YYYY-MM-DD)"}
		}
		if e.Before(start) {
			return out, &ValidationError{Message: "ended_on must not be before started_on"}
		}
		out.EndedOn = end
	}
	return out, nil
}

// ListSubscriptions returns a user's plans and the well-known ones to pick from.
func (s *TrackingService) ListSubscriptions(ctx context.Context, userID int64) (map[string]any, error) {
	subs, err := s.read.CinemaSubscriptions(ctx, userID)
	if err != nil {
		return nil, err
	}
	return map[string]any{"subscriptions": subs, "plans": SubscriptionPlans}, nil
}

func (s *TrackingService) CreateSubscription(ctx context.Context, userID int64, in SubscriptionInput) (repository.CinemaSubscription, error) {
	sub, err := s.subscriptionFrom(in)
	if err != nil {
		return sub, err
	}
	id, err := s.write.CreateCinemaSubscription(ctx, userID, sub)
	if err != nil {
		return sub, err
	}
	return s.write.CinemaSubscription(ctx, userID, id)
}

func (s *TrackingService) UpdateSubscription(ctx context.Context, userID, id int64, in SubscriptionInput) (repository.CinemaSubscription, error) {
	sub, err := s.subscriptionFrom(in)
	if err != nil {
		return sub, err
	}
	sub.ID = id
	ok, err := s.write.UpdateCinemaSubscription(ctx, userID, sub)
	if err != nil {
		return sub, err
	}
	if !ok {
		return sub, sql.ErrNoRows
	}
	return s.write.CinemaSubscription(ctx, userID, id)
}

// DeleteSubscription removes a plan; the watches it covered stay, uncovered.
func (s *TrackingService) DeleteSubscription(ctx context.Context, userID, id int64) error {
	ok, err := s.write.DeleteCinemaSubscription(ctx, userID, id)
	if err != nil {
		return err
	}
	if !ok {
		return sql.ErrNoRows
	}
	return nil
}

// checkSubscription makes sure a watch is only covered by the user's own plan.
func (s *TrackingService) checkSubscription(ctx context.Context, userID int64, c *repository.WatchContext) error {
	if c == nil || c.SubscriptionID == nil {
		return nil
	}
	if _, err := s.write.CinemaSubscription(ctx, userID, *c.SubscriptionID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return &ValidationError{Message: "subscription_id is not one of your cinema subscriptions"}
		}
		return err
	}
	return nil
}

// --- savings ----------------------------------------------------------------------

// SubscriptionSavings is one plan's balance: what it cost against what the
// tickets it covered would have cost.
type SubscriptionSavings struct {
	repository.CinemaSubscription
	Active        bool           `json:"active"`
	Bills         int            `json:"bills"`          // periods paid so far
	PaidMinor     int64          `json:"paid_minor"`     // bills x price
	Watches       int            `json:"watches"`        // covered watches in the plan's currency
	ValueMinor    int64          `json:"value_minor"`    // their face value (some estimated)
	Estimated     int            `json:"estimated"`      // covered watches valued at your usual ticket price
	Unvalued      int            `json:"unvalued"`       // covered watches with no face value and nothing to estimate from
	OtherCurrency int            `json:"other_currency"` // covered watches in another currency (not counted)
	ExtrasMinor   int64          `json:"extras_minor"`   // upcharges and fees paid on covered watches
	SavedMinor    int64          `json:"saved_minor"`    // value - paid - extras; negative = it cost more
	AvgTicket     *int64         `json:"avg_ticket_minor"`
	UsualTicket   *int64         `json:"usual_ticket_minor"` // the median paid ticket that values covered watches without a face value
	BreakEven     *int           `json:"break_even_watches"` // covered watches a period needs to pay off
	PerPeriod     float64        `json:"watches_per_period"`
	Months        []SavingsMonth `json:"months"`
}

// SavingsMonth is one month of a plan: watches and their value, and what it cost.
type SavingsMonth struct {
	Month      string `json:"month"` // YYYY-MM
	Watches    int    `json:"watches"`
	ValueMinor int64  `json:"value_minor"`
	CostMinor  int64  `json:"cost_minor"` // a bill in that month plus extras
	NetMinor   int64  `json:"net_minor"`
}

// SubscriptionWhatIf is what you spent on tickets at a chain with a known
// plan over the last year: a plan costing less a month would have paid off.
type SubscriptionWhatIf struct {
	SubscriptionPlan
	Watches          int   `json:"watches"`
	SpentMinor       int64 `json:"spent_minor"` // your tickets (one per watch) and fees
	PerMonthMinor    int64 `json:"per_month_minor"`
	AvgTicketMinor   int64 `json:"avg_ticket_minor"`
	HaveSubscription bool  `json:"have_subscription"`
}

// Savings works out every plan's balance and, for chains you paid at in the
// last 12 months, what a plan would have had to cost to pay off.
func (s *TrackingService) Savings(ctx context.Context, userID int64) (map[string]any, error) {
	subs, err := s.read.CinemaSubscriptions(ctx, userID)
	if err != nil {
		return nil, err
	}
	watches, err := s.read.CinemaWatchCosts(ctx, userID)
	if err != nil {
		return nil, err
	}
	today := time.Now().UTC().Format(time.DateOnly)
	out := make([]SubscriptionSavings, 0, len(subs))
	for _, sub := range subs {
		out = append(out, planSavings(sub, watches, today))
	}
	return map[string]any{"subscriptions": out, "what_if": whatIf(subs, watches, today)}, nil
}

func planSavings(sub repository.CinemaSubscription, watches []repository.CinemaWatchCost, today string) SubscriptionSavings {
	r := SubscriptionSavings{CinemaSubscription: sub, Months: []SavingsMonth{}}
	end := today
	if sub.EndedOn != nil && *sub.EndedOn < today {
		end = *sub.EndedOn
	}
	r.Active = sub.EndedOn == nil || *sub.EndedOn >= today
	months := map[string]*SavingsMonth{}
	month := func(day string) *SavingsMonth {
		k := day[:7]
		if months[k] == nil {
			months[k] = &SavingsMonth{Month: k}
		}
		return months[k]
	}
	for _, bill := range billingDays(sub.StartedOn, end, sub.Period) {
		r.Bills++
		r.PaidMinor += sub.PriceMinor
		month(bill).CostMinor += sub.PriceMinor
	}

	usual := usualTicket(watches, sub.Chain, sub.Currency)
	r.UsualTicket = usual
	var valued []int64
	for _, w := range watches {
		if w.SubscriptionID == nil || *w.SubscriptionID != sub.ID {
			continue
		}
		if w.Currency != nil && *w.Currency != sub.Currency {
			r.OtherCurrency++
			continue
		}
		r.Watches++
		m := month(w.Day)
		m.Watches++
		extra := minorOf(w.PriceMinor) + minorOf(w.FeesMinor)
		r.ExtrasMinor += extra
		m.CostMinor += extra
		switch {
		case w.FaceValueMinor != nil:
			r.ValueMinor += *w.FaceValueMinor
			m.ValueMinor += *w.FaceValueMinor
			valued = append(valued, *w.FaceValueMinor)
		case usual != nil:
			r.Estimated++
			r.ValueMinor += *usual
			m.ValueMinor += *usual
			valued = append(valued, *usual)
		default:
			r.Unvalued++
		}
	}
	r.SavedMinor = r.ValueMinor - r.PaidMinor - r.ExtrasMinor
	if len(valued) > 0 {
		var sum int64
		for _, v := range valued {
			sum += v
		}
		avg := sum / int64(len(valued))
		r.AvgTicket = &avg
	} else {
		r.AvgTicket = usual
	}
	if r.AvgTicket != nil && *r.AvgTicket > 0 {
		n := int((sub.PriceMinor + *r.AvgTicket - 1) / *r.AvgTicket)
		r.BreakEven = &n
	}
	if r.Bills > 0 {
		r.PerPeriod = float64(r.Watches) / float64(r.Bills)
	}
	for _, m := range months {
		m.NetMinor = m.ValueMinor - m.CostMinor
		r.Months = append(r.Months, *m)
	}
	sort.Slice(r.Months, func(i, j int) bool { return r.Months[i].Month < r.Months[j].Month })
	return r
}

// billingDays are the days a plan bills from start to end: the start day and
// every month (or year) after it, clamped to the month's last day.
func billingDays(start, end, period string) []string {
	s, err := time.Parse(time.DateOnly, start)
	if err != nil || end < start {
		return nil
	}
	var out []string
	for k := 0; k < 1200; k++ {
		y, m := s.Year(), int(s.Month())-1+k
		if period == "year" {
			y, m = s.Year()+k, int(s.Month())-1
		}
		y += m / 12
		m = m % 12
		first := time.Date(y, time.Month(m+1), 1, 0, 0, 0, 0, time.UTC)
		last := first.AddDate(0, 1, -1).Day()
		day := first.AddDate(0, 0, min(s.Day(), last)-1).Format(time.DateOnly)
		if day > end {
			break
		}
		out = append(out, day)
	}
	return out
}

// usualTicket is the median price of one ticket you paid for at the plan's
// chain (else at any cinema) in its currency, or nil.
func usualTicket(watches []repository.CinemaWatchCost, chain *string, currency string) *int64 {
	var atChain, anywhere []int64
	for _, w := range watches {
		if w.SubscriptionID != nil || w.PriceMinor == nil || *w.PriceMinor <= 0 || w.Currency == nil || *w.Currency != currency {
			continue
		}
		per := *w.PriceMinor / w.Tickets
		anywhere = append(anywhere, per)
		if chain != nil && chainMatches(*chain, w) {
			atChain = append(atChain, per)
		}
	}
	if len(atChain) > 0 {
		return medianMinor(atChain)
	}
	if len(anywhere) > 0 {
		return medianMinor(anywhere)
	}
	return nil
}

func chainMatches(chain string, w repository.CinemaWatchCost) bool {
	c := strings.ToLower(strings.TrimSpace(chain))
	return c != "" && (strings.Contains(strings.ToLower(w.Chain), c) || strings.Contains(strings.ToLower(w.VenueName), c))
}

func medianMinor(v []int64) *int64 {
	s := append([]int64(nil), v...)
	sort.Slice(s, func(i, j int) bool { return s[i] < s[j] })
	m := s[len(s)/2]
	if len(s)%2 == 0 {
		m = (s[len(s)/2-1] + s[len(s)/2]) / 2
	}
	return &m
}

func minorOf(p *int64) int64 {
	if p == nil {
		return 0
	}
	return *p
}

// whatIf sums the last 12 months of paid tickets at each chain with a known plan.
func whatIf(subs []repository.CinemaSubscription, watches []repository.CinemaWatchCost, today string) []SubscriptionWhatIf {
	t, _ := time.Parse(time.DateOnly, today)
	since := t.AddDate(-1, 0, 0).Format(time.DateOnly)
	out := []SubscriptionWhatIf{}
	for _, p := range SubscriptionPlans {
		r := SubscriptionWhatIf{SubscriptionPlan: p}
		for _, w := range watches {
			if w.Day < since || w.SubscriptionID != nil || w.PriceMinor == nil || w.Currency == nil || *w.Currency != p.Currency || !chainMatches(p.Chain, w) {
				continue
			}
			r.Watches++
			r.SpentMinor += *w.PriceMinor/w.Tickets + minorOf(w.FeesMinor)/w.Tickets
		}
		if r.Watches == 0 {
			continue
		}
		for _, sub := range subs {
			if sub.Chain != nil && strings.EqualFold(strings.TrimSpace(*sub.Chain), p.Chain) {
				r.HaveSubscription = true
			}
		}
		r.PerMonthMinor = r.SpentMinor / 12
		r.AvgTicketMinor = r.SpentMinor / int64(r.Watches)
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].SpentMinor > out[j].SpentMinor })
	return out
}
