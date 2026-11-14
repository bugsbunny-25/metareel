package repository

import (
	"context"
	"fmt"

	"github.com/bugsbunny-25/metareel/internal/repository/sqlc"
)

// CinemaSubscription is a user's cinema plan (AMC Stubs A-List, Regal
// Unlimited, ...): what it costs per period and when it ran. Periods are the
// runs between subscribing and cancelling (gaps aren't billed); Prices are
// what a billing period cost from each date. StartedOn, EndedOn and
// PriceMinor summarise them: first start, last end (nil while running) and
// the price in effect today.
type CinemaSubscription struct {
	ID         int64   `json:"id"`
	Plan       string  `json:"plan"`
	Chain      *string `json:"chain"`
	PriceMinor int64   `json:"price_minor"`
	Currency   string  `json:"currency"`
	Period     string  `json:"period"` // month | year
	StartedOn  string  `json:"started_on"`
	EndedOn    *string `json:"ended_on"`
	Note       *string `json:"note"`
	CreatedAt  string  `json:"created_at"`
	UpdatedAt  string  `json:"updated_at"`

	Periods []SubscriptionPeriod `json:"periods"`
	Prices  []SubscriptionPrice  `json:"prices"`
}

// SubscriptionPeriod is one run of a plan, from subscribing to cancelling.
type SubscriptionPeriod struct {
	StartedOn string  `json:"started_on"`
	EndedOn   *string `json:"ended_on"` // nil while it runs
}

// SubscriptionPrice is what a billing period costs from a date on (the
// earliest price also covers any bill before its date).
type SubscriptionPrice struct {
	From       string `json:"from"`
	PriceMinor int64  `json:"price_minor"`
}

func subscriptionFrom(r sqlc.CinemaSubscription) CinemaSubscription {
	return CinemaSubscription{ID: r.ID, Plan: r.Plan, Chain: nullStringPtr(r.Chain), PriceMinor: r.PriceMinor, Currency: r.Currency,
		Period: r.Period, StartedOn: r.StartedOn, EndedOn: nullStringPtr(r.EndedOn), Note: nullStringPtr(r.Note),
		CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt}
}

// CinemaSubscriptions lists a user's plans, current ones first, with their
// periods and prices.
func (r *TrackingRepository) CinemaSubscriptions(ctx context.Context, userID int64) ([]CinemaSubscription, error) {
	rows, err := r.q.ListCinemaSubscriptions(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("list cinema subscriptions: %w", err)
	}
	periods, prices, err := r.subscriptionHistory(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]CinemaSubscription, 0, len(rows))
	for _, row := range rows {
		s := subscriptionFrom(row)
		s.Periods, s.Prices = periods[s.ID], prices[s.ID]
		withHistory(&s)
		out = append(out, s)
	}
	return out, nil
}

// subscriptionHistory loads every period and price of a user's plans, by plan.
func (r *TrackingRepository) subscriptionHistory(ctx context.Context, userID int64) (map[int64][]SubscriptionPeriod, map[int64][]SubscriptionPrice, error) {
	pr, err := r.q.ListCinemaSubscriptionPeriods(ctx, userID)
	if err != nil {
		return nil, nil, fmt.Errorf("list cinema subscription periods: %w", err)
	}
	cr, err := r.q.ListCinemaSubscriptionPrices(ctx, userID)
	if err != nil {
		return nil, nil, fmt.Errorf("list cinema subscription prices: %w", err)
	}
	periods := map[int64][]SubscriptionPeriod{}
	for _, p := range pr {
		periods[p.SubscriptionID] = append(periods[p.SubscriptionID], SubscriptionPeriod{StartedOn: p.StartedOn, EndedOn: nullStringPtr(p.EndedOn)})
	}
	prices := map[int64][]SubscriptionPrice{}
	for _, p := range cr {
		prices[p.SubscriptionID] = append(prices[p.SubscriptionID], SubscriptionPrice{From: p.EffectiveOn, PriceMinor: p.PriceMinor})
	}
	return periods, prices, nil
}

// withHistory fills a plan with no stored periods or prices (written before
// they existed, or by a direct insert) from its summary columns.
func withHistory(s *CinemaSubscription) {
	if len(s.Periods) == 0 {
		s.Periods = []SubscriptionPeriod{{StartedOn: s.StartedOn, EndedOn: s.EndedOn}}
	}
	if len(s.Prices) == 0 {
		s.Prices = []SubscriptionPrice{{From: s.StartedOn, PriceMinor: s.PriceMinor}}
	}
}

// CinemaSubscription returns one of a user's plans; sql.ErrNoRows if it isn't theirs.
func (r *TrackingRepository) CinemaSubscription(ctx context.Context, userID, id int64) (CinemaSubscription, error) {
	row, err := r.q.GetCinemaSubscription(ctx, sqlc.GetCinemaSubscriptionParams{ID: id, UserID: userID})
	if err != nil {
		return CinemaSubscription{}, err
	}
	periods, prices, err := r.subscriptionHistory(ctx, userID)
	if err != nil {
		return CinemaSubscription{}, err
	}
	s := subscriptionFrom(row)
	s.Periods, s.Prices = periods[s.ID], prices[s.ID]
	withHistory(&s)
	return s, nil
}

// CreateCinemaSubscription stores a plan with its periods and prices.
func (r *TrackingRepository) CreateCinemaSubscription(ctx context.Context, userID int64, s CinemaSubscription) (int64, error) {
	var id int64
	err := r.inTx(ctx, func(q *sqlc.Queries) error {
		var err error
		id, err = q.CreateCinemaSubscription(ctx, sqlc.CreateCinemaSubscriptionParams{UserID: userID, Plan: s.Plan, Chain: toNullString(s.Chain),
			PriceMinor: s.PriceMinor, Currency: s.Currency, Period: s.Period, StartedOn: s.StartedOn, EndedOn: toNullString(s.EndedOn), Note: toNullString(s.Note)})
		if err != nil {
			return err
		}
		return replaceHistory(ctx, q, id, s)
	})
	if err != nil {
		return 0, fmt.Errorf("create cinema subscription: %w", err)
	}
	return id, nil
}

// UpdateCinemaSubscription replaces a plan, its periods and its prices.
func (r *TrackingRepository) UpdateCinemaSubscription(ctx context.Context, userID int64, s CinemaSubscription) (bool, error) {
	var found bool
	err := r.inTx(ctx, func(q *sqlc.Queries) error {
		n, err := q.UpdateCinemaSubscription(ctx, sqlc.UpdateCinemaSubscriptionParams{ID: s.ID, UserID: userID, Plan: s.Plan, Chain: toNullString(s.Chain),
			PriceMinor: s.PriceMinor, Currency: s.Currency, Period: s.Period, StartedOn: s.StartedOn, EndedOn: toNullString(s.EndedOn), Note: toNullString(s.Note)})
		if err != nil || n == 0 {
			return err
		}
		found = true
		return replaceHistory(ctx, q, s.ID, s)
	})
	if err != nil {
		return false, fmt.Errorf("update cinema subscription: %w", err)
	}
	return found, nil
}

func replaceHistory(ctx context.Context, q *sqlc.Queries, id int64, s CinemaSubscription) error {
	if err := q.DeleteCinemaSubscriptionPeriods(ctx, id); err != nil {
		return err
	}
	if err := q.DeleteCinemaSubscriptionPrices(ctx, id); err != nil {
		return err
	}
	for _, p := range s.Periods {
		if err := q.InsertCinemaSubscriptionPeriod(ctx, sqlc.InsertCinemaSubscriptionPeriodParams{SubscriptionID: id, StartedOn: p.StartedOn, EndedOn: toNullString(p.EndedOn)}); err != nil {
			return err
		}
	}
	for _, p := range s.Prices {
		if err := q.InsertCinemaSubscriptionPrice(ctx, sqlc.InsertCinemaSubscriptionPriceParams{SubscriptionID: id, EffectiveOn: p.From, PriceMinor: p.PriceMinor}); err != nil {
			return err
		}
	}
	return nil
}

func (r *TrackingRepository) inTx(ctx context.Context, fn func(q *sqlc.Queries) error) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := fn(r.q.WithTx(tx)); err != nil {
		return err
	}
	return tx.Commit()
}

// DeleteCinemaSubscription removes a plan; its watches stay, no longer covered.
func (r *TrackingRepository) DeleteCinemaSubscription(ctx context.Context, userID, id int64) (bool, error) {
	n, err := r.q.DeleteCinemaSubscription(ctx, sqlc.DeleteCinemaSubscriptionParams{ID: id, UserID: userID})
	if err != nil {
		return false, fmt.Errorf("delete cinema subscription: %w", err)
	}
	return n > 0, nil
}

// CinemaWatchCost is a cinema watch with what it cost, for the savings analysis.
type CinemaWatchCost struct {
	ID             int64
	Day            string // YYYY-MM-DD local day
	SubscriptionID *int64
	FaceValueMinor *int64
	PriceMinor     *int64
	FeesMinor      *int64
	Currency       *string
	Tickets        int64
	Chain          string // the venue's chain, "" if none
	VenueName      string
}

// CinemaWatchCosts lists a user's cinema watches, oldest first.
func (r *TrackingRepository) CinemaWatchCosts(ctx context.Context, userID int64) ([]CinemaWatchCost, error) {
	rows, err := r.q.SubscriptionCinemaWatches(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("list cinema watch costs: %w", err)
	}
	out := make([]CinemaWatchCost, 0, len(rows))
	for _, row := range rows {
		tickets := int64(1)
		if row.TicketsCount.Valid && row.TicketsCount.Int64 > 0 {
			tickets = row.TicketsCount.Int64
		}
		out = append(out, CinemaWatchCost{ID: row.ID, Day: row.WatchedOn, SubscriptionID: nullInt64Ptr(row.SubscriptionID),
			FaceValueMinor: nullInt64Ptr(row.FaceValueMinor), PriceMinor: nullInt64Ptr(row.PriceMinor), FeesMinor: nullInt64Ptr(row.FeesMinor),
			Currency: nullStringPtr(row.Currency), Tickets: tickets, Chain: row.VenueChain, VenueName: row.VenueName})
	}
	return out, nil
}
