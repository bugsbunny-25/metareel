package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"

	"github.com/bugsbunny-25/metareel/internal/client"
	"github.com/bugsbunny-25/metareel/internal/repository/sqlc"
)

// Watch provider events (watch_provider_events.event).
const (
	OfferAdded   = "added"
	OfferRemoved = "removed"
)

// offerKey identifies one offer of a title: a provider, in a country, with a
// monetization type.
type offerKey struct {
	Country      string
	Monetization string
	ProviderID   int64
}

// storedOffers returns a title's stored watch providers by offer and when
// they were fetched; check is nil when the title's details were never
// fetched (no baseline to compare with).
func storedOffers(ctx context.Context, q *sqlc.Queries, ref TmdbRef) (map[offerKey]string, *string, error) {
	t, err := q.GetTmdbTitle(ctx, sqlc.GetTmdbTitleParams{TmdbKind: ref.Kind, TmdbID: ref.ID})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, fmt.Errorf("get tmdb title %s: %w", ref, err)
	}
	if !t.DetailsFetchedAt.Valid {
		return nil, nil, nil
	}
	rows, err := q.ListTmdbWatchProviders(ctx, sqlc.ListTmdbWatchProvidersParams{TmdbKind: ref.Kind, TmdbID: ref.ID})
	if err != nil {
		return nil, nil, fmt.Errorf("list watch providers of %s: %w", ref, err)
	}
	out := make(map[offerKey]string, len(rows))
	for _, r := range rows {
		out[offerKey{Country: toString(r.Country), Monetization: r.Monetization, ProviderID: r.ProviderID}] = r.ProviderName
	}
	check := t.DetailsFetchedAt.Time.UTC().Format("2006-01-02 15:04:05")
	return out, &check, nil
}

// recordOfferChanges stores an event per offer that appeared or disappeared
// since the previous refresh (at prevCheck).
func recordOfferChanges(ctx context.Context, q *sqlc.Queries, ref TmdbRef, prev map[offerKey]string, now []client.TMDBWatchProvider, prevCheck string) error {
	type change struct {
		key   offerKey
		name  string
		event string
	}
	var changes []change
	cur := make(map[offerKey]bool, len(now))
	for _, p := range now {
		k := offerKey{Country: p.Country, Monetization: p.Monetization, ProviderID: int64(p.ProviderID)}
		if cur[k] {
			continue
		}
		cur[k] = true
		if _, ok := prev[k]; !ok {
			changes = append(changes, change{k, p.ProviderName, OfferAdded})
		}
	}
	for k, name := range prev {
		if !cur[k] {
			changes = append(changes, change{k, name, OfferRemoved})
		}
	}
	sort.Slice(changes, func(i, j int) bool {
		a, b := changes[i].key, changes[j].key
		if a.Country != b.Country {
			return a.Country < b.Country
		}
		if a.ProviderID != b.ProviderID {
			return a.ProviderID < b.ProviderID
		}
		return a.Monetization < b.Monetization
	})
	for _, c := range changes {
		if err := q.InsertWatchProviderEvent(ctx, sqlc.InsertWatchProviderEventParams{
			TmdbKind: ref.Kind, TmdbID: ref.ID, Country: c.key.Country, Monetization: c.key.Monetization,
			ProviderID: c.key.ProviderID, ProviderName: c.name, Event: c.event,
			PreviousCheckAt: sql.NullString{String: prevCheck, Valid: true},
		}); err != nil {
			return fmt.Errorf("record watch provider %s of %s: %w", c.event, ref, err)
		}
	}
	return nil
}

// HistoryRepository stores and reads the history of ratings, TMDB
// popularity and watch providers.
type HistoryRepository struct {
	db *sql.DB
	q  *sqlc.Queries
}

func NewHistoryRepository(db *sql.DB) *HistoryRepository {
	return &HistoryRepository{db: db, q: sqlc.New(db)}
}

// PopularityPoint is a title's TMDB popularity on a day.
type PopularityPoint struct {
	Ref        TmdbRef `json:"-"`
	Date       string  `json:"date"`
	Popularity float64 `json:"popularity"`
}

// KnownTmdbRefs returns every stored TMDB title as "kind:id".
func (r *HistoryRepository) KnownTmdbRefs(ctx context.Context) (map[string]bool, error) {
	rows, err := r.q.ListKnownTmdbRefs(ctx)
	if err != nil {
		return nil, fmt.Errorf("list tmdb titles: %w", err)
	}
	out := make(map[string]bool, len(rows))
	for _, row := range rows {
		out[row.TmdbKind+":"+row.TmdbID] = true
	}
	return out, nil
}

// SavePopularity stores popularity points in one transaction.
func (r *HistoryRepository) SavePopularity(ctx context.Context, points []PopularityPoint) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	q := r.q.WithTx(tx)
	for _, p := range points {
		if err := q.RecordTmdbPopularity(ctx, sqlc.RecordTmdbPopularityParams{
			TmdbKind: p.Ref.Kind, TmdbID: p.Ref.ID, CapturedOn: p.Date, Popularity: p.Popularity,
		}); err != nil {
			return fmt.Errorf("record popularity of %s: %w", p.Ref, err)
		}
	}
	return tx.Commit()
}

// RatingPoint is a rating value from a date on (until the next point).
type RatingPoint struct {
	Date  string  `json:"date"`
	Value float64 `json:"value"`
	Votes *int64  `json:"votes"`
}

// IMDbHistory returns an IMDb ID's points up to toDate (YYYY-MM-DD).
func (r *HistoryRepository) IMDbHistory(ctx context.Context, imdbID, toDate string) ([]RatingPoint, error) {
	rows, err := r.q.ListIMDbRatingHistory(ctx, sqlc.ListIMDbRatingHistoryParams{ImdbID: imdbID, ToDate: toDate})
	if err != nil {
		return nil, fmt.Errorf("list imdb rating history: %w", err)
	}
	out := make([]RatingPoint, 0, len(rows))
	for _, row := range rows {
		votes := row.NumVotes
		out = append(out, RatingPoint{Date: row.CapturedOn, Value: row.AverageRating, Votes: &votes})
	}
	return out, nil
}

// RatingSeries is one provider's history of one rating source.
type RatingSeries struct {
	Provider string        `json:"provider"`
	Source   string        `json:"source"`
	Points   []RatingPoint `json:"points"`
}

// RatingHistory returns a title's rating-source series up to toDate.
func (r *HistoryRepository) RatingHistory(ctx context.Context, ref TmdbRef, toDate string) ([]RatingSeries, error) {
	rows, err := r.q.ListTitleRatingHistory(ctx, sqlc.ListTitleRatingHistoryParams{TmdbKind: ref.Kind, TmdbID: ref.ID, ToDate: toDate})
	if err != nil {
		return nil, fmt.Errorf("list rating history: %w", err)
	}
	var out []RatingSeries
	for _, row := range rows {
		if n := len(out); n == 0 || out[n-1].Provider != row.Provider || out[n-1].Source != row.Source {
			out = append(out, RatingSeries{Provider: row.Provider, Source: row.Source})
		}
		s := &out[len(out)-1]
		var votes *int64
		if row.Votes.Valid {
			v := row.Votes.Int64
			votes = &v
		}
		s.Points = append(s.Points, RatingPoint{Date: row.CapturedOn, Value: row.Value, Votes: votes})
	}
	return out, nil
}

// PopularityHistory returns a title's TMDB popularity between two dates.
func (r *HistoryRepository) PopularityHistory(ctx context.Context, ref TmdbRef, fromDate, toDate string) ([]PopularityPoint, error) {
	rows, err := r.q.ListTmdbPopularityHistory(ctx, sqlc.ListTmdbPopularityHistoryParams{
		TmdbKind: ref.Kind, TmdbID: ref.ID, FromDate: fromDate, ToDate: toDate,
	})
	if err != nil {
		return nil, fmt.Errorf("list popularity history: %w", err)
	}
	out := make([]PopularityPoint, 0, len(rows))
	for _, row := range rows {
		out = append(out, PopularityPoint{Date: row.CapturedOn, Popularity: row.Popularity})
	}
	return out, nil
}

// OfferEvent is a watch provider offer that appeared or disappeared.
type OfferEvent struct {
	ID           int64  `json:"id"`
	TmdbKind     string `json:"tmdb_kind"`
	TmdbID       string `json:"tmdb_id"`
	Title        string `json:"title,omitempty"`
	ReleaseDate  string `json:"release_date,omitempty"`
	PosterPath   string `json:"poster_path,omitempty"`
	Country      string `json:"country"`
	Monetization string `json:"monetization"`
	ProviderID   int64  `json:"provider_id"`
	ProviderName string `json:"provider_name"`
	Event        string `json:"event"`
	// The change happened after PreviousCheckAt and by ObservedAt (UTC,
	// "YYYY-MM-DD HH:MM:SS").
	PreviousCheckAt string `json:"previous_check_at,omitempty"`
	ObservedAt      string `json:"observed_at"`
}

// TitleOfferEvents returns a title's offer changes, newest first.
func (r *HistoryRepository) TitleOfferEvents(ctx context.Context, ref TmdbRef, country string, limit int64) ([]OfferEvent, error) {
	rows, err := r.q.ListTitleWatchProviderEvents(ctx, sqlc.ListTitleWatchProviderEventsParams{
		TmdbKind: ref.Kind, TmdbID: ref.ID, Country: country, Limit: limit,
	})
	if err != nil {
		return nil, fmt.Errorf("list watch provider events: %w", err)
	}
	out := make([]OfferEvent, 0, len(rows))
	for _, e := range rows {
		out = append(out, OfferEvent{
			ID: e.ID, TmdbKind: e.TmdbKind, TmdbID: e.TmdbID, Country: e.Country, Monetization: e.Monetization,
			ProviderID: e.ProviderID, ProviderName: e.ProviderName, Event: e.Event,
			PreviousCheckAt: e.PreviousCheckAt, ObservedAt: e.ObservedAt,
		})
	}
	return out, nil
}

// OfferEventFilter narrows OfferEvents; zero values match all.
type OfferEventFilter struct {
	Since        string // UTC "YYYY-MM-DD HH:MM:SS"
	Country      string
	Event        string
	Monetization string
	ProviderID   int64
	Limit        int64
}

// OfferEvents returns offer changes across titles, newest first.
func (r *HistoryRepository) OfferEvents(ctx context.Context, f OfferEventFilter) ([]OfferEvent, error) {
	rows, err := r.q.ListWatchProviderEvents(ctx, sqlc.ListWatchProviderEventsParams{
		Since: f.Since, Country: f.Country, Event: f.Event, Monetization: f.Monetization,
		ProviderID: f.ProviderID, Limit: f.Limit,
	})
	if err != nil {
		return nil, fmt.Errorf("list watch provider events: %w", err)
	}
	out := make([]OfferEvent, 0, len(rows))
	for _, e := range rows {
		out = append(out, OfferEvent{
			ID: e.ID, TmdbKind: e.TmdbKind, TmdbID: e.TmdbID, Title: e.Title, ReleaseDate: e.ReleaseDate,
			PosterPath: e.PosterPath, Country: e.Country, Monetization: e.Monetization,
			ProviderID: e.ProviderID, ProviderName: e.ProviderName, Event: e.Event,
			PreviousCheckAt: e.PreviousCheckAt, ObservedAt: e.ObservedAt,
		})
	}
	return out, nil
}

// PageviewTarget is a title with a Wikipedia article and the last day of
// views stored ("" = none yet).
type PageviewTarget struct {
	Ref     TmdbRef
	Article string
	LastDay string
}

// PageviewTargets lists every TMDB title with an English Wikipedia article.
func (r *HistoryRepository) PageviewTargets(ctx context.Context) ([]PageviewTarget, error) {
	rows, err := r.q.ListPageviewTargets(ctx)
	if err != nil {
		return nil, fmt.Errorf("list pageview targets: %w", err)
	}
	out := make([]PageviewTarget, 0, len(rows))
	for _, row := range rows {
		out = append(out, PageviewTarget{Ref: TmdbRef{Kind: row.TmdbKind, ID: row.TmdbID}, Article: row.Article, LastDay: row.LastDay})
	}
	return out, nil
}

// PageviewPoint is a day's views.
type PageviewPoint struct {
	Date  string `json:"date"`
	Views int64  `json:"views"`
}

// SavePageviews stores a title's daily views on a project in one transaction.
func (r *HistoryRepository) SavePageviews(ctx context.Context, ref TmdbRef, project string, points []PageviewPoint) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	q := r.q.WithTx(tx)
	for _, p := range points {
		if err := q.SavePageview(ctx, sqlc.SavePageviewParams{TmdbKind: ref.Kind, TmdbID: ref.ID, Project: project, Day: p.Date, Views: p.Views}); err != nil {
			return fmt.Errorf("save pageviews of %s: %w", ref, err)
		}
	}
	return tx.Commit()
}

// Pageviews returns a title's daily views on a project between two days.
func (r *HistoryRepository) Pageviews(ctx context.Context, ref TmdbRef, project, fromDate, toDate string) ([]PageviewPoint, error) {
	rows, err := r.q.ListPageviews(ctx, sqlc.ListPageviewsParams{TmdbKind: ref.Kind, TmdbID: ref.ID, Project: project, FromDate: fromDate, ToDate: toDate})
	if err != nil {
		return nil, fmt.Errorf("list pageviews: %w", err)
	}
	out := make([]PageviewPoint, 0, len(rows))
	for _, row := range rows {
		out = append(out, PageviewPoint{Date: row.Day, Views: row.Views})
	}
	return out, nil
}
