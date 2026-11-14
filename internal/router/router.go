package router

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/labstack/echo/v5"
	emw "github.com/labstack/echo/v5/middleware"

	"github.com/bugsbunny-25/metareel/internal/config"
	"github.com/bugsbunny-25/metareel/internal/handler"
	"github.com/bugsbunny-25/metareel/internal/openapi"
)

func isAdminPath(p string) bool {
	return p == "/api/v1/admin" || strings.HasPrefix(p, "/api/v1/admin/")
}

// Register wires all routes and middleware onto the given Echo instance.
func Register(e *echo.Echo, h *handler.Handler, cfg *config.Config) {
	e.Use(emw.Recover())
	e.Use(emw.RequestID())
	e.Use(emw.RequestLogger())
	e.Use(emw.Gzip())
	// CORS for the public API only: the admin UI is same-origin, and other
	// websites a LAN user opens must not be able to drive /api/v1/admin.
	e.Use(emw.CORSWithConfig(emw.CORSConfig{
		Skipper:      func(c *echo.Context) bool { return isAdminPath(c.Request().URL.Path) },
		AllowOrigins: []string{"*"},
		AllowMethods: []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodOptions},
	}))
	e.Use(emw.BodyLimit(1 * 1024 * 1024))

	registerOpenAPI(e)

	e.GET("/healthz", h.Live)
	e.GET("/readyz", h.Ready)

	// Sign-in for users (the alternative to API keys). Browser writes must
	// come from the same origin, so other sites can't sign a visitor in or
	// out.
	auth := e.Group("/api/v1/auth", handler.SameOriginWrites)
	auth.POST("/login", h.Login)
	auth.POST("/logout", h.Logout)
	auth.GET("/me", h.Me, h.Authenticate)
	auth.GET("/session", h.AuthSession)

	// Public API: needs an API key or a user's session (unless keys are
	// made optional in the admin settings) and is rate limited per caller.
	public := e.Group("/api/v1", h.Authenticate, handler.ETag)
	registerReadRoutes(public, h)

	// Tracking API: one user's watchlist, statuses, watch log and tags.
	// Deliberately NOT in registerReadRoutes, which wraps the shared data
	// routes in ETag: these responses are per-user and must never be cached
	// or revalidated, so NoStore replaces it. SameOriginWrites is needed
	// because the session cookie authenticates browser writes here, and
	// RequireUser refuses API keys, which identify an integration, not a
	// person.
	me := e.Group("/api/v1/me", handler.SameOriginWrites, h.Authenticate, h.RequireUser, handler.NoStore)
	me.GET("/account", h.GetAccount)
	me.POST("/password", h.ChangePassword)
	me.DELETE("/sessions", h.RevokeMyOtherSessions)
	me.DELETE("/sessions/:id", h.RevokeMySession)
	me.GET("/preferences", h.GetPreferences)
	me.PUT("/preferences", h.SetPreferences)
	me.GET("/titles", h.ListTrackedTitles)
	me.GET("/titles/search", h.SearchTrackedTitles)
	me.GET("/titles/:kind/:id", h.GetTrackedTitle)
	me.PUT("/titles/:kind/:id", h.SetTrackedTitle)
	me.DELETE("/titles/:kind/:id", h.DeleteTrackedTitle)
	me.GET("/watchlist", h.GetWatchlist)
	me.PUT("/watchlist/:kind/:id", h.AddToWatchlist)
	me.DELETE("/watchlist/:kind/:id", h.RemoveFromWatchlist)
	me.GET("/watches", h.ListWatches)
	me.POST("/watches", h.LogWatch)
	me.GET("/watches/:id", h.GetWatch)
	me.PUT("/watches/:id", h.UpdateWatch)
	me.DELETE("/watches/:id", h.DeleteWatch)
	me.GET("/tags", h.ListWatchTags)
	me.DELETE("/tags/:id", h.DeleteWatchTag)
	me.GET("/formats", h.ListTrackingFormats)
	me.GET("/venues", h.ListVenues)
	me.POST("/venues", h.CreateVenue)
	me.GET("/venues/:id", h.GetVenue)
	me.PUT("/venues/:id", h.UpdateVenue)
	me.DELETE("/venues/:id", h.DeleteVenue)
	me.POST("/venues/:id/aliases", h.AddVenueAlias)
	me.DELETE("/venues/:id/aliases/:alias_id", h.DeleteVenueAlias)
	me.POST("/venues/:id/screens", h.CreateScreen)
	me.PUT("/venues/:id/screens/:screen_id", h.UpdateScreen)
	me.DELETE("/venues/:id/screens/:screen_id", h.DeleteScreen)
	me.GET("/shows", h.ShowQueue)
	me.GET("/upcoming", h.Upcoming)
	me.GET("/shows/:id", h.GetShow)
	me.POST("/shows/:id/watched", h.MarkEpisodes)
	me.DELETE("/shows/:id/watched", h.UnmarkEpisodes)
	me.GET("/up-next", h.UpNext)
	me.GET("/stats/summary", h.GetStatsSummary)
	me.GET("/stats/calendar", h.GetStatsCalendar)
	me.GET("/stats/year", h.GetStatsYear)
	me.GET("/stats/ratings", h.GetStatsRatings)
	me.GET("/stats/spend", h.GetStatsSpend)
	me.GET("/stats/people", h.GetStatsPeople)
	me.GET("/watchlist/insights", h.GetWatchlistInsights)

	// Admin API and UI. The UI reads the same data through these admin
	// copies of the public routes, so it needs no API key. With
	// AUTH_REQUIRE_ADMIN_LOGIN only signed-in admins may use it; without, it
	// is open: keep it off the public internet (e.g. expose only /api/v1
	// except /api/v1/admin through your reverse proxy).
	admin := e.Group("/api/v1/admin", handler.SameOriginWrites, h.AdminAuth, handler.ETag)
	registerReadRoutes(admin, h)
	admin.GET("/stats", h.GetAdminStats)
	admin.PATCH("/titles/:id", h.PatchTitle)
	admin.POST("/titles/:id/rematch", h.RematchTitle)
	admin.GET("/task-types", h.GetTaskTypes)
	admin.POST("/jobs/:type/run", h.RunJob)
	admin.POST("/backfill", h.Backfill)
	admin.GET("/data-quality", h.GetDataQuality)
	admin.GET("/titles/:id/candidates", h.GetTitleCandidates)
	admin.GET("/netflix/titles", h.ListNetflixTitles)
	admin.GET("/netflix/titles/:id", h.GetNetflixTitle)
	admin.PATCH("/netflix/titles/:id", h.PatchNetflixTitle)
	admin.POST("/netflix/titles/:id/rematch", h.RematchNetflixTitle)
	admin.GET("/netflix/titles/:id/candidates", h.GetNetflixTitleCandidates)
	admin.PATCH("/netflix/seasons/:id", h.PatchNetflixSeason)
	admin.GET("/mappings", h.ListMappings)
	admin.POST("/mappings/sync", h.SyncMappings)
	admin.GET("/mappings/review", h.ReviewMappings)
	admin.GET("/mappings/review/:subject/:id/suggestions", h.GetReviewSuggestions)
	admin.GET("/mappings/:kind/:id", h.GetMapping)
	admin.POST("/mappings/:kind/:id/remap", h.RemapMapping)
	admin.GET("/mapping-events", h.ListMappingEvents)
	admin.GET("/mapping-refs", h.ListMappingRefs)
	admin.GET("/task-schedules", h.GetTaskSchedules)
	admin.PATCH("/task-schedules/:id", h.PatchTaskSchedule)
	admin.GET("/task-schedules/:id", h.GetTaskScheduleByID)
	admin.GET("/task-schedules/:id/runs", h.GetTaskRunsByScheduleID)
	admin.POST("/task-schedules/:id/run-now", h.RunTaskScheduleNow)
	admin.GET("/task-runs", h.GetTaskRuns)
	admin.GET("/task-runs/:run_id", h.GetTaskRunByID)
	admin.GET("/task-runs/:run_id/logs", h.GetTaskRunLogsByRunID)
	admin.POST("/task-schedules", h.CreateTaskSchedule)
	admin.POST("/task-schedules/:id/flixpatrol-targets", h.AddFlixPatrolTargets)
	admin.GET("/api-keys", h.ListAPIKeys)
	admin.POST("/api-keys", h.CreateAPIKey)
	admin.PATCH("/api-keys/:id", h.RenameAPIKey)
	admin.POST("/api-keys/:id/rotate", h.RotateAPIKey)
	admin.DELETE("/api-keys/:id", h.DeleteAPIKey)
	admin.GET("/webhooks", h.ListWebhooks)
	admin.POST("/webhooks", h.CreateWebhook)
	admin.PATCH("/webhooks/:id", h.UpdateWebhook)
	admin.DELETE("/webhooks/:id", h.DeleteWebhook)
	admin.POST("/webhooks/:id/rotate-secret", h.RotateWebhookSecret)
	admin.POST("/webhooks/:id/test", h.TestWebhook)
	admin.GET("/webhooks/:id/deliveries", h.ListWebhookDeliveries)
	admin.GET("/webhook-deliveries/:id", h.GetWebhookDelivery)
	admin.POST("/webhook-deliveries/:id/retry", h.RetryWebhookDelivery)
	admin.GET("/users", h.ListUsers)
	admin.POST("/users", h.CreateUser)
	admin.PATCH("/users/:username", h.PatchUser)
	admin.DELETE("/users/:username", h.DeleteUser)
	admin.POST("/users/:username/logout", h.LogoutUser)
	admin.GET("/sessions", h.ListSessions)
	admin.DELETE("/sessions/:id", h.RevokeSession)
	admin.GET("/watch-formats", h.ListWatchFormatsAdmin)
	admin.POST("/watch-formats", h.CreateWatchFormat)
	admin.PATCH("/watch-formats/:id", h.UpdateWatchFormat)
	admin.PUT("/watch-formats/:id/logo", h.PutWatchFormatLogo)
	// Cinemas, for the console's Cinemas page. The same handlers as the /me
	// routes users add cinemas through while logging; venues are shared data.
	admin.GET("/venues", h.ListVenues)
	admin.POST("/venues", h.CreateVenue)
	admin.GET("/venues/:id", h.GetVenue)
	admin.PUT("/venues/:id", h.UpdateVenue)
	admin.DELETE("/venues/:id", h.DeleteVenue)
	admin.POST("/venues/:id/aliases", h.AddVenueAlias)
	admin.DELETE("/venues/:id/aliases/:alias_id", h.DeleteVenueAlias)
	admin.POST("/venues/:id/screens", h.CreateScreen)
	admin.PUT("/venues/:id/screens/:screen_id", h.UpdateScreen)
	admin.DELETE("/venues/:id/screens/:screen_id", h.DeleteScreen)
	admin.DELETE("/watch-formats/:id/logo", h.DeleteWatchFormatLogo)
	admin.GET("/settings", h.GetSettings)
	admin.PATCH("/settings", h.PatchSettings)

	if cfg.UI.ServeStatic {
		registerStatic(e, cfg.UI.StaticDir, cfg.UI.AppStaticDir)
	}
}

// registerReadRoutes adds the data routes shared by the public API and the
// admin API.
func registerReadRoutes(g *echo.Group, h *handler.Handler) {
	g.GET("/test", h.Test)
	g.GET("/titles", h.ListTitles)
	g.GET("/titles/:id", h.GetTitle)
	g.GET("/titles/:id/rankings", h.GetTitleRankingsByID)
	g.GET("/titles/lookup", h.LookupTitle)
	g.GET("/search", h.SearchTitles)
	g.GET("/titles/tmdb/rankings", h.GetTitleRankingsBatch)
	g.GET("/titles/tmdb/ratings", h.GetTitleRatingsBatch)
	g.GET("/titles/tmdb/:kind/:id", h.GetTitleOverview)
	g.GET("/titles/tmdb/:kind/:id/rankings", h.GetTitleRankings)
	g.GET("/titles/tmdb/:kind/:id/ratings", h.GetTitleRatings)
	g.GET("/titles/tmdb/:kind/:id/stats", h.GetTitleStats)
	g.GET("/titles/tmdb/:kind/:id/availability", h.GetTitleAvailability)
	g.GET("/titles/tmdb/:kind/:id/availability/history", h.GetTitleAvailabilityHistory)
	g.GET("/titles/tmdb/:kind/:id/history", h.GetTitleHistory)
	g.GET("/titles/tmdb/:kind/:id/timeline", h.GetTitleTimeline)
	g.GET("/titles/tmdb/:kind/:id/releases", h.GetTitleReleases)
	g.GET("/titles/tmdb/:kind/:id/credits", h.GetTitleCredits)
	g.GET("/people/:id", h.GetPerson)
	g.GET("/availability/changes", h.GetAvailabilityChanges)
	g.GET("/calendar/:country", h.GetCalendar)
	g.GET("/titles/tmdb/:kind/:id/netflix", h.GetTitleNetflix)
	g.GET("/titles/new/:country/:service", h.GetNewTitles)
	g.GET("/titles/upcoming/:country/:service", h.GetUpcomingTitles)
	g.GET("/services/:country", h.GetServices)
	g.GET("/top10/movies/:country", h.GetTop10MoviesAllProviders)
	g.GET("/top10/movies/:country/:provider", h.GetTop10MoviesByProvider)
	g.GET("/top10/tv-shows/:country", h.GetTop10TVShowsAllProviders)
	g.GET("/top10/tv-shows/:country/:provider", h.GetTop10TVShowsByProvider)
	g.GET("/top10/:country", h.GetCountryTop10)
	g.GET("/top10/global/:provider", h.GetGlobalTop10)
	g.GET("/top10/movies/:country/:provider/history", h.GetChartHistory("movies"))
	g.GET("/top10/tv-shows/:country/:provider/history", h.GetChartHistory("tv_shows"))

	g.GET("/charts", h.GetCharts)
	g.GET("/charts/:country/:provider/dates", h.GetChartDates)
	g.GET("/changes", h.GetChanges)
	g.GET("/movers", h.GetMovers)
	g.GET("/breakouts", h.GetBreakouts)
	g.GET("/discover/:country/hidden-gems", h.GetHiddenGems)
	g.GET("/discover/:country/elsewhere", h.GetElsewhere)
	g.GET("/discover/:country/rotation", h.GetRotation)
	g.GET("/leaderboards", h.GetLeaderboard)
	g.GET("/export/rankings", h.ExportRankings)
	g.GET("/countries", h.GetCountries)
	g.GET("/providers", h.GetProviders)

	// Premium-format logos: images, served under both mounts so the user app
	// and the admin console each load them with their own credentials.
	g.GET("/formats/:slug/logo", h.GetFormatLogo)
	g.GET("/netflix/top10", h.GetNetflixTop10)
	g.GET("/netflix/top10/:country", h.GetNetflixTop10)
	g.GET("/netflix/most-popular", h.GetNetflixMostPopular)

	g.GET("/analytics/decay", h.GetDecay())
	g.GET("/analytics/country-similarity", h.GetCountrySimilarity())
	g.GET("/analytics/release-lag", h.GetReleaseLag())
	g.GET("/analytics/ratings-vs-popularity", h.GetRatingsVsPopularity())
	g.GET("/analytics/genres", h.GetGenres())
	g.GET("/analytics/origins", h.GetOrigins())
	g.GET("/analytics/release-patterns", h.GetReleasePatterns())
	g.GET("/analytics/spread-lag", h.GetSpreadLag())
	g.GET("/analytics/release-windows", h.GetReleaseWindows)
	g.GET("/analytics/people", h.GetPeopleAnalytics())
	g.GET("/analytics/netflix-calibration", h.GetNetflixCalibration())
}

func registerOpenAPI(e *echo.Echo) {
	e.GET("/openapi/public.yaml", func(c *echo.Context) error {
		b, err := openapi.SpecPublicYAML()
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]any{"error": "failed to load openapi spec"})
		}
		openapi.WriteYAML(c.Response(), b)
		return nil
	})
	e.GET("/openapi/admin.yaml", func(c *echo.Context) error {
		b, err := openapi.SpecAdminYAML()
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]any{"error": "failed to load openapi spec"})
		}
		openapi.WriteYAML(c.Response(), b)
		return nil
	})

	e.GET("/docs", func(c *echo.Context) error {
		raw, err := openapi.SwaggerUIHTMLPublic()
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]any{"error": "failed to load swagger ui"})
		}
		html, err := openapi.InjectSpecURL(raw, "/openapi/public.yaml")
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]any{"error": "failed to render swagger ui"})
		}
		openapi.WriteHTML(c.Response(), html)
		return nil
	})
	e.GET("/docs/admin", func(c *echo.Context) error {
		raw, err := openapi.SwaggerUIHTMLAdmin()
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]any{"error": "failed to load swagger ui"})
		}
		html, err := openapi.InjectSpecURL(raw, "/openapi/admin.yaml")
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]any{"error": "failed to render swagger ui"})
		}
		openapi.WriteHTML(c.Response(), html)
		return nil
	})
}

// registerStatic serves the two single-page apps.
//
// The admin console (adminDir, built with base /admin/) lives under /admin:
// its assets are /admin/assets/..., and any other /admin path loads its
// index.html (it uses a hash router). The user app (appDir) owns the rest of
// the site: real files are served as they are and any other path falls back
// to its index.html. Without a user app build, / redirects to the console so
// a deployment that only built the admin UI still opens somewhere useful.
//
// Paths stay relative to the working directory: echo binds its filesystem
// root at echo.New(), so c.File never resolves an absolute path.
func registerStatic(e *echo.Echo, adminDir, appDir string) {
	adminOK := isDir(adminDir)
	if adminOK {
		e.GET("/admin", spa(adminDir))
		e.GET("/admin/*", spa(adminDir))
	}
	switch {
	case isDir(appDir):
		e.GET("/*", spa(appDir))
	case adminOK:
		e.GET("/", func(c *echo.Context) error { return c.Redirect(http.StatusFound, "/admin/") })
	}
}

// spa serves a file from dir when the wildcard names one, else dir/index.html.
func spa(dir string) echo.HandlerFunc {
	indexPath := filepath.Join(dir, "index.html")
	return func(c *echo.Context) error {
		filePath := filepath.Join(dir, filepath.Clean("/"+c.Param("*")))
		if info, err := os.Stat(filePath); err == nil && !info.IsDir() {
			return c.File(filePath)
		}
		return c.File(indexPath)
	}
}

func isDir(dir string) bool {
	info, err := os.Stat(dir)
	return err == nil && info.IsDir()
}
