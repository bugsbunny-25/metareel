-- +goose Up
-- Support chart reads: latest / previous date per chart, and per-title
-- history within a chart (previous_rank, days_in_top10, ranking history).
CREATE INDEX idx_rankings_chart_date ON rankings(country, streaming_provider, category, ranked_on);
CREATE INDEX idx_rankings_title_chart_date ON rankings(title_id, country, streaming_provider, category, ranked_on);

-- +goose Down
DROP INDEX IF EXISTS idx_rankings_title_chart_date;
DROP INDEX IF EXISTS idx_rankings_chart_date;
