package service

import (
	"bufio"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/bugsbunny-25/metareel/internal/client"
	"github.com/bugsbunny-25/metareel/internal/repository"
)

// tmdbExportURL is TMDB's daily ID export of one kind (movie | tv_series) for
// a date (MM_DD_YYYY): gzipped JSON lines {"id":…,"popularity":…}, published
// around 08:00 UTC and kept for three months. No API key needed.
// https://developer.themoviedb.org/docs/daily-id-exports
const tmdbExportURL = "https://files.tmdb.org/p/exports/%s_ids_%s.json.gz"

const popularityBatch = 2000

// TMDBPopularityImporter records the daily TMDB popularity of the titles
// metareel knows from TMDB's ID exports (the tmdb.popularity.import job).
type TMDBPopularityImporter struct {
	log     *slog.Logger
	dl      *client.Downloader
	imports *repository.ImportsRepository
	history *repository.HistoryRepository
	now     func() time.Time
	url     string // format with (export kind, MM_DD_YYYY); tests override
}

func NewTMDBPopularityImporter(log *slog.Logger, dl *client.Downloader, imports *repository.ImportsRepository, history *repository.HistoryRepository) *TMDBPopularityImporter {
	return &TMDBPopularityImporter{log: log, dl: dl, imports: imports, history: history, now: time.Now, url: tmdbExportURL}
}

// Run imports today's exports (yesterday's while today's are not published),
// each skipped when unchanged since the last import unless force.
func (i *TMDBPopularityImporter) Run(ctx context.Context, force bool, runLog func(level, msg string)) (*ImportReport, error) {
	logf := func(level, format string, args ...any) {
		if runLog != nil {
			runLog(level, fmt.Sprintf(format, args...))
		}
	}
	known, err := i.history.KnownTmdbRefs(ctx)
	if err != nil {
		return nil, err
	}
	logf("info", "Recording TMDB popularity of the %d TMDB titles metareel knows", len(known))

	rep := &ImportReport{}
	var failed []string
	for _, k := range []struct{ export, kind string }{{"movie", "movie"}, {"tv_series", "tv"}} {
		res := i.importKind(ctx, k.export, k.kind, known, force)
		rep.Files = append(rep.Files, res)
		switch res.Status {
		case "imported":
			logf("info", "%s: recorded popularity of %d titles", res.Source, res.Rows)
		case "unchanged":
			logf("info", "%s: unchanged since the last import", res.Source)
		default:
			logf("error", "%s: %s", res.Source, res.Error)
			failed = append(failed, res.Source)
		}
	}
	if len(failed) > 0 {
		return rep, fmt.Errorf("tmdb popularity import failed: %v", failed)
	}
	return rep, nil
}

// importKind imports one export kind; the source key ("tmdb.popularity.tv")
// tracks the last imported file's version.
func (i *TMDBPopularityImporter) importKind(ctx context.Context, export, kind string, known map[string]bool, force bool) ImportFileResult {
	source := "tmdb.popularity." + kind
	today := i.now().UTC()
	for _, day := range []time.Time{today, today.AddDate(0, 0, -1)} {
		url := fmt.Sprintf(i.url, export, day.Format("01_02_2006"))
		date := repository.FormatDate(day)
		res := importFile(ctx, i.dl, i.imports, source, url, force, func(r io.Reader) (int, int, error) {
			return i.parse(ctx, r, kind, date, known)
		})
		// importFile flattens the error; retry yesterday only on a 404.
		if res.Status == "failed" && day.Equal(today) && isNotFound(res.Error) {
			continue
		}
		return res
	}
	return ImportFileResult{Source: source, Status: "failed", Error: "no export for today or yesterday"}
}

func isNotFound(msg string) bool {
	return strings.Contains(msg, client.ErrFileNotFound.Error())
}

func (i *TMDBPopularityImporter) parse(ctx context.Context, r io.Reader, kind, date string, known map[string]bool) (int, int, error) {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return 0, 0, fmt.Errorf("gunzip: %w", err)
	}
	defer gz.Close()
	sc := bufio.NewScanner(gz)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	var batch []repository.PopularityPoint
	rows, skipped := 0, 0
	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		err := i.history.SavePopularity(ctx, batch)
		rows += len(batch)
		batch = batch[:0]
		return err
	}
	for sc.Scan() {
		var line struct {
			ID         int64    `json:"id"`
			Popularity *float64 `json:"popularity"`
		}
		if err := json.Unmarshal(sc.Bytes(), &line); err != nil || line.ID <= 0 || line.Popularity == nil {
			skipped++
			continue
		}
		id := strconv.FormatInt(line.ID, 10)
		if !known[kind+":"+id] {
			continue
		}
		batch = append(batch, repository.PopularityPoint{
			Ref: repository.TmdbRef{Kind: kind, ID: id}, Date: date, Popularity: *line.Popularity,
		})
		if len(batch) >= popularityBatch {
			if err := flush(); err != nil {
				return rows, skipped, err
			}
		}
	}
	if err := sc.Err(); err != nil {
		return rows, skipped, fmt.Errorf("read export: %w", err)
	}
	return rows, skipped, flush()
}
