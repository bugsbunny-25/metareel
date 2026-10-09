package service

import (
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"log/slog"
	"strconv"
	"strings"

	"github.com/bugsbunny-25/metareel/internal/client"
	"github.com/bugsbunny-25/metareel/internal/repository"
)

// imdbRatingsURL is IMDb's daily ratings dataset (personal and
// non-commercial use only: https://developer.imdb.com/non-commercial-datasets/).
const imdbRatingsURL = "https://datasets.imdbws.com/title.ratings.tsv.gz"

// IMDbImporter imports IMDb's ratings dataset (the imdb.ratings.import job).
type IMDbImporter struct {
	log  *slog.Logger
	dl   *client.Downloader
	repo *repository.ImportsRepository
	// All keeps every title in the dataset (~1.6M rows) instead of only the
	// IMDb IDs metareel knows.
	All bool
}

func NewIMDbImporter(log *slog.Logger, dl *client.Downloader, repo *repository.ImportsRepository, all bool) *IMDbImporter {
	return &IMDbImporter{log: log, dl: dl, repo: repo, All: all}
}

const imdbBatch = 5000

// Run imports the ratings dataset unless it is unchanged (or force).
func (i *IMDbImporter) Run(ctx context.Context, force bool, runLog func(level, msg string)) (*ImportReport, error) {
	logf := func(level, format string, args ...any) {
		if runLog != nil {
			runLog(level, fmt.Sprintf(format, args...))
		}
	}
	var known map[string]bool
	if !i.All {
		var err error
		if known, err = i.repo.KnownIMDbIDs(ctx); err != nil {
			return nil, err
		}
		logf("info", "Keeping ratings for the %d IMDb IDs metareel knows", len(known))
	}

	res := importFile(ctx, i.dl, i.repo, "imdb.ratings", imdbRatingsURL, force, func(r io.Reader) (int, int, error) {
		return i.parse(ctx, r, known)
	})
	rep := &ImportReport{Files: []ImportFileResult{res}}
	switch res.Status {
	case "imported":
		logf("info", "imdb.ratings: stored %d ratings (%d malformed rows skipped)", res.Rows, res.Skipped)
	case "unchanged":
		logf("info", "imdb.ratings: unchanged since the last import")
	default:
		logf("error", "imdb.ratings: %s", res.Error)
		return rep, fmt.Errorf("imdb ratings import: %s", res.Error)
	}
	return rep, nil
}

func (i *IMDbImporter) parse(ctx context.Context, r io.Reader, known map[string]bool) (int, int, error) {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return 0, 0, fmt.Errorf("gunzip: %w", err)
	}
	defer gz.Close()
	t, err := newTSVReader(gz, "tconst", "averageRating", "numVotes")
	if err != nil {
		return 0, 0, err
	}
	var batch []repository.IMDbRating
	rows, skipped := 0, 0
	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		err := i.repo.SaveIMDbRatings(ctx, batch)
		rows += len(batch)
		batch = batch[:0]
		return err
	}
	for t.next() {
		id := t.str("tconst")
		if known != nil && !known[id] {
			continue
		}
		rating, err1 := strconv.ParseFloat(t.str("averageRating"), 64)
		votes, err2 := strconv.ParseInt(t.str("numVotes"), 10, 64)
		if !strings.HasPrefix(id, "tt") || err1 != nil || err2 != nil {
			skipped++
			continue
		}
		batch = append(batch, repository.IMDbRating{IMDbID: id, Rating: rating, Votes: votes})
		if len(batch) >= imdbBatch {
			if err := flush(); err != nil {
				return rows, skipped, err
			}
		}
	}
	if err := t.err(); err != nil {
		return rows, skipped, err
	}
	return rows, skipped, flush()
}
