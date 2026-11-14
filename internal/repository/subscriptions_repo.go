package repository

import (
	"context"
	"fmt"

	"github.com/bugsbunny-25/metareel/internal/repository/sqlc"
)

// CinemaSubscription is a user's cinema plan (AMC Stubs A-List, Regal
// Unlimited, ...): what it costs per period and when it ran.
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
}

func subscriptionFrom(r sqlc.CinemaSubscription) CinemaSubscription {
	return CinemaSubscription{ID: r.ID, Plan: r.Plan, Chain: nullStringPtr(r.Chain), PriceMinor: r.PriceMinor, Currency: r.Currency,
		Period: r.Period, StartedOn: r.StartedOn, EndedOn: nullStringPtr(r.EndedOn), Note: nullStringPtr(r.Note),
		CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt}
}

// CinemaSubscriptions lists a user's plans, current ones first.
func (r *TrackingRepository) CinemaSubscriptions(ctx context.Context, userID int64) ([]CinemaSubscription, error) {
	rows, err := r.q.ListCinemaSubscriptions(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("list cinema subscriptions: %w", err)
	}
	out := make([]CinemaSubscription, 0, len(rows))
	for _, row := range rows {
		out = append(out, subscriptionFrom(row))
	}
	return out, nil
}

// CinemaSubscription returns one of a user's plans; sql.ErrNoRows if it isn't theirs.
func (r *TrackingRepository) CinemaSubscription(ctx context.Context, userID, id int64) (CinemaSubscription, error) {
	row, err := r.q.GetCinemaSubscription(ctx, sqlc.GetCinemaSubscriptionParams{ID: id, UserID: userID})
	if err != nil {
		return CinemaSubscription{}, err
	}
	return subscriptionFrom(row), nil
}

func (r *TrackingRepository) CreateCinemaSubscription(ctx context.Context, userID int64, s CinemaSubscription) (int64, error) {
	id, err := r.q.CreateCinemaSubscription(ctx, sqlc.CreateCinemaSubscriptionParams{UserID: userID, Plan: s.Plan, Chain: toNullString(s.Chain),
		PriceMinor: s.PriceMinor, Currency: s.Currency, Period: s.Period, StartedOn: s.StartedOn, EndedOn: toNullString(s.EndedOn), Note: toNullString(s.Note)})
	if err != nil {
		return 0, fmt.Errorf("create cinema subscription: %w", err)
	}
	return id, nil
}

func (r *TrackingRepository) UpdateCinemaSubscription(ctx context.Context, userID int64, s CinemaSubscription) (bool, error) {
	n, err := r.q.UpdateCinemaSubscription(ctx, sqlc.UpdateCinemaSubscriptionParams{ID: s.ID, UserID: userID, Plan: s.Plan, Chain: toNullString(s.Chain),
		PriceMinor: s.PriceMinor, Currency: s.Currency, Period: s.Period, StartedOn: s.StartedOn, EndedOn: toNullString(s.EndedOn), Note: toNullString(s.Note)})
	if err != nil {
		return false, fmt.Errorf("update cinema subscription: %w", err)
	}
	return n > 0, nil
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
