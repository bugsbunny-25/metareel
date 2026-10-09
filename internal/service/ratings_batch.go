package service

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/bugsbunny-25/metareel/internal/repository"
)

// MaxRatingsBatch caps how many titles one batch ratings request may ask for.
const MaxRatingsBatch = 50

// ratingsBatchRefreshLimit is how many uncached titles a batch request
// refreshes from the providers; the rest come back in Missing (the
// ratings.prewarm job keeps charting titles warm).
const ratingsBatchRefreshLimit = 10

// RatingsBatchResponse holds ratings for several titles.
type RatingsBatchResponse struct {
	Items   []*RatingsResponse `json:"items"`
	Missing []string           `json:"missing"` // "kind:id" not rated yet (or every provider failed)
}

// GetRatingsBatch returns ratings for refs: stored ratings within the TTL as
// is, and expired or missing ones refreshed (up to a limit, four at a time).
func (s *RatingsService) GetRatingsBatch(ctx context.Context, refs []TmdbRef) (*RatingsBatchResponse, error) {
	if len(refs) == 0 {
		return nil, &ValidationError{Message: "ids is required, e.g. ids=movie:425,tv:154385"}
	}
	if len(refs) > MaxRatingsBatch {
		return nil, &ValidationError{Message: fmt.Sprintf("too many ids: at most %d", MaxRatingsBatch)}
	}
	refs = dedupeRefs(refs)
	out := &RatingsBatchResponse{Items: make([]*RatingsResponse, len(refs)), Missing: []string{}}

	var (
		wg      sync.WaitGroup
		sem     = make(chan struct{}, 4)
		mu      sync.Mutex
		refresh = 0
		errs    []error
	)
	for i, ref := range refs {
		ident, sources, err := s.repo.GetTitleRatings(ctx, ref.Kind, ref.ID)
		if err != nil {
			return nil, err
		}
		if ident != nil && s.now().Sub(ident.RefreshedAt) < s.ttl {
			out.Items[i] = s.withDataset(ctx, s.buildResponse(ref, ident, sources, true, false))
			continue
		}
		if refresh >= ratingsBatchRefreshLimit {
			if ident != nil {
				out.Items[i] = s.withDataset(ctx, s.buildResponse(ref, ident, sources, true, true))
			}
			continue
		}
		refresh++
		wg.Add(1)
		go func(i int, ref TmdbRef) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			r, err := s.GetRatings(ctx, ref, false)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				errs = append(errs, err)
				return
			}
			out.Items[i] = r
		}(i, ref)
	}
	wg.Wait()

	items := out.Items[:0]
	for i, r := range out.Items {
		if r == nil {
			out.Missing = append(out.Missing, refs[i].String())
			continue
		}
		items = append(items, r)
	}
	out.Items = items
	if len(items) == 0 && len(errs) > 0 {
		return nil, errs[0]
	}
	return out, nil
}

// PrewarmReport summarises a ratings.prewarm run.
type PrewarmReport struct {
	Titles    int `json:"titles"`
	Refreshed int `json:"refreshed"`
	Fresh     int `json:"already_fresh"`
	Failed    int `json:"failed"`
}

const (
	prewarmDays  = 3
	prewarmLimit = 300
	prewarmGap   = 500 * time.Millisecond
)

// Prewarm refreshes expired ratings of titles charting in the last few days,
// so API requests for them are served from the database.
func (s *RatingsService) Prewarm(ctx context.Context, titles *repository.FlixPatrolRepository, runLog func(level, msg string)) (*PrewarmReport, error) {
	logf := func(level, format string, args ...any) {
		if runLog != nil {
			runLog(level, fmt.Sprintf(format, args...))
		}
	}
	since := s.now().UTC().AddDate(0, 0, -prewarmDays)
	refs, err := titles.TmdbRefsChartingSince(ctx, since, prewarmLimit)
	if err != nil {
		return nil, err
	}
	rep := &PrewarmReport{Titles: len(refs)}
	logf("info", "%d matched titles charted since %s", len(refs), since.Format("2006-01-02"))
	for _, ref := range refs {
		if err := ctx.Err(); err != nil {
			return rep, err
		}
		ident, _, err := s.repo.GetTitleRatings(ctx, ref.Kind, ref.ID)
		if err != nil {
			return rep, err
		}
		if ident != nil && s.now().Sub(ident.RefreshedAt) < s.ttl {
			rep.Fresh++
			continue
		}
		if rep.Refreshed+rep.Failed > 0 {
			if err := sleepCtx(ctx, prewarmGap); err != nil {
				return rep, err
			}
		}
		r, err := s.GetRatings(ctx, ref, true)
		if err != nil || r.Stale {
			rep.Failed++
			s.log.Warn("ratings prewarm failed", slog.String("title", ref.String()), slog.Any("err", err))
			continue
		}
		rep.Refreshed++
	}
	logf("info", "Ratings: %d refreshed, %d already fresh, %d failed", rep.Refreshed, rep.Fresh, rep.Failed)
	if rep.Failed > 0 && rep.Refreshed == 0 {
		return rep, fmt.Errorf("every ratings refresh failed (%d)", rep.Failed)
	}
	return rep, nil
}
