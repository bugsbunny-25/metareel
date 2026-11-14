package service

import (
	"context"
	"encoding/json"
	"regexp"
	"slices"
	"strings"
)

// Preferences are what a user sets once and expects on every device. Each
// is optional; an unset one falls back to the client's or the server's
// default. Device-only choices (the theme) are not stored here.
type Preferences struct {
	// Country (ISO 3166-1 alpha-2) and Provider (chart service slug) the app
	// opens with on a device that has no choice of its own yet.
	Country  *string `json:"country,omitempty"`
	Provider *string `json:"provider,omitempty"`
	// Currency (ISO 4217) prefilled for ticket and rental prices.
	Currency *string `json:"currency,omitempty"`
	// ScreenUnit is how screen sizes are entered: m or ft.
	ScreenUnit *string `json:"screen_unit,omitempty"`
	// StaleDays is how long without a watch moves a show part way through
	// from "watching" to "stalled" in GET /me/shows (7-365, default 30).
	StaleDays *int `json:"stale_days,omitempty"`
	// Services are the TMDB provider IDs of the streaming services the user
	// pays for (at most 20): Discover's rotation reads them as subscribed.
	Services []int64 `json:"services,omitempty"`
}

const maxPreferredServices = 20

var providerSlugRe = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// Preferences returns the user's preferences and when they were last saved.
func (s *TrackingService) Preferences(ctx context.Context, userID int64) (map[string]any, error) {
	p, updated, err := s.preferences(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := map[string]any{"preferences": p}
	if updated != "" {
		out["updated_at"] = updated
	}
	return out, nil
}

func (s *TrackingService) preferences(ctx context.Context, userID int64) (Preferences, string, error) {
	raw, updated, err := s.read.UserPreferences(ctx, userID)
	if err != nil {
		return Preferences{}, "", err
	}
	var p Preferences
	if err := json.Unmarshal([]byte(raw), &p); err != nil {
		// Stored by this code, so this should not happen; an unreadable row
		// behaves like no preferences rather than breaking every read.
		s.log.Warn("unreadable user preferences", "user_id", userID, "error", err)
		return Preferences{}, updated, nil
	}
	return p, updated, nil
}

// SetPreferences validates and replaces the user's preferences. Omitted or
// null fields are cleared.
func (s *TrackingService) SetPreferences(ctx context.Context, userID int64, in Preferences) (map[string]any, error) {
	upper := func(v *string) *string {
		if t := trimmedPtr(v); t != nil {
			u := strings.ToUpper(*t)
			return &u
		}
		return nil
	}
	p := Preferences{Country: upper(in.Country), Currency: upper(in.Currency), Provider: trimmedPtr(in.Provider),
		ScreenUnit: trimmedPtr(in.ScreenUnit), StaleDays: in.StaleDays}
	if len(in.Services) > maxPreferredServices {
		return nil, &ValidationError{Message: "at most 20 services"}
	}
	for _, id := range in.Services {
		if id <= 0 {
			return nil, &ValidationError{Message: "services must be TMDB provider IDs"}
		}
		if !slices.Contains(p.Services, id) {
			p.Services = append(p.Services, id)
		}
	}
	switch {
	case p.Country != nil && !countryRe.MatchString(*p.Country):
		return nil, &ValidationError{Message: "country must be a 2-letter ISO 3166-1 code"}
	case p.Provider != nil && (len(*p.Provider) > 40 || !providerSlugRe.MatchString(*p.Provider)):
		return nil, &ValidationError{Message: "provider must be a service slug such as netflix"}
	case p.Currency != nil && !currencyRe.MatchString(*p.Currency):
		return nil, &ValidationError{Message: "currency must be a 3-letter ISO 4217 code, e.g. INR or USD"}
	case p.ScreenUnit != nil && !trackingDimUnits[*p.ScreenUnit]:
		return nil, &ValidationError{Message: "screen_unit must be m or ft"}
	case p.StaleDays != nil && (*p.StaleDays < minStaleDays || *p.StaleDays > maxStaleDays):
		return nil, &ValidationError{Message: "stale_days must be between 7 and 365"}
	}
	data, err := json.Marshal(p)
	if err != nil {
		return nil, err
	}
	if err := s.write.SaveUserPreferences(ctx, userID, string(data)); err != nil {
		return nil, err
	}
	return s.Preferences(ctx, userID)
}
