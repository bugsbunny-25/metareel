package service

import (
	"context"
	"fmt"
	"log/slog"
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
