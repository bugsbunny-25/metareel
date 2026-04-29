package scraper

import (
	"github.com/gocolly/colly/v2"

	"github.com/bugsbunny-25/metareel/internal/config"
)

// NewCollector returns a configured *colly.Collector. Callers should clone
// this base collector per-request (c.Clone()) so that per-run state (visited
// URLs, in-flight queues) does not leak across concurrent scrapes.
func NewCollector(cfg config.Scraper) *colly.Collector {
	c := colly.NewCollector(
		colly.UserAgent(cfg.UserAgent),
		colly.Async(true),
		colly.AllowURLRevisit(),
		colly.CheckHead(),
	)

	c.SetRequestTimeout(cfg.RequestTimeout)

	_ = c.Limit(&colly.LimitRule{
		DomainGlob:  "*",
		Parallelism: cfg.Parallelism,
	})

	return c
}
