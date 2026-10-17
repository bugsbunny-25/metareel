package service

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/bugsbunny-25/metareel/internal/client"
	"github.com/bugsbunny-25/metareel/internal/repository"
)

// justWatchOffers is the part of the JustWatch client the metadata refresh
// uses to find each watch provider's own page for a title.
type justWatchOffers interface {
	GetOffers(ctx context.Context, nodeID string, countries []string) (map[string][]client.JWOffer, bool, error)
	GetTitlesByTopSearchPopular(ctx context.Context, searchQuery string, first int, country, language string, objectType client.ObjectType, packages []client.Package) (*client.GetTitlesByTopSearchPopularQuery, error)
}

// addOfferURLs fills d's watch providers' URL from the title's JustWatch
// offers (JustWatch package IDs are TMDB provider IDs). It returns whether
// the offers were looked up (found or not) and the JustWatch node ID when it
// was found by searching. On a JustWatch error the URLs stored by the last
// refresh are kept.
func (s *TitleMetadataService) addOfferURLs(ctx context.Context, ref repository.TmdbRef, d *client.TMDBFullDetails) (checked bool, searchedID string) {
	if s.justWatch == nil {
		return false, ""
	}
	if len(d.WatchProviders) == 0 {
		return true, ""
	}
	countries := make([]string, 0, 8)
	seen := map[string]bool{}
	for _, p := range d.WatchProviders {
		if !seen[p.Country] {
			seen[p.Country] = true
			countries = append(countries, p.Country)
		}
	}
	offers, nodeID, searched, err := s.justWatchOffersFor(ctx, ref, d, countries)
	if err != nil {
		s.log.Warn("justwatch offers failed", slog.String("ref", ref.String()), slog.Any("err", err))
		s.keepStoredOfferURLs(ctx, ref, d)
		return false, ""
	}
	if nodeID == "" {
		return true, ""
	}
	applyOfferURLs(d.WatchProviders, offers)
	if searched {
		return true, nodeID
	}
	return true, ""
}

// justWatchOffersFor finds the title on JustWatch (stored node ID, else a
// search whose hit must carry the same TMDB ID) and fetches its offers.
// nodeID is "" when JustWatch doesn't know the title.
func (s *TitleMetadataService) justWatchOffersFor(ctx context.Context, ref repository.TmdbRef, d *client.TMDBFullDetails, countries []string) (offers map[string][]client.JWOffer, nodeID string, searched bool, err error) {
	meta, err := s.repo.Get(ctx, ref)
	if err != nil {
		return nil, "", false, err
	}
	if meta != nil && meta.JustWatchID != nil && *meta.JustWatchID != "" {
		offers, found, err := s.justWatch.GetOffers(ctx, *meta.JustWatchID, countries)
		if err != nil {
			return nil, "", false, fmt.Errorf("justwatch node %s: %w", *meta.JustWatchID, err)
		}
		if found {
			return offers, *meta.JustWatchID, false, nil
		}
	}
	nodeID, err = s.searchJustWatch(ctx, ref, d)
	if err != nil || nodeID == "" {
		return nil, "", false, err
	}
	offers, found, err := s.justWatch.GetOffers(ctx, nodeID, countries)
	if err != nil {
		return nil, "", false, fmt.Errorf("justwatch node %s: %w", nodeID, err)
	}
	if !found {
		return nil, "", false, nil
	}
	return offers, nodeID, true, nil
}

// searchJustWatch searches the title's name (then its original name) and
// returns the node of the hit with the same TMDB ID, or "".
func (s *TitleMetadataService) searchJustWatch(ctx context.Context, ref repository.TmdbRef, d *client.TMDBFullDetails) (string, error) {
	objectType := client.ObjectTypeMovie
	if ref.Kind == "tv" {
		objectType = client.ObjectTypeTVShow
	}
	names := []string{d.Title}
	if d.OriginalTitle != "" && d.OriginalTitle != d.Title {
		names = append(names, d.OriginalTitle)
	}
	for _, name := range names {
		if name == "" {
			continue
		}
		res, err := s.justWatch.GetTitlesByTopSearchPopular(ctx, name, 10, justWatchCountry, "en", objectType, nil)
		if err != nil {
			return "", fmt.Errorf("justwatch search %q: %w", name, err)
		}
		for _, e := range res.PoupularTitles.Edges {
			if id := e.Node.Content.ExternalIds.TmdbId; id != nil && *id == ref.ID && e.Node.Id != "" {
				return e.Node.Id, nil
			}
		}
	}
	return "", nil
}

// applyOfferURLs sets each provider's URL from the offer of the same
// country, provider and monetization, else of the same country and provider.
func applyOfferURLs(providers []client.TMDBWatchProvider, offers map[string][]client.JWOffer) {
	type key struct {
		country, monetization string
		packageID             int
	}
	exact := map[key]string{}
	for country, list := range offers {
		for _, o := range list {
			for _, k := range []key{{country, o.Monetization, o.PackageID}, {country, "", o.PackageID}} {
				if _, ok := exact[k]; !ok {
					exact[k] = o.URL
				}
			}
		}
	}
	for i := range providers {
		p := &providers[i]
		if u, ok := exact[key{p.Country, p.Monetization, p.ProviderID}]; ok {
			p.URL = u
		} else if u, ok := exact[key{p.Country, "", p.ProviderID}]; ok {
			p.URL = u
		}
	}
}

// keepStoredOfferURLs copies the URLs stored by the last refresh onto d's
// providers, so a failed JustWatch request doesn't drop them.
func (s *TitleMetadataService) keepStoredOfferURLs(ctx context.Context, ref repository.TmdbRef, d *client.TMDBFullDetails) {
	stored, err := s.repo.WatchProviders(ctx, ref, "")
	if err != nil {
		s.log.Warn("read stored watch providers failed", slog.String("ref", ref.String()), slog.Any("err", err))
		return
	}
	offers := map[string][]client.JWOffer{}
	for _, p := range stored {
		if p.URL != nil {
			offers[p.Country] = append(offers[p.Country], client.JWOffer{Monetization: p.Monetization, PackageID: int(p.ProviderID), URL: *p.URL})
		}
	}
	applyOfferURLs(d.WatchProviders, offers)
}
