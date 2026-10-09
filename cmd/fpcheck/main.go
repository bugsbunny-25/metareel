// Command fpcheck runs one FlixPatrol scrape + title mapping end to end
// without touching the database, printing debug logs as it goes. Use it to
// check that pages come through (directly or via FlareSolverr) and that
// titles map to the right TMDB IDs:
//
//	go run ./cmd/fpcheck -flaresolverr http://kundan-eq13.local:8191
//	go run ./cmd/fpcheck -flaresolverr http://kundan-eq13.local:8191 -slug bugonia
//
// Other settings (TMDB_API_KEY, FLARESOLVERR_*, SCRAPER_*) come from the
// environment / .env as for the server.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/bugsbunny-25/metareel/internal/client"
	"github.com/bugsbunny-25/metareel/internal/config"
	"github.com/bugsbunny-25/metareel/internal/scraper"
	"github.com/bugsbunny-25/metareel/internal/scraper/flixpatrol"
	"github.com/bugsbunny-25/metareel/internal/service"
)

func main() {
	var (
		flareURL = flag.String("flaresolverr", "", "FlareSolverr base URL (overrides FLARESOLVERR_URL)")
		direct   = flag.Bool("direct", false, "fetch FlixPatrol directly with Colly even if FLARESOLVERR_URL is set")
		provider = flag.String("provider", "netflix", "FlixPatrol provider slug")
		country  = flag.String("country", "united-states", "FlixPatrol country slug")
		titles   = flag.Int("titles", 3, "number of Top 10 movies + TV shows to map (0 = skip mapping)")
		slug     = flag.String("slug", "", "only fetch and print this FlixPatrol title page")
		delay    = flag.Duration("delay", 5*time.Second, "delay between FlixPatrol requests")
	)
	flag.Parse()

	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
	if err := run(log, *flareURL, *direct, flixpatrol.Provider(*provider), *country, *titles, *slug, *delay); err != nil {
		log.Error("fpcheck failed", slog.Any("err", err))
		os.Exit(1)
	}
}

func run(log *slog.Logger, flareURL string, direct bool, provider flixpatrol.Provider, countrySlug string, titles int, slug string, delay time.Duration) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if flareURL != "" {
		cfg.FlareSolverr.URL = flareURL
	}
	if direct {
		cfg.FlareSolverr.URL = ""
	}

	var fetcher flixpatrol.Fetcher = flixpatrol.LoggingFetcher{
		Next: flixpatrol.CollyFetcher{Base: scraper.NewCollector(cfg.Scraper)},
		Via:  "colly",
		Log:  log,
	}
	if fs := client.NewFlareSolverr(cfg.FlareSolverr); fs != nil {
		version, err := fs.Health(context.Background())
		if err != nil {
			return fmt.Errorf("flaresolverr at %s: %w", fs.Endpoint(), err)
		}
		log.Info("using flaresolverr", slog.String("endpoint", fs.Endpoint()), slog.String("version", version), slog.String("session", cfg.FlareSolverr.Session))
		fetcher = flixpatrol.LoggingFetcher{
			Next: flixpatrol.FlareSolverrFetcher{Client: fs, Log: log},
			Via:  "flaresolverr",
			Log:  log,
		}
	} else {
		log.Info("fetching flixpatrol directly")
	}

	ctx := context.Background()
	opts := service.FlixPatrolRunOptions{RequestDelay: delay, UserAgent: cfg.Scraper.UserAgent}

	if slug != "" {
		d, err := flixpatrol.ScrapeTitleDetails(ctx, fetcher, slug, opts.UserAgent, opts.RespectRobots)
		if err != nil {
			return err
		}
		fmt.Printf("%s: name=%q kind=%s year=%d country=%q\n", slug, d.Name, d.TitleKind, d.Year, d.Country)
		return nil
	}

	u := flixpatrol.Top10URL(provider, countrySlug, time.Now().UTC())
	top10, err := flixpatrol.ScrapeTop10(ctx, fetcher, u, opts.UserAgent, opts.RespectRobots)
	if err != nil {
		return err
	}
	fmt.Printf("\n%s\n", u)
	for _, list := range [][]flixpatrol.Entry{top10.Movies, top10.TVShows} {
		for _, e := range list {
			fmt.Printf("  %-8s #%-2d %-40s %s\n", e.TitleKind, e.Rank, e.Name, e.Slug)
		}
	}
	if titles <= 0 {
		return nil
	}

	countryCode, err := flixpatrol.GetCountryCode(countrySlug)
	if err != nil {
		return err
	}
	httpClient := client.New(cfg.HTTP)
	matcher := service.NewTitleMatcher(log, fetcher,
		client.NewTMDB(httpClient, cfg.TMDB.APIKey),
		client.NewJustWatchClient(httpClient.GetClient()),
		client.NewRottenTomatoes(cfg.HTTP.Timeout))

	fmt.Println("\nmapping:")
	for _, list := range [][]flixpatrol.Entry{top10.Movies, top10.TVShows} {
		for i, e := range list {
			if i >= titles {
				break
			}
			details, err := matcher.FetchDetails(ctx, e.Slug, service.FetchOptions{RequestDelay: opts.RequestDelay, UserAgent: opts.UserAgent, RespectRobots: opts.RespectRobots})
			if err != nil {
				fmt.Printf("  %-40s    (title page unavailable: %v)\n", e.Slug, err)
			}
			m, err := matcher.Match(ctx, service.MatchInput{
				Slug: e.Slug, ChartName: e.Name, ChartKind: e.TitleKind, Details: details,
				Provider: provider, Country: string(countryCode),
			})
			if err != nil {
				return err
			}
			if m.TmdbID == "" {
				fmt.Printf("  %-40s -> not mapped (kind %s)\n", e.Slug, m.Kind)
				continue
			}
			rtSlug, _ := matcher.FindRTSlug(ctx, service.RTQuery{Slug: e.Slug, Name: m.Title, Year: m.Year, Kind: m.Kind, TmdbID: m.TmdbID, From: m.Source})
			fmt.Printf("  %-40s -> %s %q (%d) tmdb=%s imdb=%s rt=%s via %s\n", e.Slug, m.Kind, m.Title, m.Year, m.TmdbID, m.ImdbID, rtSlug, m.Source)
		}
	}
	return nil
}
