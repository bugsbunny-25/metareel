package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

// JWOffer is one way to watch a title on JustWatch. PackageID is the same
// number as TMDB's watch provider_id (TMDB's provider data is JustWatch's).
type JWOffer struct {
	Monetization string // flatrate | free | ads | rent | buy (lowercased monetizationType)
	PackageID    int
	URL          string // the service's own page for the title (standardWebURL)
}

var (
	jwNodeIDRe  = regexp.MustCompile(`^t[ms][0-9]+$`)
	jwCountryRe = regexp.MustCompile(`^[A-Z]{2}$`)
)

// GetOffers fetches a title's offers in every given country (ISO 3166-1
// alpha-2) in one request, each country as an alias of the node's offers.
// found is false when JustWatch has no such node. Invalid country codes are
// skipped.
func (c *JustWatchClient) GetOffers(ctx context.Context, nodeID string, countries []string) (offers map[string][]JWOffer, found bool, err error) {
	if !jwNodeIDRe.MatchString(nodeID) {
		return nil, false, fmt.Errorf("invalid justwatch node id %q", nodeID)
	}
	var b strings.Builder
	b.WriteString("query GetOffers($id: ID!) { node(id: $id) { id ... on MovieOrShow {")
	seen := map[string]bool{}
	for _, cc := range countries {
		cc = strings.ToUpper(cc)
		if !jwCountryRe.MatchString(cc) || seen[cc] {
			continue
		}
		seen[cc] = true
		// Aliases and enum values are the validated country code itself.
		fmt.Fprintf(&b, " %s: offers(country: %s, platform: WEB) { monetizationType standardWebURL package { packageId } }", cc, cc)
	}
	b.WriteString(" } } }")
	if len(seen) == 0 {
		return map[string][]JWOffer{}, true, nil
	}
	raw, err := c.client.ExecRaw(ctx, b.String(), map[string]any{"id": nodeID})
	if err != nil {
		return nil, false, err
	}
	var resp struct {
		Node map[string]json.RawMessage `json:"node"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, false, fmt.Errorf("decode justwatch offers: %w", err)
	}
	if resp.Node == nil {
		return nil, false, nil
	}
	offers = map[string][]JWOffer{}
	for cc := range seen {
		var list []struct {
			MonetizationType string  `json:"monetizationType"`
			StandardWebURL   *string `json:"standardWebURL"`
			Package          struct {
				PackageID int `json:"packageId"`
			} `json:"package"`
		}
		if msg, ok := resp.Node[cc]; ok && len(msg) > 0 {
			if err := json.Unmarshal(msg, &list); err != nil {
				return nil, false, fmt.Errorf("decode justwatch offers for %s: %w", cc, err)
			}
		}
		for _, o := range list {
			if o.StandardWebURL == nil || !isWebURL(*o.StandardWebURL) {
				continue
			}
			offers[cc] = append(offers[cc], JWOffer{
				Monetization: strings.ToLower(o.MonetizationType), PackageID: o.Package.PackageID, URL: *o.StandardWebURL,
			})
		}
	}
	return offers, true, nil
}

// isWebURL accepts absolute http(s) URLs only: offers end up as links in
// clients.
func isWebURL(s string) bool {
	u, err := url.Parse(s)
	return err == nil && (u.Scheme == "https" || u.Scheme == "http") && u.Host != ""
}
