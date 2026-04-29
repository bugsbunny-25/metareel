package router

import (
	"net/http"
	"os"
	"path/filepath"

	"github.com/labstack/echo/v5"
	emw "github.com/labstack/echo/v5/middleware"

	"github.com/bugsbunny-25/metareel/internal/config"
	"github.com/bugsbunny-25/metareel/internal/handler"
	"github.com/bugsbunny-25/metareel/internal/openapi"
)

// Register wires all routes and middleware onto the given Echo instance.
func Register(e *echo.Echo, h *handler.Handler, cfg *config.Config) {
	e.Use(emw.Recover())
	e.Use(emw.RequestID())
	e.Use(emw.RequestLogger())
	e.Use(emw.Gzip())
	e.Use(emw.CORSWithConfig(emw.CORSConfig{
		AllowOrigins: []string{"*"},
		AllowMethods: []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodOptions},
	}))
	e.Use(emw.BodyLimit(1 * 1024 * 1024))

	registerOpenAPI(e)

	e.GET("/healthz", h.Health)
	e.GET("/readyz", h.Health)

	api := e.Group("/api/v1")
	api.GET("/test", h.Test)
	api.GET("/titles", h.ListTitles)
	api.PATCH("/titles/:id", h.PatchTitle)
	api.GET("/top10/movies/:country", h.GetTop10MoviesAllProviders)
	api.GET("/top10/movies/:country/:provider", h.GetTop10MoviesByProvider)
	api.GET("/top10/tv-shows/:country", h.GetTop10TVShowsAllProviders)
	api.GET("/top10/tv-shows/:country/:provider", h.GetTop10TVShowsByProvider)

	admin := api.Group("/admin")
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

	if cfg.UI.ServeStatic {
		registerStatic(e, cfg.UI.StaticDir)
	}
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

// registerStatic serves the built SPA from `dir`.
// Real asset files (JS, CSS, etc.) are served directly; any other path falls
// back to index.html so client-side routing works.
func registerStatic(e *echo.Echo, dir string) {
	if _, err := os.Stat(dir); err != nil {
		return
	}

	indexPath := filepath.Join(dir, "index.html")

	e.GET("/*", func(c *echo.Context) error {
		filePath := filepath.Join(dir, filepath.Clean("/"+c.Param("*")))
		if info, err := os.Stat(filePath); err == nil && !info.IsDir() {
			return c.File(filePath)
		}
		return c.File(indexPath)
	})
}
