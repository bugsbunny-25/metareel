package service

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/gocolly/colly/v2"

	"github.com/bugsbunny-25/metareel/internal/client"
	"github.com/bugsbunny-25/metareel/internal/repository"
	"github.com/bugsbunny-25/metareel/internal/scraper/flixpatrol"
)

type FlixPatrolJob struct {
	log       *slog.Logger
	colly     *colly.Collector
	repo      *repository.FlixPatrolRepository
	tmdb      *client.TMDBClient
	justWatch *client.JustWatchClient
	wikidata  *client.WikidataClient
}

func NewFlixPatrolJob(log *slog.Logger, collyCollector *colly.Collector, repo *repository.FlixPatrolRepository, tmdbClient *client.TMDBClient, justWatchClient *client.JustWatchClient, wikidataClient *client.WikidataClient) *FlixPatrolJob {
	return &FlixPatrolJob{
		log:       log,
		colly:     collyCollector,
		repo:      repo,
		tmdb:      tmdbClient,
		justWatch: justWatchClient,
		wikidata:  wikidataClient,
	}
}

type FlixPatrolScrapeTarget struct {
	CountrySlug string
	Provider    flixpatrol.Provider
}

type FlixPatrolRunOptions struct {
	RequestDelay  time.Duration
	UserAgent     string
	RespectRobots bool
}

func (j *FlixPatrolJob) RunTargets(ctx context.Context, targets []FlixPatrolScrapeTarget, date time.Time, opts FlixPatrolRunOptions) error {
	for i, target := range targets {
		if i > 0 && opts.RequestDelay > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(opts.RequestDelay):
			}
		}

		u := flixpatrol.Top10URL(target.Provider, target.CountrySlug, date)
		top10, err := flixpatrol.ScrapeTop10(ctx, j.colly, u, opts.UserAgent, opts.RespectRobots)
		if err != nil {
			return fmt.Errorf("scrape %s: %w", u, err)
		}

		if err := j.persistTop10(ctx, top10); err != nil {
			return err
		}
		j.log.Info("flixpatrol scraped", slog.String("provider", string(target.Provider)), slog.String("country", target.CountrySlug), slog.String("date", top10.Date.Format("2006-01-02")))
	}
	return nil
}

func (j *FlixPatrolJob) persistTop10(ctx context.Context, top10 *flixpatrol.Top10) error {
	countryCode, err := flixpatrol.GetCountryCode(top10.CountrySlug)
	if err != nil {
		return err
	}

	if err := j.persistEntries(ctx, top10.Provider, top10.Date, string(countryCode), "movies", top10.Movies); err != nil {
		return err
	}
	if err := j.persistEntries(ctx, top10.Provider, top10.Date, string(countryCode), "tv_shows", top10.TVShows); err != nil {
		return err
	}
	return nil
}

func (j *FlixPatrolJob) persistEntries(ctx context.Context, provider flixpatrol.Provider, rankedOn time.Time, country string, category string, entries []flixpatrol.Entry) error {
	for _, e := range entries {
		// check if the title already exists
		existing, err := j.repo.GetTitleBySlug(ctx, e.Slug)
		if err != nil {
			return fmt.Errorf("get title %s: %w", e.Slug, err)
		}
		// if the title already exists and has both TMDB ID and IMDb ID and Rotten Tomatoes URL, upsert the ranking
		if existing != nil && existing.TmdbID.Valid && existing.TmdbID.String != "" && existing.ImdbID.Valid && existing.ImdbID.String != "" && existing.RtUrl.Valid && existing.RtUrl.String != "" {
			_, err = j.repo.UpsertRanking(ctx, repository.UpsertRankingInput{
				TitleID:           existing.ID,
				RankedOn:          rankedOn,
				Country:           country,
				StreamingProvider: string(provider),
				Category:          category,
				Rank:              int64(e.Rank),
			})
			if err != nil {
				return fmt.Errorf("upsert ranking %s rank %d: %w", e.Slug, e.Rank, err)
			}
			continue
		}

		var objectType client.ObjectType
		var slugType string
		if e.TitleKind == flixpatrol.TitleKindMovie {
			objectType = client.ObjectTypeMovie
			slugType = "movie"
		} else {
			objectType = client.ObjectTypeTVShow
			slugType = "tv-show"
		}

		// get the existing title IDs from the database
		var (
			tmdbID string
			imdbID string
			rtURL  string
		)
		if existing != nil {
			if existing.TmdbID.Valid && existing.TmdbID.String != "" {
				tmdbID = existing.TmdbID.String
			}
			if existing.ImdbID.Valid && existing.ImdbID.String != "" {
				imdbID = existing.ImdbID.String
			}
			if existing.RtUrl.Valid && existing.RtUrl.String != "" {
				rtURL = existing.RtUrl.String
			}
		}

		if tmdbID == "" {
			// search for tmdbID using justWatch
			if j.justWatch != nil {
				// try to get the tmdbID using the path lookup same as the flixpatrol slug
				fullPath := fmt.Sprintf("/%s/%s/%s", country, slugType, e.Slug)
				justWatchID, err := j.justWatch.GetTitlesByPath(ctx, fullPath, country, "en")
				if err == nil {
					if strings.EqualFold(justWatchID.UrlV2.Node.MovieOrShowFragment.Content.Title, e.Name) {
						if justWatchID.UrlV2.Node.MovieOrShowFragment.Content.ExternalIds.TmdbId != nil {
							tmdbID = *justWatchID.UrlV2.Node.MovieOrShowFragment.Content.ExternalIds.TmdbId
						}
						if justWatchID.UrlV2.Node.MovieOrShowFragment.Content.ExternalIds.ImdbId != nil {
							imdbID = *justWatchID.UrlV2.Node.MovieOrShowFragment.Content.ExternalIds.ImdbId
						}
					}
				} else {
					j.log.Debug("justwatch mapping failed by path lookup", slog.String("name", e.Name), slog.Any("err", err))
					// try to search the title using the top search popular
					pack := GetPackageFromFlixPatrolProvider(provider)
					justWatchSearch, err := j.justWatch.GetTitlesByTopSearchPopular(ctx, e.Slug, country, "en", objectType, pack)
					if err == nil {
						if justWatchSearch.PoupularTitles.TotalCount > 0 && strings.EqualFold(justWatchSearch.PoupularTitles.Edges[0].Node.Content.Title, e.Name) {
							if justWatchSearch.PoupularTitles.Edges[0].Node.Content.ExternalIds.TmdbId != nil {
								tmdbID = *justWatchSearch.PoupularTitles.Edges[0].Node.Content.ExternalIds.TmdbId
							}
							if justWatchSearch.PoupularTitles.Edges[0].Node.Content.ExternalIds.ImdbId != nil {
								imdbID = *justWatchSearch.PoupularTitles.Edges[0].Node.Content.ExternalIds.ImdbId
							}
						}
					} else {
						j.log.Debug("justwatch mapping failed by top search popular", slog.String("name", e.Name), slog.Any("err", err))
					}
				}
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(1 * time.Second):
				}
			}

			if j.tmdb != nil && j.tmdb.Enabled() {
				// search for tmdbID using tmdb API
				if tmdbID == "" {
					var id int
					var title string
					var err error
					switch e.TitleKind {
					case flixpatrol.TitleKindMovie:
						id, title, err = j.tmdb.SearchMovie(ctx, e.Name)
						if err == nil && strings.EqualFold(title, e.Name) {
							tmdbID = fmt.Sprintf("%d", id)
						}
					case flixpatrol.TitleKindTVShow:
						id, title, err = j.tmdb.SearchTV(ctx, e.Name)
						if err == nil && strings.EqualFold(title, e.Name) {
							tmdbID = fmt.Sprintf("%d", id)
						}
					default:
						err = fmt.Errorf("invalid title kind: %s", e.TitleKind)
					}
					if err != nil {
						j.log.Debug("tmdb mapping failed by search", slog.String("name", e.Name), slog.Any("err", err))
					}
				}
			}
		}

		// if tmdbID is found, get the external IDs
		if tmdbID != "" && j.tmdb != nil && j.tmdb.Enabled() {
			tmdbIDInt, err := strconv.Atoi(tmdbID)
			var wikidataID string
			var tempImdbID string
			if err != nil {
				j.log.Debug("tmdb mapping failed by conversion", slog.String("name", e.Name), slog.Any("err", err))
			} else {
				switch e.TitleKind {
				case flixpatrol.TitleKindMovie:
					tempImdbID, wikidataID, _ = j.tmdb.MovieExternalIDs(ctx, tmdbIDInt)
				case flixpatrol.TitleKindTVShow:
					tempImdbID, wikidataID, _ = j.tmdb.TVExternalIDs(ctx, tmdbIDInt)
				default:
					err = fmt.Errorf("invalid title kind: %s", e.TitleKind)
				}
			}
			if tempImdbID != "" {
				imdbID = tempImdbID
			}

			// if wikidataID is found, use it to get rtURL
			if wikidataID != "" && j.wikidata != nil && j.wikidata.Enabled() {
				rtURL, err = j.wikidata.GetRottenTomatoesID(ctx, wikidataID, client.WikidataTitleKind(e.TitleKind))
				if err != nil {
					j.log.Debug("wikidata mapping failed by get rotten tomatoes id", slog.String("name", e.Name), slog.Any("err", err))
				}
			}
		}

		kind := "movie"
		if e.TitleKind == flixpatrol.TitleKindTVShow {
			kind = "tv_show"
		}

		title, err := j.repo.UpsertTitle(ctx, repository.UpsertTitleInput{
			Slug:   e.Slug,
			Name:   e.Name,
			Kind:   kind,
			TmdbID: tmdbID,
			ImdbID: imdbID,
			RtURL:  rtURL,
		})
		if err != nil {
			return fmt.Errorf("upsert title %s: %w", e.Slug, err)
		}

		_, err = j.repo.UpsertRanking(ctx, repository.UpsertRankingInput{
			TitleID:           title.ID,
			RankedOn:          rankedOn,
			Country:           country,
			StreamingProvider: string(provider),
			Category:          category,
			Rank:              int64(e.Rank),
		})
		if err != nil {
			return fmt.Errorf("upsert ranking %s rank %d: %w", e.Slug, e.Rank, err)
		}
	}
	return nil
}

func GetPackageFromFlixPatrolProvider(provider flixpatrol.Provider) client.Package {
	switch provider {
	case flixpatrol.ProviderAmazonPrime:
		return client.PackageAmazonPrime
	case flixpatrol.ProviderAppleTV:
		return client.PackageAppleTVPlus
	case flixpatrol.ProviderDisneyPlus:
		return client.PackageDisneyPlus
	case flixpatrol.ProviderHBOMax:
		return client.PackageHBOMax
	case flixpatrol.ProviderNetflix:
		return client.PackageNetflix
	case flixpatrol.ProviderParamountPlus:
		return client.PackageParamountPlus
	case flixpatrol.ProviderPeacock:
		return client.PackagePeacock
	default:
		return client.PackageNetflix
	}
}
