package service

import (
	"context"
	"sync"
	"time"

	"github.com/bugsbunny-25/metareel/internal/repository"
)

// chartProviderTMDB maps each FlixPatrol chart provider to its TMDB watch
// provider IDs: the main one first, then its other tiers (ads, premium,
// regional) that count as the same service. Store channels (e.g. "HBO Max
// Amazon Channel") are separate services and not listed.
var chartProviderTMDB = map[string][]int64{
	"netflix":        {8, 1796, 175}, // Netflix, Standard with Ads, Kids
	"hbo-max":        {1899, 2284},   // HBO Max, HBO Max on U-Next (JP)
	"disney":         {337, 122},     // Disney Plus, Disney+ (Hotstar regions)
	"amazon-prime":   {119, 9, 2100}, // Prime Video (two regional IDs), with Ads
	"paramount-plus": {531, 2303, 2304, 2616},
	"peacock":        {386, 387},
	"apple-tv":       {350},
}

var tmdbChartProvider = func() map[int64]string {
	out := map[int64]string{}
	for slug, ids := range chartProviderTMDB {
		for _, id := range ids {
			out[id] = slug
		}
	}
	return out
}()

// ChartProviderOf returns the FlixPatrol chart provider slug a TMDB watch
// provider belongs to ("" = none).
func ChartProviderOf(tmdbProviderID int64) string {
	return tmdbChartProvider[tmdbProviderID]
}

// ProviderLogos maps TMDB watch provider IDs to their logo paths, read from
// stored watch providers and kept in memory for providerLogosTTL.
type ProviderLogos struct {
	metadata *repository.MetadataRepository
	now      func() time.Time

	mu      sync.Mutex
	logos   map[int64]string
	fetched time.Time
}

const providerLogosTTL = 6 * time.Hour

func NewProviderLogos(metadata *repository.MetadataRepository) *ProviderLogos {
	return &ProviderLogos{metadata: metadata, now: time.Now}
}

// Get returns the logo map (empty, never nil, when nothing is stored). A
// failed refresh keeps serving the previous map.
func (p *ProviderLogos) Get(ctx context.Context) (map[int64]string, error) {
	if p == nil {
		return map[int64]string{}, nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.logos != nil && p.now().Sub(p.fetched) < providerLogosTTL {
		return p.logos, nil
	}
	logos, err := p.metadata.ProviderLogos(ctx)
	if err != nil {
		if p.logos != nil {
			return p.logos, nil
		}
		return nil, err
	}
	p.logos, p.fetched = logos, p.now()
	return logos, nil
}

// logoPtr returns the logo of id, or nil.
func logoPtr(logos map[int64]string, id int64) *string {
	if l, ok := logos[id]; ok && l != "" {
		return &l
	}
	return nil
}
