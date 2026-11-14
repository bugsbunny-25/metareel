package repository

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/bugsbunny-25/metareel/internal/repository/sqlc"
)

// StatsRange bounds an aggregate by local day; empty means unbounded.
type StatsRange struct {
	From string // YYYY-MM-DD
	To   string
}

// StatsTotals is the headline count of what a user watched in a range.
type StatsTotals struct {
	Watches        int64 `json:"watches"`
	MovieWatches   int64 `json:"movie_watches"`
	EpisodeWatches int64 `json:"episode_watches"`
	Rewatches      int64 `json:"rewatches"`
	DistinctTitles int64 `json:"distinct_titles"`
	Minutes        int64 `json:"minutes"`
	Days           int64 `json:"days"`
	Rated          int64 `json:"rated"`
	Liked          int64 `json:"liked"`
}

// StatsTotals counts a user's watching over a range.
func (r *TrackingRepository) StatsTotals(ctx context.Context, userID int64, rng StatsRange) (StatsTotals, error) {
	row, err := r.q.StatsTotals(ctx, sqlc.StatsTotalsParams{
		UserID: userID, FromDay: rng.From, ToDay: rng.To})
	if err != nil {
		return StatsTotals{}, fmt.Errorf("stats totals: %w", err)
	}
	return StatsTotals{
		Watches: row.Watches, MovieWatches: row.MovieWatches, EpisodeWatches: row.EpisodeWatches,
		Rewatches: row.Rewatches, DistinctTitles: row.DistinctTitles, Minutes: row.Minutes,
		Days: row.Days, Rated: row.Rated, Liked: row.Liked,
	}, nil
}

// LabelledStat is one bucket of a breakdown.
type LabelledStat struct {
	Label   string `json:"label"`
	Watches int64  `json:"watches"`
	Minutes int64  `json:"minutes,omitempty"`
}

// StatsByVenueKind splits watching between the cinema, streaming and home.
func (r *TrackingRepository) StatsByVenueKind(ctx context.Context, userID int64, rng StatsRange) ([]LabelledStat, error) {
	rows, err := r.q.StatsByVenueKind(ctx, sqlc.StatsByVenueKindParams{
		UserID: userID, FromDay: rng.From, ToDay: rng.To})
	if err != nil {
		return nil, fmt.Errorf("stats by venue kind: %w", err)
	}
	out := make([]LabelledStat, 0, len(rows))
	for _, row := range rows {
		label := row.VenueKind
		if label == "" {
			label = "unspecified"
		}
		out = append(out, LabelledStat{Label: label, Watches: row.Watches, Minutes: row.Minutes})
	}
	return out, nil
}

// DayStat is one day of the calendar.
type DayStat struct {
	Day     string `json:"day"`
	Watches int64  `json:"watches"`
	Minutes int64  `json:"minutes"`
}

// StatsPerDay is the calendar, oldest day first.
func (r *TrackingRepository) StatsPerDay(ctx context.Context, userID int64, rng StatsRange) ([]DayStat, error) {
	rows, err := r.q.StatsPerDay(ctx, sqlc.StatsPerDayParams{
		UserID: userID, FromDay: rng.From, ToDay: rng.To})
	if err != nil {
		return nil, fmt.Errorf("stats per day: %w", err)
	}
	out := make([]DayStat, 0, len(rows))
	for _, row := range rows {
		out = append(out, DayStat{Day: row.WatchedOn, Watches: row.Watches, Minutes: row.Minutes})
	}
	return out, nil
}

// StatsByHour buckets watching by the local hour it ended. Only watches
// logged to the minute can be placed in an hour.
func (r *TrackingRepository) StatsByHour(ctx context.Context, userID int64, rng StatsRange) ([]LabelledStat, error) {
	rows, err := r.q.StatsByHour(ctx, sqlc.StatsByHourParams{
		UserID: userID, FromDay: rng.From, ToDay: rng.To})
	if err != nil {
		return nil, fmt.Errorf("stats by hour: %w", err)
	}
	out := make([]LabelledStat, 0, len(rows))
	for _, row := range rows {
		out = append(out, LabelledStat{Label: row.Hour, Watches: row.Watches})
	}
	return out, nil
}

// StatsByWeekday buckets by local day of the week ("0" = Sunday).
func (r *TrackingRepository) StatsByWeekday(ctx context.Context, userID int64, rng StatsRange) ([]LabelledStat, error) {
	rows, err := r.q.StatsByWeekday(ctx, sqlc.StatsByWeekdayParams{
		UserID: userID, FromDay: rng.From, ToDay: rng.To})
	if err != nil {
		return nil, fmt.Errorf("stats by weekday: %w", err)
	}
	out := make([]LabelledStat, 0, len(rows))
	for _, row := range rows {
		out = append(out, LabelledStat{Label: row.Weekday, Watches: row.Watches})
	}
	return out, nil
}

// StatsByMonth buckets by calendar month (YYYY-MM).
func (r *TrackingRepository) StatsByMonth(ctx context.Context, userID int64, rng StatsRange) ([]LabelledStat, error) {
	rows, err := r.q.StatsByMonth(ctx, sqlc.StatsByMonthParams{
		UserID: userID, FromDay: rng.From, ToDay: rng.To})
	if err != nil {
		return nil, fmt.Errorf("stats by month: %w", err)
	}
	out := make([]LabelledStat, 0, len(rows))
	for _, row := range rows {
		out = append(out, LabelledStat{Label: row.Month, Watches: row.Watches, Minutes: row.Minutes})
	}
	return out, nil
}

// StatsRatingDistribution counts how often each rating was given.
func (r *TrackingRepository) StatsRatingDistribution(ctx context.Context, userID int64, rng StatsRange) (map[int64]int64, error) {
	rows, err := r.q.StatsRatingDistribution(ctx, sqlc.StatsRatingDistributionParams{
		UserID: userID, FromDay: rng.From, ToDay: rng.To})
	if err != nil {
		return nil, fmt.Errorf("stats rating distribution: %w", err)
	}
	out := make(map[int64]int64, len(rows))
	for _, row := range rows {
		out[row.Rating] = row.Watches
	}
	return out, nil
}

// RatingComparison puts a user's rating next to the crowd's. The outside
// scores are on a 0-10 scale, and nil where nobody has scored it.
type RatingComparison struct {
	Kind       string   `json:"tmdb_kind"`
	TmdbID     string   `json:"tmdb_id"`
	Title      string   `json:"title"`
	UserRating int64    `json:"user_rating"`
	TmdbRating *float64 `json:"tmdb_rating,omitempty"`
	ImdbRating *float64 `json:"imdb_rating,omitempty"`
	RtRating   *float64 `json:"rt_rating,omitempty"`
}

// StatsRatingComparison lists every title the user rated with the crowd's
// scores beside it.
func (r *TrackingRepository) StatsRatingComparison(ctx context.Context, userID int64) ([]RatingComparison, error) {
	rows, err := r.q.StatsRatingComparison(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("stats rating comparison: %w", err)
	}
	out := make([]RatingComparison, 0, len(rows))
	for _, row := range rows {
		c := RatingComparison{
			Kind: row.TmdbKind, TmdbID: row.TmdbID, Title: row.DisplayTitle,
			UserRating: row.UserRating,
			TmdbRating: nullFloatPtr(row.TmdbRating), ImdbRating: nullFloatPtr(row.ImdbRating),
		}
		// -1 is the query's "nobody has scored it" sentinel.
		if row.RtRating >= 0 {
			rt := row.RtRating
			c.RtRating = &rt
		}
		out = append(out, c)
	}
	return out, nil
}

// WatchedTitle is a title watched in a range, with the metadata the genre,
// decade and country breakdowns are built from.
type WatchedTitle struct {
	Kind        string   `json:"tmdb_kind"`
	TmdbID      string   `json:"tmdb_id"`
	Title       string   `json:"title"`
	Watches     int64    `json:"watches"`
	Minutes     int64    `json:"minutes"`
	ReleaseDate string   `json:"release_date,omitempty"`
	Genres      []string `json:"genres,omitempty"`
	Countries   []string `json:"countries,omitempty"`
	Language    string   `json:"language,omitempty"`
}

// StatsWatchedTitles lists the distinct titles watched in a range.
func (r *TrackingRepository) StatsWatchedTitles(ctx context.Context, userID int64, rng StatsRange) ([]WatchedTitle, error) {
	rows, err := r.q.StatsWatchedTitles(ctx, sqlc.StatsWatchedTitlesParams{
		UserID: userID, FromDay: rng.From, ToDay: rng.To})
	if err != nil {
		return nil, fmt.Errorf("stats watched titles: %w", err)
	}
	out := make([]WatchedTitle, 0, len(rows))
	for _, row := range rows {
		out = append(out, WatchedTitle{
			Kind: row.TmdbKind, TmdbID: row.TmdbID, Title: row.DisplayTitle,
			Watches: row.Watches, Minutes: row.Minutes, ReleaseDate: row.ReleaseDate,
			Genres: decodeStringList(row.Genres), Countries: decodeStringList(row.OriginCountries),
			Language: row.OriginalLanguage,
		})
	}
	return out, nil
}

// decodeStringList reads one of tmdb_titles' JSON string arrays.
func decodeStringList(raw string) []string {
	if raw == "" {
		return nil
	}
	var out []string
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil
	}
	return out
}

// PersonStat is one of the people behind what a user watched.
type PersonStat struct {
	PersonID    int64  `json:"person_id"`
	Name        string `json:"name"`
	ProfilePath string `json:"profile_path,omitempty"`
	Titles      int64  `json:"titles"`
	Watches     int64  `json:"watches"`
}

// StatsTopPeople ranks people by how many of a user's titles they worked on.
// job "" counts cast; anything else matches that crew job.
func (r *TrackingRepository) StatsTopPeople(ctx context.Context, userID int64, rng StatsRange, job string, limit int64) ([]PersonStat, error) {
	rows, err := r.q.StatsTopPeople(ctx, sqlc.StatsTopPeopleParams{
		UserID: userID, Job: job, FromDay: rng.From, ToDay: rng.To, Limit: limit})
	if err != nil {
		return nil, fmt.Errorf("stats top people: %w", err)
	}
	out := make([]PersonStat, 0, len(rows))
	for _, row := range rows {
		out = append(out, PersonStat{
			PersonID: row.PersonID, Name: row.Name, ProfilePath: row.ProfilePath,
			Titles: row.Titles, Watches: row.Watches,
		})
	}
	return out, nil
}

// CurrencySpend totals money in one currency. Amounts are minor units and
// are never converted between currencies.
type CurrencySpend struct {
	Currency         string `json:"currency"`
	PriceMinor       int64  `json:"price_minor"`
	FeesMinor        int64  `json:"fees_minor"`
	ConcessionsMinor int64  `json:"concessions_minor"`
	TotalMinor       int64  `json:"total_minor"`
	Watches          int64  `json:"watches"`
	Tickets          int64  `json:"tickets"`
}

// StatsSpendByCurrency totals money per currency.
func (r *TrackingRepository) StatsSpendByCurrency(ctx context.Context, userID int64, rng StatsRange) ([]CurrencySpend, error) {
	rows, err := r.q.StatsSpendByCurrency(ctx, sqlc.StatsSpendByCurrencyParams{
		UserID: userID, FromDay: rng.From, ToDay: rng.To})
	if err != nil {
		return nil, fmt.Errorf("stats spend by currency: %w", err)
	}
	out := make([]CurrencySpend, 0, len(rows))
	for _, row := range rows {
		out = append(out, CurrencySpend{
			Currency: row.Currency, PriceMinor: row.PriceMinor, FeesMinor: row.FeesMinor,
			ConcessionsMinor: row.ConcessionsMinor,
			TotalMinor:       row.PriceMinor + row.FeesMinor + row.ConcessionsMinor,
			Watches:          row.Watches, Tickets: row.Tickets,
		})
	}
	return out, nil
}

// VenueSpend totals money at one cinema, in one currency.
type VenueSpend struct {
	Venue      string `json:"venue"`
	Currency   string `json:"currency"`
	Watches    int64  `json:"watches"`
	TotalMinor int64  `json:"total_minor"`
}

// StatsSpendByVenue totals money per cinema.
func (r *TrackingRepository) StatsSpendByVenue(ctx context.Context, userID int64, rng StatsRange) ([]VenueSpend, error) {
	rows, err := r.q.StatsSpendByVenue(ctx, sqlc.StatsSpendByVenueParams{
		UserID: userID, FromDay: rng.From, ToDay: rng.To})
	if err != nil {
		return nil, fmt.Errorf("stats spend by venue: %w", err)
	}
	out := make([]VenueSpend, 0, len(rows))
	for _, row := range rows {
		out = append(out, VenueSpend{
			Venue: row.Venue, Currency: row.Currency, Watches: row.Watches, TotalMinor: row.TotalMinor})
	}
	return out, nil
}

// FormatStat is how much was watched in one premium format.
type FormatStat struct {
	Slug     string `json:"slug"`
	Label    string `json:"label"`
	Category string `json:"category"`
	Watches  int64  `json:"watches"`
	Minutes  int64  `json:"minutes"`
	LogoURL  string `json:"logo_url,omitempty"`
	// LogoDisplay is "original" or "mono" (draw the logo white).
	LogoDisplay string `json:"logo_display"`
}

// StatsByFormat counts watches and time per premium format.
func (r *TrackingRepository) StatsByFormat(ctx context.Context, userID int64, rng StatsRange) ([]FormatStat, error) {
	rows, err := r.q.StatsByFormat(ctx, sqlc.StatsByFormatParams{
		UserID: userID, FromDay: rng.From, ToDay: rng.To})
	if err != nil {
		return nil, fmt.Errorf("stats by format: %w", err)
	}
	out := make([]FormatStat, 0, len(rows))
	for _, row := range rows {
		out = append(out, FormatStat{
			Slug: row.Slug, Label: row.Label, Category: row.Category,
			Watches: row.Watches, Minutes: row.Minutes, LogoURL: FormatLogoURL(row.Slug, nullStringPtr(row.LogoVersion)),
			LogoDisplay: row.LogoDisplay})
	}
	return out, nil
}

// ChartHit is a watchlist title on a Top 10.
type ChartHit struct {
	Kind     string `json:"tmdb_kind"`
	TmdbID   string `json:"tmdb_id"`
	Title    string `json:"title"`
	Country  string `json:"country"`
	Provider string `json:"provider"`
	Category string `json:"category"`
	BestRank int64  `json:"best_rank"`
	RankedOn string `json:"ranked_on"`
}

// WatchlistInCharts finds watchlist titles that have charted since a date.
func (r *TrackingRepository) WatchlistInCharts(ctx context.Context, userID int64, since, country string, limit int64) ([]ChartHit, error) {
	rows, err := r.q.WatchlistInCharts(ctx, sqlc.WatchlistInChartsParams{
		UserID: userID, Since: since, Country: country, Limit: limit})
	if err != nil {
		return nil, fmt.Errorf("watchlist in charts: %w", err)
	}
	out := make([]ChartHit, 0, len(rows))
	for _, row := range rows {
		out = append(out, ChartHit{
			Kind: row.TmdbKind, TmdbID: row.TmdbID, Title: row.DisplayTitle,
			Country: toString(row.Country), Provider: row.StreamingProvider, Category: row.Category,
			BestRank: row.BestRank, RankedOn: row.RankedOn,
		})
	}
	return out, nil
}

// StreamingHit is a watchlist title available in a country.
type StreamingHit struct {
	Kind         string `json:"tmdb_kind"`
	TmdbID       string `json:"tmdb_id"`
	Title        string `json:"title"`
	ProviderID   int64  `json:"provider_id"`
	ProviderName string `json:"provider_name"`
	Monetization string `json:"monetization"`
	Link         string `json:"link,omitempty"`
}

// WatchlistStreamingNow finds watchlist titles available in a country.
func (r *TrackingRepository) WatchlistStreamingNow(ctx context.Context, userID int64, country, monetization string, limit int64) ([]StreamingHit, error) {
	rows, err := r.q.WatchlistStreamingNow(ctx, sqlc.WatchlistStreamingNowParams{
		UserID: userID, Country: country, Monetization: monetization, Limit: limit})
	if err != nil {
		return nil, fmt.Errorf("watchlist streaming now: %w", err)
	}
	out := make([]StreamingHit, 0, len(rows))
	for _, row := range rows {
		out = append(out, StreamingHit{
			Kind: row.TmdbKind, TmdbID: row.TmdbID, Title: row.DisplayTitle,
			ProviderID: row.ProviderID, ProviderName: row.ProviderName,
			Monetization: row.Monetization, Link: row.Link,
		})
	}
	return out, nil
}

// LeftHit is a watchlist title that came off a service.
type LeftHit struct {
	Kind         string `json:"tmdb_kind"`
	TmdbID       string `json:"tmdb_id"`
	Title        string `json:"title"`
	ProviderName string `json:"provider_name"`
	Monetization string `json:"monetization"`
	Country      string `json:"country"`
	ObservedAt   string `json:"observed_at"`
}

// WatchlistRecentlyLeft finds watchlist titles that came off a service.
// metareel records what has happened, not what is scheduled, so this is
// what has already gone rather than what is leaving soon.
func (r *TrackingRepository) WatchlistRecentlyLeft(ctx context.Context, userID int64, country, since string, limit int64) ([]LeftHit, error) {
	rows, err := r.q.WatchlistRecentlyLeft(ctx, sqlc.WatchlistRecentlyLeftParams{
		UserID: userID, Country: country, Since: since, Limit: limit})
	if err != nil {
		return nil, fmt.Errorf("watchlist recently left: %w", err)
	}
	out := make([]LeftHit, 0, len(rows))
	for _, row := range rows {
		out = append(out, LeftHit{
			Kind: row.TmdbKind, TmdbID: row.TmdbID, Title: row.DisplayTitle,
			ProviderName: row.ProviderName, Monetization: row.Monetization,
			Country: row.Country, ObservedAt: row.ObservedAt,
		})
	}
	return out, nil
}
