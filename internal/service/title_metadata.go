package service

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/bugsbunny-25/metareel/internal/client"
	"github.com/bugsbunny-25/metareel/internal/repository"
)

// TitleMetadataService keeps TMDB metadata + watch providers and Wikidata
// IDs of matched titles up to date (the titles.metadata job), and fetches
// them for a title right after it is matched.
type TitleMetadataService struct {
	log      *slog.Logger
	repo     *repository.MetadataRepository
	tmdb     *client.TMDBClient
	wikidata *client.WikidataSPARQL
}

func NewTitleMetadataService(log *slog.Logger, repo *repository.MetadataRepository, tmdbClient *client.TMDBClient, wikidata *client.WikidataSPARQL) *TitleMetadataService {
	return &TitleMetadataService{log: log, repo: repo, tmdb: tmdbClient, wikidata: wikidata}
}

const (
	// Charting titles' watch providers change; refresh them often.
	metadataRecentWindow = "-3 days"
	metadataStaleWindow  = "-30 days"
	wikidataWindow       = "-30 days"
	metadataBatch        = 300
	wikidataBatch        = 500
	tmdbRequestGap       = 250 * time.Millisecond
)

// RefreshDetails fetches and stores one title's TMDB details and watch
// providers. It returns nil, nil when TMDB is not configured or has no such
// title.
func (s *TitleMetadataService) RefreshDetails(ctx context.Context, ref repository.TmdbRef) (*client.TMDBFullDetails, error) {
	if !s.tmdb.Enabled() {
		return nil, nil
	}
	d, err := s.tmdb.FullDetails(ctx, ref.Kind, ref.ID)
	if err != nil || d == nil {
		return nil, err
	}
	if err := s.repo.SaveDetails(ctx, d); err != nil {
		return nil, err
	}
	return d, nil
}

// LookupWikidata batch-fetches Wikidata IDs for refs, stores them (also
// recording refs Wikidata doesn't know, so they wait for the next window)
// and returns what was found keyed by ref.String().
func (s *TitleMetadataService) LookupWikidata(ctx context.Context, refs []repository.TmdbRef) (map[string]client.WikidataIDs, error) {
	found := map[string]client.WikidataIDs{}
	if s.wikidata == nil || len(refs) == 0 {
		return found, nil
	}
	byKind := map[string][]string{}
	for _, r := range refs {
		byKind[r.Kind] = append(byKind[r.Kind], r.ID)
	}
	for kind, ids := range byKind {
		res, err := s.wikidata.LookupByTMDB(ctx, kind, ids)
		if err != nil {
			return found, err
		}
		for _, id := range ids {
			ref := repository.TmdbRef{Kind: kind, ID: id}
			w := res[id]
			if err := s.repo.SaveWikidata(ctx, ref, w); err != nil {
				return found, err
			}
			if w.WikidataID != "" {
				found[ref.String()] = w
			}
		}
	}
	return found, nil
}

// MetadataReport summarises a titles.metadata run.
type MetadataReport struct {
	DetailsRefreshed int `json:"details_refreshed"`
	DetailsMissing   int `json:"details_missing"`
	DetailsFailed    int `json:"details_failed"`
	WikidataChecked  int `json:"wikidata_checked"`
	WikidataFound    int `json:"wikidata_found"`
}

// Run refreshes stale TMDB details (charting titles every few days, the rest
// monthly) and looks up Wikidata IDs for titles not checked recently.
func (s *TitleMetadataService) Run(ctx context.Context, runLog func(level, msg string)) (*MetadataReport, error) {
	rep := &MetadataReport{}
	logf := func(level, format string, args ...any) {
		if runLog != nil {
			runLog(level, fmt.Sprintf(format, args...))
		}
	}

	if s.tmdb.Enabled() {
		refs, err := s.repo.RefsDueForDetails(ctx, metadataRecentWindow, metadataStaleWindow, metadataBatch)
		if err != nil {
			return rep, err
		}
		logf("info", "TMDB details: %d titles due", len(refs))
		for i, ref := range refs {
			if i > 0 {
				if err := sleepCtx(ctx, tmdbRequestGap); err != nil {
					return rep, err
				}
			}
			d, err := s.RefreshDetails(ctx, ref)
			switch {
			case err != nil:
				rep.DetailsFailed++
				s.log.Warn("tmdb details refresh failed", slog.String("ref", ref.String()), slog.Any("err", err))
				if rep.DetailsFailed <= 5 {
					logf("warn", "%s: %v", ref, err)
				}
			case d == nil:
				rep.DetailsMissing++
			default:
				rep.DetailsRefreshed++
			}
		}
		logf("info", "TMDB details: %d refreshed, %d not on TMDB, %d failed", rep.DetailsRefreshed, rep.DetailsMissing, rep.DetailsFailed)
	} else {
		logf("info", "TMDB details skipped: TMDB_API_KEY is not set")
	}

	refs, err := s.repo.RefsDueForWikidata(ctx, wikidataWindow, wikidataBatch)
	if err != nil {
		return rep, err
	}
	if len(refs) > 0 {
		found, err := s.LookupWikidata(ctx, refs)
		rep.WikidataChecked, rep.WikidataFound = len(refs), len(found)
		if err != nil {
			logf("error", "Wikidata lookup failed: %v", err)
			return rep, err
		}
	}
	logf("info", "Wikidata: %d titles checked, %d found", rep.WikidataChecked, rep.WikidataFound)
	if rep.DetailsFailed > 0 && rep.DetailsRefreshed == 0 {
		return rep, fmt.Errorf("every TMDB details request failed (%d)", rep.DetailsFailed)
	}
	return rep, nil
}

// FindNetflixOnWikidata looks Netflix Top 10 names up among Wikidata items
// with a Netflix ID (nil map when Wikidata is not configured).
func (s *TitleMetadataService) FindNetflixOnWikidata(ctx context.Context, names []string) (map[string][]client.WikidataNetflixHit, error) {
	if s == nil || s.wikidata == nil || len(names) == 0 {
		return nil, nil
	}
	return s.wikidata.FindNetflixTitles(ctx, names)
}

// SaveMatchedIDs stores IDs of a TMDB title found by matching (TMDB's,
// Wikidata's and hand-set ones win over them).
func (s *TitleMetadataService) SaveMatchedIDs(ctx context.Context, ref repository.TmdbRef, ids repository.TmdbIDs) error {
	if s == nil {
		return nil
	}
	return s.repo.SaveIDs(ctx, ref, ids, repository.IDSourceMatch)
}

// ResolvedTmdb is the TMDB title an IMDb or Wikidata ID resolved to.
type ResolvedTmdb struct {
	Ref   repository.TmdbRef
	Title string
	Year  int
	Via   string               // tmdb_find | wikidata
	Wiki  *client.WikidataItem // the Wikidata item, when resolved through it
}

// ResolveExternalID finds the TMDB title of an IMDb (tt…) or Wikidata (Q…)
// ID: TMDB's /find first, then (Wikidata IDs) the item's own TMDB IDs.
// kindHint (movie | tv) picks one when both a movie and a TV show match.
// Returns a ValidationError when the ID is malformed, matches nothing or is
// ambiguous.
func (s *TitleMetadataService) ResolveExternalID(ctx context.Context, id, kindHint string) (*ResolvedTmdb, error) {
	id = strings.TrimSpace(id)
	isQID := client.WikidataQIDRe.MatchString(strings.ToUpper(id))
	if isQID {
		id = strings.ToUpper(id)
	} else if !imdbIDRe.MatchString(id) {
		return nil, &ValidationError{Message: "expected an IMDb ID (tt1234567) or a Wikidata ID (Q141461750)"}
	}
	var found []client.TMDBFound
	via := "tmdb_find"
	if s.tmdb.Enabled() {
		var err error
		if found, err = s.tmdb.FindByExternalID(ctx, id); err != nil {
			s.log.Warn("tmdb find failed", slog.String("external_id", id), slog.Any("err", err))
		}
	}
	var wiki *client.WikidataItem
	if isQID && s.wikidata != nil {
		it, err := s.wikidata.LookupByQID(ctx, id)
		if err != nil {
			s.log.Warn("wikidata item lookup failed", slog.String("wikidata_id", id), slog.Any("err", err))
		}
		wiki = it
		if len(found) == 0 && it != nil {
			via = "wikidata"
			if it.TmdbMovieID != "" {
				found = append(found, client.TMDBFound{Kind: "movie", ID: it.TmdbMovieID})
			}
			if it.TmdbTVID != "" {
				found = append(found, client.TMDBFound{Kind: "tv", ID: it.TmdbTVID})
			}
		}
	}
	if kindHint != "" && len(found) > 1 {
		var keep []client.TMDBFound
		for _, f := range found {
			if f.Kind == kindHint {
				keep = append(keep, f)
			}
		}
		found = keep
	}
	switch len(found) {
	case 0:
		return nil, &ValidationError{Message: "no TMDB title has " + id}
	case 1:
		return &ResolvedTmdb{Ref: repository.TmdbRef{Kind: found[0].Kind, ID: found[0].ID}, Title: found[0].Title, Year: found[0].Year, Via: via, Wiki: wiki}, nil
	default:
		return nil, &ValidationError{Message: id + " matches several TMDB titles; pick the TMDB kind"}
	}
}

// FindNetflixIDsOnWikidata returns Wikidata's TMDB titles per Netflix ID.
func (s *TitleMetadataService) FindNetflixIDsOnWikidata(ctx context.Context, netflixIDs []string) (map[string][]client.WikidataNetflixHit, error) {
	if s == nil || s.wikidata == nil || len(netflixIDs) == 0 {
		return nil, nil
	}
	return s.wikidata.LookupByNetflixIDs(ctx, netflixIDs)
}

// RefsByNetflixID returns the TMDB titles already holding a Netflix ID.
func (s *TitleMetadataService) RefsByNetflixID(ctx context.Context, netflixID string) ([]repository.TmdbRef, error) {
	if s == nil {
		return nil, nil
	}
	return s.repo.RefsByNetflixID(ctx, netflixID)
}

// RefsDueForRT lists TMDB titles to look up on Rotten Tomatoes (only = one
// ref, or any due one).
func (s *TitleMetadataService) RefsDueForRT(ctx context.Context, only *repository.TmdbRef, limit int64) ([]repository.RTDue, error) {
	return s.repo.RefsDueForRT(ctx, only, limit)
}

// RecordRTAttempt records one Rotten Tomatoes lookup of a TMDB title (slug
// "" = not found; the next one waits retryAfter).
func (s *TitleMetadataService) RecordRTAttempt(ctx context.Context, ref repository.TmdbRef, slug, retryAfter string) error {
	return s.repo.RecordRTAttempt(ctx, ref, slug, retryAfter)
}
