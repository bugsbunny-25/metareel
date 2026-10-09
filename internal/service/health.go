package service

import (
	"context"
	"database/sql"
	"time"

	"github.com/bugsbunny-25/metareel/internal/scraper/flixpatrol"
)

// HealthService answers /readyz: the database and Redis are required;
// FlixPatrol's fetch backend (FlareSolverr) is reported but not required,
// since the API keeps serving stored data without it.
type HealthService struct {
	db      *sql.DB
	redis   func() error // e.g. asynq.Client.Ping
	fetcher flixpatrol.Fetcher
}

func NewHealthService(db *sql.DB, redisPing func() error, fetcher flixpatrol.Fetcher) *HealthService {
	return &HealthService{db: db, redis: redisPing, fetcher: fetcher}
}

// HealthCheck is one dependency's state.
type HealthCheck struct {
	Status   string `json:"status"` // ok | error
	Error    string `json:"error,omitempty"`
	Required bool   `json:"required"`
}

// ReadinessReport is the /readyz body.
type ReadinessReport struct {
	Status string                 `json:"status"` // ok | degraded | error
	Checks map[string]HealthCheck `json:"checks"`
}

// Check probes each dependency (2s each) and reports whether every required
// one is up.
func (h *HealthService) Check(ctx context.Context) (*ReadinessReport, bool) {
	rep := &ReadinessReport{Status: "ok", Checks: map[string]HealthCheck{}}
	probe := func(name string, required bool, fn func(context.Context) error) {
		c, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		check := HealthCheck{Status: "ok", Required: required}
		if err := fn(c); err != nil {
			// Generic text: dependency errors can carry hostnames and URLs.
			check.Status, check.Error = "error", "unreachable"
			if required {
				rep.Status = "error"
			} else if rep.Status == "ok" {
				rep.Status = "degraded"
			}
		}
		rep.Checks[name] = check
	}
	probe("database", true, func(c context.Context) error { return h.db.PingContext(c) })
	if h.redis != nil {
		probe("redis", true, func(context.Context) error { return h.redis() })
	}
	if h.fetcher != nil {
		if _, ok := h.fetcher.(flixpatrol.HealthChecker); ok {
			probe("flixpatrol_fetcher", false, func(c context.Context) error { return flixpatrol.CheckHealth(c, h.fetcher) })
		}
	}
	return rep, rep.Status != "error"
}
