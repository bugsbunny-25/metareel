package config

import (
	"fmt"
	"time"

	"github.com/caarlos0/env/v11"
	"github.com/joho/godotenv"
)

// Config is the top-level application configuration populated from the
// environment (optionally seeded by a `.env` file in local development).
type Config struct {
	App          App
	Server       Server
	Log          Log
	Database     Database
	Redis        Redis
	Asynq        Asynq
	HTTP         HTTPClient
	Scraper      Scraper
	FlareSolverr FlareSolverr
	UI           UI
	TMDB         TMDB
	Ratings      Ratings
	MDBList      MDBList
}

type App struct {
	Env string `env:"APP_ENV" envDefault:"development"`
}

type Server struct {
	Host            string        `env:"SERVER_HOST" envDefault:"0.0.0.0"`
	Port            int           `env:"SERVER_PORT" envDefault:"8080"`
	ReadTimeout     time.Duration `env:"SERVER_READ_TIMEOUT" envDefault:"15s"`
	WriteTimeout    time.Duration `env:"SERVER_WRITE_TIMEOUT" envDefault:"15s"`
	ShutdownTimeout time.Duration `env:"SERVER_SHUTDOWN_TIMEOUT" envDefault:"10s"`
}

func (s Server) Address() string {
	return fmt.Sprintf("%s:%d", s.Host, s.Port)
}

type Log struct {
	Level    string `env:"LOG_LEVEL" envDefault:"info"`
	FilePath string `env:"LOG_FILE_PATH" envDefault:".data/metareel.log"`
}

type Database struct {
	URL           string `env:"DATABASE_URL" envDefault:"file:./data/metareel.db?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)"`
	MigrationsDir string `env:"DATABASE_MIGRATIONS_DIR" envDefault:"db/migrations"`
}

type Redis struct {
	Addr       string        `env:"REDIS_ADDR" envDefault:"localhost:6379"`
	Password   string        `env:"REDIS_PASSWORD" envDefault:""`
	DB         int           `env:"REDIS_DB" envDefault:"0"`
	DefaultTTL time.Duration `env:"REDIS_DEFAULT_TTL" envDefault:"5m"`
}

type Asynq struct {
	Queue       string `env:"ASYNQ_QUEUE" envDefault:"default"`
	Concurrency int    `env:"ASYNQ_CONCURRENCY" envDefault:"10"`
}

type HTTPClient struct {
	Timeout    time.Duration `env:"HTTP_CLIENT_TIMEOUT" envDefault:"20s"`
	RetryCount int           `env:"HTTP_CLIENT_RETRY_COUNT" envDefault:"2"`
	UserAgent  string        `env:"HTTP_CLIENT_USER_AGENT" envDefault:"metareel/0.1"`
}

type Scraper struct {
	UserAgent      string        `env:"SCRAPER_USER_AGENT" envDefault:"metareel-bot/0.1"`
	Parallelism    int           `env:"SCRAPER_PARALLELISM" envDefault:"4"`
	RequestTimeout time.Duration `env:"SCRAPER_REQUEST_TIMEOUT" envDefault:"20s"`
}

// FlareSolverr routes FlixPatrol page fetches through a FlareSolverr instance
// when URL is set (e.g. "http://flaresolverr:8191"); otherwise pages are
// fetched directly with Colly.
type FlareSolverr struct {
	URL        string        `env:"FLARESOLVERR_URL" envDefault:""`
	MaxTimeout time.Duration `env:"FLARESOLVERR_MAX_TIMEOUT" envDefault:"60s"`
	// Session, when set, reuses one FlareSolverr browser session (and its
	// Cloudflare clearance cookie) across requests instead of starting a new
	// browser per page.
	Session    string        `env:"FLARESOLVERR_SESSION" envDefault:"metareel"`
	SessionTTL time.Duration `env:"FLARESOLVERR_SESSION_TTL" envDefault:"30m"`
}

type UI struct {
	StaticDir   string `env:"UI_STATIC_DIR" envDefault:"web/dist"`
	ServeStatic bool   `env:"UI_SERVE_STATIC" envDefault:"true"`
}

type TMDB struct {
	APIKey string `env:"TMDB_API_KEY" envDefault:""`
}

// Ratings configures GET /api/v1/titles/tmdb/{kind}/{id}/ratings.
type Ratings struct {
	// TTL is how long stored ratings are served before the next request
	// refreshes them.
	TTL time.Duration `env:"RATINGS_TTL" envDefault:"12h"`
	// PreferredProvider ("justwatch" or "mdblist") wins when both have an
	// IMDb / RT value; RT's own scores always win for RT. Falls back to
	// justwatch when MDBLIST_API_KEY is not set.
	PreferredProvider string `env:"RATINGS_PREFERRED_PROVIDER" envDefault:"justwatch"`
}

type MDBList struct {
	// APIKey enables MDBList ratings (https://mdblist.com/preferences/).
	APIKey string `env:"MDBLIST_API_KEY" envDefault:""`
}

// Load reads configuration from environment variables. If a `.env` file is
// present in the working directory it is loaded first (non-fatal if missing).
func Load() (*Config, error) {
	_ = godotenv.Load()

	cfg := &Config{}
	if err := env.Parse(cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	return cfg, nil
}
